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

// SSHSession 承载可复用的 SSH 连接元数据,不含任何密文。
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
	HostSpecs           SSHHostSpecs           `json:"hostSpecs"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
}

// SSHHostSpecs 承载经 SSH 采集一次并随 Session 持久化的主机容量信息。
type SSHHostSpecs struct {
	CPUCount    int        `json:"cpuCount"`
	MemoryBytes int64      `json:"memoryBytes"`
	DiskBytes   int64      `json:"diskBytes"`
	CollectedAt *time.Time `json:"collectedAt,omitempty"`
}

// Empty 返回该 Session 是否尚无持久化主机规格,迁移前的历史行即为此状态。
func (s SSHHostSpecs) Empty() bool {
	return s.CollectedAt == nil && s.CPUCount == 0 && s.MemoryBytes == 0 && s.DiskBytes == 0
}

// Collected 返回持久化的主机规格是否完整,完整则列表无需再次 SSH 探测。
func (s SSHHostSpecs) Collected() bool {
	return s.CollectedAt != nil && !s.CollectedAt.IsZero() && s.CPUCount > 0 && s.MemoryBytes > 0 && s.DiskBytes > 0
}

// Validate 拒绝采集不完整或非正数的主机容量信息。
func (s SSHHostSpecs) Validate() error {
	if s.CPUCount < 1 || s.MemoryBytes < 1 || s.DiskBytes < 1 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH 主机规格必须为正数")
	}
	if s.CollectedAt == nil || s.CollectedAt.IsZero() {
		return apperror.New(apperror.CodeValidationRequired, "SSH 主机规格采集时间不能为空")
	}
	return nil
}

// EffectiveSSHConfig 承载应用全局默认与 Session 覆盖后解析出的连接取值。
type EffectiveSSHConfig struct {
	Port                uint16
	ConnectTimeoutSec   int
	HandshakeTimeoutSec int
	KeepAliveSec        int
	Compression         bool
	HostKeyPolicy       enums.SSHHostKeyPolicy
}

// ResolveSSHConfig 应用全局设置,除非该 Session 显式启用了逐连接覆盖。
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

// SSHCredential 存放加密库中的密文字节,绝不能经桌面 DTO 暴露。
type SSHCredential struct {
	ID         ID                `json:"-"`
	AuthType   enums.SSHAuthType `json:"-"`
	Secret     []byte            `json:"-"`
	Passphrase []byte            `json:"-"`
	CreatedAt  time.Time         `json:"-"`
	UpdatedAt  time.Time         `json:"-"`
}

// KnownHost 存放一条受信任的 SSH 公钥指纹及其替换历史。
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

// NewSSHSession 创建已校验的连接元数据,默认启用严格主机密钥校验。
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

// Validate 校验 SSH 连接元数据与凭据引用,不做网络访问。
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
	if !s.HostSpecs.Empty() {
		return s.HostSpecs.Validate()
	}
	return nil
}

// NewSSHCredential 创建口令或私钥凭据,用于加密 SQLite 持久化。
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

// Clear 覆写内存中的凭据字节切片并释放其引用。
func (c *SSHCredential) Clear() {
	if c == nil {
		return
	}
	clear(c.Secret)
	clear(c.Passphrase)
	c.Secret = nil
	c.Passphrase = nil
}

// NewKnownHost 在校验标识与指纹后创建一条受信任的主机密钥记录。
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

// ValidateKnownHost 接受明文、非标准端口或 OpenSSH 哈希形式的主机标识。
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

// MatchKnownHostIdentifier 比较明文、非标准端口或 OpenSSH 哈希形式的标识。
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
