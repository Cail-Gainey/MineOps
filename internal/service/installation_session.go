package service

import (
	"context"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// InstallationSessionFactory creates the shared SSH session used by every step of one installation task.
type InstallationSessionFactory func(serverID model.ID) *InstallationSession

// InstallationSession owns the single SSH connection shared by all steps of one installation task.
//
// 一次安装只握手一次:步骤通过 Client 取用共享连接、通过 Home 取用缓存的远程 $HOME,不再各自 Connect/Close。
// x/crypto/ssh 的 *ssh.Client 允许并发开通道,所以同一波次内的并发步骤可以共用它。
// 只有 InstallationRunner 可以 Close,且必须在全部步骤结束之后 —— 提前关闭会打断在途命令。
type InstallationSession struct {
	clients  *SSHClientFactory
	store    repository.Store
	settings *appsettings.Manager
	serverID model.ID

	mu      sync.Mutex
	client  *SSHClient
	session *model.SSHSession
	home    string
	closed  bool
}

func newInstallationSession(clients *SSHClientFactory, store repository.Store, settings *appsettings.Manager, serverID model.ID) *InstallationSession {
	return &InstallationSession{clients: clients, store: store, settings: settings, serverID: serverID}
}

// Client returns the shared authenticated client, dialing it at most once per task.
//
// 连接建立后不再替换:步骤会把 client 存进局部变量并跨数分钟的命令持有它
// (install_java 与 download_server 的超时都是 20 分钟),中途重连会把在途命令的通道一起关掉。
// 因此这里不做存活探测、不做惰性重连 —— 连接真的断了就让步骤失败,由 Retry 重新建连。
func (s *InstallationSession) Client(ctx context.Context) (*SSHClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, apperror.New(apperror.CodeValidationConflict, "安装 SSH 会话已关闭")
	}
	if s.client != nil {
		return s.client, nil
	}
	sshSessionID, err := s.sshSessionID(ctx)
	if err != nil {
		return nil, err
	}
	sshSession, err := s.store.SSHSessions().Get(ctx, sshSessionID)
	if err != nil {
		return nil, err
	}
	client, err := s.clients.Connect(ctx, sshSession, s.settings.Snapshot().SSH)
	if err != nil {
		return nil, err
	}
	s.client, s.session = client, sshSession
	return client, nil
}

// SSHSession returns the SSH Session metadata backing the shared connection.
func (s *InstallationSession) SSHSession(ctx context.Context) (*model.SSHSession, error) {
	if _, err := s.Client(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil, apperror.New(apperror.CodeIONotFound, "安装 SSH Session 不可用")
	}
	return s.session, nil
}

// Server returns a freshly read Minecraft Server row.
//
// 不缓存:resolve_java、install_java、write_eula 都会更新这一行,缓存会让后续步骤读到过期数据。
func (s *InstallationSession) Server(ctx context.Context) (*model.MinecraftServer, error) {
	return s.store.MinecraftServers().Get(ctx, s.serverID, false)
}

// Home returns the remote $HOME, resolving it at most once per connection.
func (s *InstallationSession) Home(ctx context.Context) (string, error) {
	s.mu.Lock()
	cached := s.home
	s.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	client, err := s.Client(ctx)
	if err != nil {
		return "", err
	}
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "printenv", Arguments: []string{"HOME"},
		Timeout: 5 * time.Second, MaximumOutput: 4096,
	})
	if err != nil || strings.TrimSpace(result.Stdout) == "" {
		return "", apperror.Wrap(apperror.CodeIOPermissionDenied, "无法解析远程 Home", err)
	}
	home := path.Clean(strings.TrimSpace(result.Stdout))
	s.mu.Lock()
	s.home = home
	s.mu.Unlock()
	return home, nil
}

// Close releases the shared connection; it is idempotent and safe to call from a defer.
func (s *InstallationSession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	client := s.client
	s.client = nil
	s.home = ""
	s.closed = true
	s.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Close()
}

// sshSessionID reads the server row only to learn which SSH Session to dial. Caller holds s.mu.
func (s *InstallationSession) sshSessionID(ctx context.Context) (model.ID, error) {
	if s.session != nil {
		return s.session.ID, nil
	}
	server, err := s.store.MinecraftServers().Get(ctx, s.serverID, false)
	if err != nil {
		return "", err
	}
	return server.SSHSessionID, nil
}
