package model

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// SSHSession contains reusable SSH connection metadata without secret material.
type SSHSession struct {
	ID                  ID                     `json:"id"`
	Name                string                 `json:"name"`
	Host                string                 `json:"host"`
	Port                uint16                 `json:"port"`
	Username            string                 `json:"username"`
	AuthType            enums.SSHAuthType      `json:"authType"`
	CredentialID        *ID                    `json:"credentialID,omitempty"`
	HostKeyPolicy       enums.SSHHostKeyPolicy `json:"hostKeyPolicy"`
	Group               string                 `json:"group"`
	Favourite           bool                   `json:"favourite"`
	Remark              string                 `json:"remark"`
	ConnectTimeoutSec   int                    `json:"connectTimeoutSec"`
	HandshakeTimeoutSec int                    `json:"handshakeTimeoutSec"`
	KeepAliveSec        int                    `json:"keepAliveSec"`
	Compression         bool                   `json:"compression"`
	OverrideSettings    bool                   `json:"overrideSettings"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
}

// EffectiveSSHConfig contains resolved connection values after applying global defaults and Session overrides.
type EffectiveSSHConfig struct {
	Port                uint16
	ConnectTimeoutSec   int
	HandshakeTimeoutSec int
	KeepAliveSec        int
	Compression         bool
	HostKeyPolicy       enums.SSHHostKeyPolicy
}

// ResolveSSHConfig applies global Settings unless the Session explicitly enables per-connection overrides.
func ResolveSSHConfig(session SSHSession, settings SSHSettings) EffectiveSSHConfig {
	if session.OverrideSettings {
		return EffectiveSSHConfig{
			Port: session.Port, ConnectTimeoutSec: session.ConnectTimeoutSec,
			HandshakeTimeoutSec: session.HandshakeTimeoutSec, KeepAliveSec: session.KeepAliveSec,
			Compression: session.Compression, HostKeyPolicy: session.HostKeyPolicy,
		}
	}
	return EffectiveSSHConfig{
		Port: settings.DefaultPort, ConnectTimeoutSec: settings.ConnectTimeoutSec,
		HandshakeTimeoutSec: settings.HandshakeTimeoutSec, KeepAliveSec: settings.KeepAliveSec,
		Compression: settings.Compression, HostKeyPolicy: enums.SSHHostKeyPolicy(settings.DefaultHostKeyPolicy),
	}
}

// SSHCredential stores encrypted-database secret bytes and must never be exposed through a desktop DTO.
type SSHCredential struct {
	ID         ID                `json:"-"`
	AuthType   enums.SSHAuthType `json:"-"`
	Secret     []byte            `json:"-"`
	Passphrase []byte            `json:"-"`
	CreatedAt  time.Time         `json:"-"`
	UpdatedAt  time.Time         `json:"-"`
}

// KnownHost stores one trusted SSH public-key fingerprint and replacement history.
type KnownHost struct {
	ID             ID         `json:"id"`
	HostIdentifier string     `json:"hostIdentifier"`
	Host           string     `json:"host"`
	Port           uint16     `json:"port"`
	Algorithm      string     `json:"algorithm"`
	PublicKey      []byte     `json:"-"`
	Fingerprint    string     `json:"fingerprint"`
	FirstSeenAt    time.Time  `json:"firstSeenAt"`
	LastSeenAt     time.Time  `json:"lastSeenAt"`
	ReplacedAt     *time.Time `json:"replacedAt,omitempty"`
	ReplacedByID   *ID        `json:"replacedByID,omitempty"`
}

// NewSSHSession creates validated connection metadata with strict host-key verification by default.
func NewSSHSession(clock Clock, session SSHSession) (*SSHSession, error) {
	if clock == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Clock 不能为空")
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 SSH Session ID 失败", err)
	}
	session.ID = id
	session.CreatedAt = now
	session.UpdatedAt = now
	if session.Port == 0 {
		session.Port = 22
	}
	if session.HostKeyPolicy == "" {
		session.HostKeyPolicy = enums.SSHHostKeyStrict
	}
	if session.ConnectTimeoutSec == 0 {
		session.ConnectTimeoutSec = 10
	}
	if session.HandshakeTimeoutSec == 0 {
		session.HandshakeTimeoutSec = 15
	}
	if session.KeepAliveSec == 0 {
		session.KeepAliveSec = 30
	}
	if err := session.Validate(); err != nil {
		return nil, err
	}
	return &session, nil
}

// Validate checks SSH connection metadata and credential references without performing network access.
func (s SSHSession) Validate() error {
	if strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.Host) == "" || strings.TrimSpace(s.Username) == "" {
		return apperror.New(apperror.CodeValidationRequired, "SSH 名称、主机和用户名不能为空")
	}
	if s.Port == 0 || !s.AuthType.Valid() || !s.HostKeyPolicy.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH 端口、认证方式或主机密钥策略无效")
	}
	if s.AuthType == enums.SSHAuthAgent && s.CredentialID != nil {
		return apperror.New(apperror.CodeValidationConflict, "SSH Agent 认证不能绑定数据库凭据")
	}
	if s.AuthType != enums.SSHAuthAgent && (s.CredentialID == nil || !s.CredentialID.Valid()) {
		return apperror.New(apperror.CodeValidationRequired, "密码或私钥认证必须绑定有效凭据")
	}
	if s.ConnectTimeoutSec < 1 || s.HandshakeTimeoutSec < 1 || s.KeepAliveSec < 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH 超时或 KeepAlive 配置无效")
	}
	return nil
}

// NewSSHCredential creates a password or private-key credential for encrypted SQLite persistence.
func NewSSHCredential(clock Clock, authType enums.SSHAuthType, secret, passphrase []byte) (*SSHCredential, error) {
	if clock == nil || len(secret) == 0 {
		return nil, apperror.New(apperror.CodeValidationRequired, "SSH 凭据内容不能为空")
	}
	if authType != enums.SSHAuthPassword && authType != enums.SSHAuthPrivateKey {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "SSH 凭据类型无效")
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 SSH Credential ID 失败", err)
	}
	return &SSHCredential{
		ID: id, AuthType: authType, Secret: append([]byte(nil), secret...),
		Passphrase: append([]byte(nil), passphrase...), CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Clear overwrites in-memory credential byte slices and releases their references.
func (c *SSHCredential) Clear() {
	if c == nil {
		return
	}
	clear(c.Secret)
	clear(c.Passphrase)
	c.Secret = nil
	c.Passphrase = nil
}

// NewKnownHost creates a trusted host-key record after validating its identifier and fingerprint.
func NewKnownHost(clock Clock, hostIdentifier, host string, port uint16, algorithm string, publicKey []byte, fingerprint string) (*KnownHost, error) {
	if clock == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Clock 不能为空")
	}
	if err := ValidateKnownHost(hostIdentifier, host, port, algorithm, publicKey, fingerprint); err != nil {
		return nil, err
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 Known Host ID 失败", err)
	}
	return &KnownHost{
		ID: id, HostIdentifier: hostIdentifier, Host: host, Port: port, Algorithm: algorithm,
		PublicKey: append([]byte(nil), publicKey...), Fingerprint: fingerprint, FirstSeenAt: now, LastSeenAt: now,
	}, nil
}

// ValidateKnownHost accepts plain, non-standard-port, or OpenSSH hashed host identifiers.
func ValidateKnownHost(hostIdentifier, host string, port uint16, algorithm string, publicKey []byte, fingerprint string) error {
	if strings.TrimSpace(hostIdentifier) == "" || strings.TrimSpace(host) == "" || port == 0 {
		return apperror.New(apperror.CodeValidationRequired, "Known Host 标识、主机和端口不能为空")
	}
	if !validHostIdentifier(hostIdentifier) || !validHostKeyAlgorithm(algorithm) || len(publicKey) == 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Known Host 标识或公钥算法无效")
	}
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Known Host 指纹必须使用 SHA256 格式")
	}
	if _, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(fingerprint, "SHA256:")); err != nil {
		return apperror.Wrap(apperror.CodeValidationInvalidArgument, "Known Host SHA256 指纹无效", err)
	}
	return nil
}

// MatchKnownHostIdentifier compares plain, non-standard-port, or OpenSSH hashed identifiers.
func MatchKnownHostIdentifier(stored, candidate string) bool {
	if stored == candidate {
		return true
	}
	if !strings.HasPrefix(stored, "|1|") {
		return false
	}
	parts := strings.Split(stored, "|")
	if len(parts) != 4 {
		return false
	}
	salt, saltErr := base64.StdEncoding.DecodeString(parts[2])
	expected, hashErr := base64.StdEncoding.DecodeString(parts[3])
	if saltErr != nil || hashErr != nil {
		return false
	}
	hash := hmac.New(sha1.New, salt)
	_, _ = hash.Write([]byte(candidate))
	return hmac.Equal(hash.Sum(nil), expected)
}

func validHostIdentifier(value string) bool {
	if strings.HasPrefix(value, "|1|") {
		parts := strings.Split(value, "|")
		if len(parts) != 4 {
			return false
		}
		_, saltErr := base64.StdEncoding.DecodeString(parts[2])
		_, hashErr := base64.StdEncoding.DecodeString(parts[3])
		return saltErr == nil && hashErr == nil
	}
	if strings.HasPrefix(value, "[") {
		return strings.Contains(value, "]:")
	}
	return !strings.ContainsAny(value, " \t\r\n")
}

func validHostKeyAlgorithm(value string) bool {
	return strings.HasPrefix(value, "ssh-") || strings.HasPrefix(value, "ecdsa-") || strings.HasPrefix(value, "sk-")
}
