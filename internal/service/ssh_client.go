package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHClient owns an authenticated SSH transport, optional Agent socket, and KeepAlive loop.
type SSHClient struct {
	client        *ssh.Client
	jump          *SSHClient
	remoteAddress string
	agentConn     net.Conn
	cancel        context.CancelFunc
	wait          sync.WaitGroup
	requestMu     sync.Mutex
	closeOnce     sync.Once
}

// Raw returns the authenticated x/crypto SSH client for Terminal and SFTP adapters.
func (c *SSHClient) Raw() *ssh.Client {
	if c == nil {
		return nil
	}
	return c.client
}

// RemoteAddress returns the configured SSH target address even when the transport is tunneled through a Jump Host.
func (c *SSHClient) RemoteAddress() string {
	if c == nil {
		return ""
	}
	return c.remoteAddress
}

// Close stops KeepAlive and releases the SSH transport and Agent socket.
func (c *SSHClient) Close() error {
	if c == nil {
		return nil
	}
	var closeError error
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		if c.client != nil {
			closeError = c.client.Close()
		}
		if c.jump != nil {
			closeError = errors.Join(closeError, c.jump.Close())
		}
		if c.agentConn != nil {
			closeError = errors.Join(closeError, c.agentConn.Close())
		}
		c.wait.Wait()
	})
	return closeError
}

// SSHClientFactory builds authenticated clients using encrypted credentials and strict host-key decisions.
type SSHClientFactory struct {
	sessions   *SSHSessionManager
	knownHosts *KnownHostManager
}

// SSHConnectionTestResult contains authenticated handshake and command-channel evidence.
type SSHConnectionTestResult struct {
	ServerVersion   string
	RemoteAddress   string
	AuthType        enums.SSHAuthType
	ConnectDuration time.Duration
}

// NewSSHClientFactory creates the unified SSH transport factory.
func NewSSHClientFactory(sessions *SSHSessionManager, knownHosts *KnownHostManager) (*SSHClientFactory, error) {
	if sessions == nil || knownHosts == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "SSH Client Factory 依赖不能为空")
	}
	return &SSHClientFactory{sessions: sessions, knownHosts: knownHosts}, nil
}

// Connect performs TCP, SSH handshake, strict host-key verification, and configured authentication.
func (f *SSHClientFactory) Connect(ctx context.Context, session *model.SSHSession, settings model.SSHSettings) (*SSHClient, error) {
	return f.connect(ctx, session, settings, nil)
}

func (f *SSHClientFactory) connect(ctx context.Context, session *model.SSHSession, settings model.SSHSettings, credentialOverride *model.SSHCredential) (*SSHClient, error) {
	if session == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "SSH Session 不能为空")
	}
	effective := model.ResolveSSHConfig(*session, settings)
	authMethods, agentConn, err := f.authMethods(ctx, session, credentialOverride)
	if err != nil {
		return nil, err
	}
	cleanupAgent := true
	defer func() {
		if cleanupAgent && agentConn != nil {
			_ = agentConn.Close()
		}
	}()

	address := net.JoinHostPort(session.Host, strconv.Itoa(int(effective.Port)))
	var jumpClient *SSHClient
	var connection net.Conn
	jumpID, useJumpHost, err := resolveJumpHostID(session, settings)
	if err != nil {
		return nil, err
	}
	if useJumpHost {
		jumpSession, jumpErr := f.sessions.Get(ctx, jumpID)
		if jumpErr != nil {
			return nil, apperror.Wrap(apperror.CodeSSHConnectionFailed, "读取默认 Jump Host 失败", jumpErr)
		}
		jumpSettings := settings
		jumpSettings.DefaultJumpHostID = ""
		jumpClient, err = f.Connect(ctx, jumpSession, jumpSettings)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeSSHConnectionFailed, "连接默认 Jump Host 失败", err)
		}
		connection, err = dialThroughJump(ctx, jumpClient.Raw(), address, time.Duration(effective.ConnectTimeoutSec)*time.Second)
	} else {
		dialCtx, cancelDial := context.WithTimeout(ctx, time.Duration(effective.ConnectTimeoutSec)*time.Second)
		defer cancelDial()
		connection, err = (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
	}
	if err != nil {
		if jumpClient != nil {
			_ = jumpClient.Close()
		}
		return nil, apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH TCP 连接失败", err).WithDetails(map[string]any{
			"stage": "tcp", "host": session.Host, "port": effective.Port, "jumpHost": useJumpHost,
		})
	}
	closeConnection := true
	defer func() {
		if closeConnection {
			_ = connection.Close()
		}
		if closeConnection && jumpClient != nil {
			_ = jumpClient.Close()
		}
	}()

	handshakeDeadline := time.Now().Add(time.Duration(effective.HandshakeTimeoutSec) * time.Second)
	if err := connection.SetDeadline(handshakeDeadline); err != nil {
		return nil, apperror.Wrap(apperror.CodeSSHConnectionFailed, "设置 SSH 握手超时失败", err)
	}
	config := &ssh.ClientConfig{
		User:            session.Username,
		Auth:            authMethods,
		HostKeyCallback: f.hostKeyCallback(ctx, session, effective),
		ClientVersion:   "SSH-2.0-MineOps",
		Timeout:         time.Duration(effective.ConnectTimeoutSec) * time.Second,
	}
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if err != nil {
		return nil, mapSSHHandshakeError(err, session.AuthType)
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		_ = clientConnection.Close()
		return nil, apperror.Wrap(apperror.CodeSSHConnectionFailed, "清除 SSH 握手超时失败", err)
	}
	clientCtx, cancelClient := context.WithCancel(context.Background())
	result := &SSHClient{
		client: ssh.NewClient(clientConnection, channels, requests), jump: jumpClient, remoteAddress: address,
		agentConn: agentConn, cancel: cancelClient,
	}
	if effective.KeepAliveSec > 0 {
		result.wait.Add(1)
		go result.keepAlive(clientCtx, time.Duration(effective.KeepAliveSec)*time.Second)
	}
	cleanupAgent = false
	closeConnection = false
	return result, nil
}

func resolveJumpHostID(session *model.SSHSession, settings model.SSHSettings) (model.ID, bool, error) {
	if settings.DefaultJumpHostID == "" {
		return "", false, nil
	}
	jumpID := model.ID(settings.DefaultJumpHostID)
	if !jumpID.Valid() {
		return "", false, apperror.New(apperror.CodeValidationInvalidArgument, "默认 Jump Host ID 无效")
	}
	if session != nil && jumpID == session.ID {
		return "", false, nil
	}
	return jumpID, true, nil
}

func dialThroughJump(ctx context.Context, jump *ssh.Client, address string, timeout time.Duration) (net.Conn, error) {
	if jump == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Jump Host SSH Client 不能为空")
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		connection net.Conn
		err        error
	}
	completed := make(chan result, 1)
	go func() {
		connection, err := jump.Dial("tcp", address)
		completed <- result{connection: connection, err: err}
	}()
	select {
	case <-dialCtx.Done():
		_ = jump.Close()
		return nil, dialCtx.Err()
	case value := <-completed:
		return value.connection, value.err
	}
}

// TestConnection authenticates, opens a session channel, and runs the no-op `true` command.
func (f *SSHClientFactory) TestConnection(ctx context.Context, session *model.SSHSession, settings model.SSHSettings) (SSHConnectionTestResult, error) {
	return f.testConnection(ctx, session, settings, nil)
}

// TestConnectionWithCredential tests an in-memory draft without loading or persisting its credential.
func (f *SSHClientFactory) TestConnectionWithCredential(ctx context.Context, session *model.SSHSession, credential *model.SSHCredential, settings model.SSHSettings) (SSHConnectionTestResult, error) {
	if session != nil && session.AuthType != enums.SSHAuthAgent && credential == nil {
		return SSHConnectionTestResult{}, apperror.New(apperror.CodeValidationRequired, "SSH 连接测试凭据不能为空")
	}
	return f.testConnection(ctx, session, settings, credential)
}

func (f *SSHClientFactory) testConnection(ctx context.Context, session *model.SSHSession, settings model.SSHSettings, credentialOverride *model.SSHCredential) (SSHConnectionTestResult, error) {
	startedAt := time.Now()
	client, err := f.connect(ctx, session, settings, credentialOverride)
	if err != nil {
		return SSHConnectionTestResult{}, err
	}
	defer func() { _ = client.Close() }()
	remoteAddress := client.RemoteAddress()
	serverVersion := string(client.Raw().ServerVersion())
	commandSession, err := client.Raw().NewSession()
	if err != nil {
		return SSHConnectionTestResult{}, apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 会话通道创建失败", err).WithDetails(map[string]any{"stage": "channel"})
	}
	defer func() { _ = commandSession.Close() }()
	commandResult := make(chan error, 1)
	go func() { commandResult <- commandSession.Run("true") }()
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case <-commandCtx.Done():
		_ = client.Close()
		return SSHConnectionTestResult{}, apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 权限验证命令超时", commandCtx.Err()).WithDetails(map[string]any{"stage": "command"})
	case err := <-commandResult:
		if err != nil {
			return SSHConnectionTestResult{}, apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 权限验证命令失败", err).WithDetails(map[string]any{"stage": "command"})
		}
	}
	return SSHConnectionTestResult{
		ServerVersion: serverVersion, RemoteAddress: remoteAddress,
		AuthType: session.AuthType, ConnectDuration: time.Since(startedAt),
	}, nil
}

func (f *SSHClientFactory) authMethods(ctx context.Context, session *model.SSHSession, credentialOverride *model.SSHCredential) ([]ssh.AuthMethod, net.Conn, error) {
	if session.AuthType == enums.SSHAuthAgent {
		if runtime.GOOS == "windows" {
			return nil, nil, apperror.New(apperror.CodeAgentUnavailable, "Windows OpenSSH Agent 命名管道尚不可用").WithDetails(map[string]any{
				"platform": runtime.GOOS,
			})
		}
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return nil, nil, apperror.New(apperror.CodeAgentUnavailable, "SSH_AUTH_SOCK 未设置")
		}
		dialer := net.Dialer{}
		connection, err := dialer.DialContext(ctx, "unix", socket)
		if err != nil {
			return nil, nil, apperror.Wrap(apperror.CodeAgentUnavailable, "连接 SSH Agent 失败", err)
		}
		signers, err := agent.NewClient(connection).Signers()
		if err != nil {
			_ = connection.Close()
			return nil, nil, apperror.Wrap(apperror.CodeAgentUnavailable, "读取 SSH Agent Keys 失败", err)
		}
		if len(signers) == 0 {
			_ = connection.Close()
			return nil, nil, apperror.New(apperror.CodeAgentUnavailable, "SSH Agent 中没有可用 Key")
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, connection, nil
	}

	credential := credentialOverride
	if credential == nil {
		var err error
		credential, err = f.sessions.LoadCredential(ctx, session)
		if err != nil {
			return nil, nil, err
		}
		defer credential.Clear()
	}
	if credential.AuthType != session.AuthType {
		return nil, nil, apperror.New(apperror.CodeValidationConflict, "SSH Session 与测试凭据认证类型不一致")
	}
	if session.AuthType == enums.SSHAuthPassword {
		password := string(credential.Secret)
		keyboardInteractive := ssh.KeyboardInteractive(func(_ string, _ string, questions []string, echos []bool) ([]string, error) {
			if len(questions) == 0 {
				return []string{}, nil
			}
			if len(questions) != 1 || len(echos) != 1 || echos[0] {
				return nil, errors.New("SSH keyboard-interactive 需要密码以外的交互信息")
			}
			return []string{password}, nil
		})
		return []ssh.AuthMethod{
			ssh.PasswordCallback(func() (string, error) { return password, nil }),
			keyboardInteractive,
		}, nil, nil
	}
	var signer ssh.Signer
	var err error
	if len(credential.Passphrase) > 0 {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(credential.Secret, credential.Passphrase)
	} else {
		signer, err = ssh.ParsePrivateKey(credential.Secret)
	}
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil, nil, apperror.Wrap(apperror.CodeSSHAuthenticationFailed, "SSH 私钥需要口令", err)
		}
		return nil, nil, apperror.Wrap(apperror.CodeSSHAuthenticationFailed, "解析 SSH 私钥失败", err)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil, nil
}

func (f *SSHClientFactory) hostKeyCallback(ctx context.Context, session *model.SSHSession, effective model.EffectiveSSHConfig) ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		hostIdentifier := knownhosts.Normalize(hostname)
		fingerprint := ssh.FingerprintSHA256(key)
		check, err := f.knownHosts.Check(ctx, hostIdentifier, fingerprint)
		if err != nil {
			return err
		}
		if check.Decision == HostKeyTrusted {
			return nil
		}
		details := map[string]any{
			"decision": string(check.Decision), "hostIdentifier": hostIdentifier,
			"host": session.Host, "port": effective.Port, "algorithm": key.Type(),
			"fingerprint": fingerprint, "publicKeyBase64": encodePublicKey(key.Marshal()),
		}
		if check.Existing != nil {
			details["previousFingerprint"] = check.Existing.Fingerprint
			details["previousAlgorithm"] = check.Existing.Algorithm
		}
		message := "SSH 主机密钥需要首次信任确认"
		if check.Decision == HostKeyChanged {
			message = "SSH 主机指纹发生变化，连接已拒绝"
		}
		return apperror.New(apperror.CodeSSHHostKeyRejected, message).WithDetails(details)
	}
}

func (c *SSHClient) keepAlive(ctx context.Context, interval time.Duration) {
	defer c.wait.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := c.sendKeepAliveRequest(); err != nil {
				_ = c.client.Close()
				return
			}
		}
	}
}

func (c *SSHClient) sendKeepAliveRequest() (time.Duration, error) {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()
	startedAt := time.Now()
	_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
	return time.Since(startedAt), err
}

func mapSSHHandshakeError(err error, authType enums.SSHAuthType) error {
	var applicationError *apperror.Error
	if errors.As(err, &applicationError) {
		return applicationError
	}
	message := strings.ToLower(err.Error())
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		if strings.Contains(message, "handshake failed: read") || strings.Contains(message, "banner exchange") {
			return apperror.Wrap(apperror.CodeSSHConnectionFailed, "TCP 已连接，但服务器未在超时时间内发送 SSH 协议 Banner", err).WithDetails(map[string]any{
				"stage": "banner",
			})
		}
		return apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 握手超时", err).WithDetails(map[string]any{"stage": "handshake"})
	}
	var algorithmError *ssh.AlgorithmNegotiationError
	if errors.As(err, &algorithmError) {
		return apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 加密算法协商失败，目标服务器可能只支持旧算法", err).WithDetails(map[string]any{
			"stage": "algorithm_negotiation", "algorithmType": algorithmError.What,
			"clientAlgorithms": algorithmError.SupportedAlgorithms, "serverAlgorithms": algorithmError.RequestedAlgorithms,
		})
	}
	if strings.Contains(message, "unable to authenticate") || strings.Contains(message, "no supported methods remain") {
		methods := []string{authType.String()}
		if authType == enums.SSHAuthPassword {
			methods = []string{"password", "keyboard-interactive"}
		}
		return apperror.Wrap(apperror.CodeSSHAuthenticationFailed, "SSH 用户名、凭据或服务器允许的认证方式不匹配", err).WithDetails(map[string]any{
			"stage": "authentication", "authType": authType.String(), "attemptedMethods": methods,
		})
	}
	if strings.Contains(message, "connection reset") || strings.Contains(message, "connection closed") || strings.Contains(message, "unexpected eof") || strings.HasSuffix(message, ": eof") {
		return apperror.Wrap(apperror.CodeSSHConnectionFailed, "SSH 服务在握手期间主动断开连接", err).WithDetails(map[string]any{
			"stage": "handshake",
		})
	}
	return apperror.Wrap(apperror.CodeSSHAuthenticationFailed, "SSH 握手或认证失败", err).WithDetails(map[string]any{
		"stage": "authentication", "authType": authType.String(),
	})
}

func encodePublicKey(value []byte) string {
	return base64.StdEncoding.EncodeToString(value)
}
