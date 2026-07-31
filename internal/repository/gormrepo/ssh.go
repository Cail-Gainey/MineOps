package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
)

// SSHSessionRecord 是可复用 SSH 连接元数据在加密 SQLite 中的表示。
type SSHSessionRecord struct {
	ID                  string `gorm:"primaryKey;size:36"`
	Name                string `gorm:"uniqueIndex;size:160"`
	Host                string `gorm:"index;size:255"`
	Port                uint16
	Username            string  `gorm:"size:160"`
	AuthType            string  `gorm:"size:24"`
	CredentialID        *string `gorm:"index;size:36"`
	HostKeyPolicy       string  `gorm:"size:32"`
	Group               string  `gorm:"index;size:120"`
	Favourite           bool    `gorm:"index"`
	Remark              string
	ConnectTimeoutSec   int
	HandshakeTimeoutSec int
	KeepAliveSec        int
	Compression         bool
	OverrideSettings    bool
	// 主机规格为持久化元数据:仅在创建或连接目标变更时通过 SSH 采集一次,列表直接读取。
	// 迁移前的历史行没有这些列,新增列带 NOT NULL DEFAULT 0 并允许采集时间为 NULL。
	HostCPUCount         int   `gorm:"not null;default:0"`
	HostMemoryBytes      int64 `gorm:"not null;default:0"`
	HostDiskBytes        int64 `gorm:"not null;default:0"`
	HostSpecsCollectedAt *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time `gorm:"index"`
}

// SSHCredentialRecord 仅在 SQLCipher 加密的数据库内存放密文字节。
type SSHCredentialRecord struct {
	ID         string `gorm:"primaryKey;size:36"`
	AuthType   string `gorm:"size:24"`
	Secret     []byte
	Passphrase []byte
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// KnownHostRecord 在加密 SQLite 中存放生效与历史的 SSH 主机密钥。
type KnownHostRecord struct {
	ID             string `gorm:"primaryKey;size:36"`
	HostIdentifier string `gorm:"index:idx_known_host_active,priority:1;size:512"`
	Host           string `gorm:"index;size:255"`
	Port           uint16
	Algorithm      string `gorm:"size:96"`
	PublicKey      []byte
	Fingerprint    string     `gorm:"size:160"`
	FirstSeenAt    time.Time  `gorm:"index"`
	LastSeenAt     time.Time  `gorm:"index"`
	ReplacedAt     *time.Time `gorm:"index:idx_known_host_active,priority:2"`
	ReplacedByID   *string    `gorm:"index;size:36"`
}

type sshSessionRepository struct{ database *gorm.DB }
type sshCredentialRepository struct{ database *gorm.DB }
type knownHostRepository struct{ store *Store }

// Create 新增一条记录。
func (r *sshSessionRepository) Create(ctx context.Context, session *model.SSHSession) error {
	if session == nil {
		return apperror.New(apperror.CodeValidationRequired, "SSH Session 不能为空")
	}
	if err := session.Validate(); err != nil {
		return err
	}
	record := sshSessionToRecord(session)
	if err := r.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 SSH Session 失败", err)
	}
	return nil
}

// Update 更新一条记录。
func (r *sshSessionRepository) Update(ctx context.Context, session *model.SSHSession) error {
	if session == nil {
		return apperror.New(apperror.CodeValidationRequired, "SSH Session 不能为空")
	}
	if err := session.Validate(); err != nil {
		return err
	}
	record := sshSessionToRecord(session)
	result := r.database.WithContext(ctx).Model(&SSHSessionRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 SSH Session 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "SSH Session 不存在")
	}
	return nil
}

// Get 按 ID 返回一条记录,未命中时返回 NotFound。
func (r *sshSessionRepository) Get(ctx context.Context, id model.ID) (*model.SSHSession, error) {
	var record SSHSessionRecord
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		return nil, mapSSHReadError("查询 SSH Session 失败", err)
	}
	session := recordToSSHSession(record)
	return &session, nil
}

// List 按查询条件分页列出记录。
func (r *sshSessionRepository) List(ctx context.Context, query repository.SSHSessionQuery) ([]model.SSHSession, error) {
	database := r.database.WithContext(ctx).Order("favourite desc, name asc")
	if search := strings.TrimSpace(query.Search); search != "" {
		pattern := "%" + search + "%"
		database = database.Where("name LIKE ? OR host LIKE ? OR username LIKE ? OR remark LIKE ?", pattern, pattern, pattern, pattern)
	}
	if query.Group != "" {
		database = database.Where("`group` = ?", query.Group)
	}
	if query.FavouriteOnly {
		database = database.Where("favourite = ?", true)
	}
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	var records []SSHSessionRecord
	if err := database.Limit(limit).Offset(query.Offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 SSH Session 列表失败", err)
	}
	result := make([]model.SSHSession, len(records))
	for index, record := range records {
		result[index] = recordToSSHSession(record)
	}
	return result, nil
}

// Delete 删除一条记录。
func (r *sshSessionRepository) Delete(ctx context.Context, id model.ID) error {
	result := r.database.WithContext(ctx).Delete(&SSHSessionRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 SSH Session 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "SSH Session 不存在")
	}
	return nil
}

// UpdateHostSpecs 写回某个 SSH Session 的主机规格与采集时间。
func (r *sshSessionRepository) UpdateHostSpecs(ctx context.Context, id model.ID, specs model.SSHHostSpecs) error {
	if err := specs.Validate(); err != nil {
		return err
	}
	result := r.database.WithContext(ctx).Model(&SSHSessionRecord{}).Where("id = ?", id.String()).
		Select("HostCPUCount", "HostMemoryBytes", "HostDiskBytes", "HostSpecsCollectedAt").
		UpdateColumns(&SSHSessionRecord{
			HostCPUCount: specs.CPUCount, HostMemoryBytes: specs.MemoryBytes,
			HostDiskBytes: specs.DiskBytes, HostSpecsCollectedAt: copyTime(specs.CollectedAt),
		})
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 SSH 主机规格失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "SSH Session 不存在")
	}
	return nil
}

// CountServerReferences 统计引用该 SSH Session 的 Server 数量,用于阻止误删。
func (r *sshSessionRepository) CountServerReferences(ctx context.Context, id model.ID) (int64, error) {
	if !r.database.Migrator().HasTable("server_records") {
		return 0, nil
	}
	var count int64
	if err := r.database.WithContext(ctx).Table("server_records").Where("ssh_session_id = ?", id.String()).Count(&count).Error; err != nil {
		return 0, apperror.Wrap(apperror.CodeIOReadFailed, "查询 SSH Session 引用失败", err)
	}
	return count, nil
}

// Create 新增一条记录。
func (r *sshCredentialRepository) Create(ctx context.Context, credential *model.SSHCredential) error {
	if credential == nil || len(credential.Secret) == 0 {
		return apperror.New(apperror.CodeValidationRequired, "SSH Credential 不能为空")
	}
	record := sshCredentialToRecord(credential)
	if err := r.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 SSH Credential 失败", err)
	}
	return nil
}

// Update 更新一条记录。
func (r *sshCredentialRepository) Update(ctx context.Context, credential *model.SSHCredential) error {
	if credential == nil || len(credential.Secret) == 0 {
		return apperror.New(apperror.CodeValidationRequired, "SSH Credential 不能为空")
	}
	record := sshCredentialToRecord(credential)
	result := r.database.WithContext(ctx).Model(&SSHCredentialRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 SSH Credential 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "SSH Credential 不存在")
	}
	return nil
}

// Get 按 ID 返回一条记录,未命中时返回 NotFound。
func (r *sshCredentialRepository) Get(ctx context.Context, id model.ID) (*model.SSHCredential, error) {
	var record SSHCredentialRecord
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		return nil, mapSSHReadError("查询 SSH Credential 失败", err)
	}
	credential := recordToSSHCredential(record)
	return &credential, nil
}

// Delete 删除一条记录。
func (r *sshCredentialRepository) Delete(ctx context.Context, id model.ID) error {
	result := r.database.WithContext(ctx).Delete(&SSHCredentialRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 SSH Credential 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "SSH Credential 不存在")
	}
	return nil
}

// Create 新增一条记录。
func (r *knownHostRepository) Create(ctx context.Context, knownHost *model.KnownHost) error {
	if knownHost == nil {
		return apperror.New(apperror.CodeValidationRequired, "Known Host 不能为空")
	}
	record := knownHostToRecord(knownHost)
	if err := r.store.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Known Host 失败", err)
	}
	return nil
}

// Update 更新一条记录。
func (r *knownHostRepository) Update(ctx context.Context, knownHost *model.KnownHost) error {
	if knownHost == nil {
		return apperror.New(apperror.CodeValidationRequired, "Known Host 不能为空")
	}
	record := knownHostToRecord(knownHost)
	result := r.store.database.WithContext(ctx).Model(&KnownHostRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Known Host 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Known Host 不存在")
	}
	return nil
}

// GetActive 返回某个主机标识当前生效的 Known Host,未命中时返回 NotFound。
func (r *knownHostRepository) GetActive(ctx context.Context, hostIdentifier string) (*model.KnownHost, error) {
	var record KnownHostRecord
	if err := r.store.database.WithContext(ctx).Where("host_identifier = ? AND replaced_at IS NULL", hostIdentifier).Order("last_seen_at desc").First(&record).Error; err != nil {
		return nil, mapSSHReadError("查询 Known Host 失败", err)
	}
	knownHost := recordToKnownHost(record)
	return &knownHost, nil
}

// FindActive 查找某个主机标识当前生效的 Known Host,未命中时返回空值而非错误。
func (r *knownHostRepository) FindActive(ctx context.Context, hostIdentifier string) (*model.KnownHost, error) {
	var records []KnownHostRecord
	if err := r.store.database.WithContext(ctx).Where("replaced_at IS NULL").Order("last_seen_at desc").Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Known Host 失败", err)
	}
	for _, record := range records {
		if model.MatchKnownHostIdentifier(record.HostIdentifier, hostIdentifier) {
			knownHost := recordToKnownHost(record)
			return &knownHost, nil
		}
	}
	return nil, apperror.New(apperror.CodeIONotFound, "Known Host 不存在")
}

// List 按查询条件分页列出记录。
func (r *knownHostRepository) List(ctx context.Context, search string, limit, offset int) ([]model.KnownHost, error) {
	database := r.store.database.WithContext(ctx).Order("last_seen_at desc")
	if search = strings.TrimSpace(search); search != "" {
		pattern := "%" + search + "%"
		database = database.Where("host LIKE ? OR host_identifier LIKE ? OR fingerprint LIKE ?", pattern, pattern, pattern)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var records []KnownHostRecord
	if err := database.Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Known Hosts 失败", err)
	}
	result := make([]model.KnownHost, len(records))
	for index, record := range records {
		result[index] = recordToKnownHost(record)
	}
	return result, nil
}

// Replace 把旧指纹置为历史并写入新的生效指纹。
func (r *knownHostRepository) Replace(ctx context.Context, previous, replacement *model.KnownHost) error {
	if previous == nil || replacement == nil || previous.HostIdentifier != replacement.HostIdentifier {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Known Host 替换记录无效")
	}
	return r.store.Transaction(ctx, func(registry repository.Registry) error {
		now := replacement.FirstSeenAt.UTC()
		previous.ReplacedAt = &now
		previous.ReplacedByID = &replacement.ID
		if err := registry.KnownHosts().Update(ctx, previous); err != nil {
			return err
		}
		return registry.KnownHosts().Create(ctx, replacement)
	})
}

// Delete 删除一条记录。
func (r *knownHostRepository) Delete(ctx context.Context, id model.ID) error {
	result := r.store.database.WithContext(ctx).Delete(&KnownHostRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Known Host 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Known Host 不存在")
	}
	return nil
}

func mapSSHReadError(message string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New(apperror.CodeIONotFound, strings.TrimSuffix(message, "失败")+"不存在")
	}
	return apperror.Wrap(apperror.CodeIOReadFailed, message, err)
}

func sshSessionToRecord(session *model.SSHSession) SSHSessionRecord {
	var credentialID *string
	if session.CredentialID != nil {
		value := session.CredentialID.String()
		credentialID = &value
	}
	return SSHSessionRecord{
		ID: session.ID.String(), Name: session.Name, Host: session.Host, Port: session.Port,
		Username: session.Username, AuthType: session.AuthType.String(), CredentialID: credentialID,
		HostKeyPolicy: session.HostKeyPolicy.String(), Group: session.Group, Favourite: session.Favourite,
		Remark: session.Remark, ConnectTimeoutSec: session.ConnectTimeoutSec,
		HandshakeTimeoutSec: session.HandshakeTimeoutSec, KeepAliveSec: session.KeepAliveSec,
		Compression: session.Compression, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
		OverrideSettings: session.OverrideSettings,
		HostCPUCount:     session.HostSpecs.CPUCount, HostMemoryBytes: session.HostSpecs.MemoryBytes,
		HostDiskBytes: session.HostSpecs.DiskBytes, HostSpecsCollectedAt: copyTime(session.HostSpecs.CollectedAt),
	}
}

func recordToSSHSession(record SSHSessionRecord) model.SSHSession {
	var credentialID *model.ID
	if record.CredentialID != nil {
		value := model.ID(*record.CredentialID)
		credentialID = &value
	}
	return model.SSHSession{
		ID: model.ID(record.ID), Name: record.Name, Host: record.Host, Port: record.Port,
		Username: record.Username, AuthType: enums.SSHAuthType(record.AuthType), CredentialID: credentialID,
		HostKeyPolicy: enums.SSHHostKeyPolicy(record.HostKeyPolicy), Group: record.Group,
		Favourite: record.Favourite, Remark: record.Remark, ConnectTimeoutSec: record.ConnectTimeoutSec,
		HandshakeTimeoutSec: record.HandshakeTimeoutSec, KeepAliveSec: record.KeepAliveSec,
		Compression: record.Compression, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		OverrideSettings: record.OverrideSettings,
		HostSpecs: model.SSHHostSpecs{
			CPUCount: record.HostCPUCount, MemoryBytes: record.HostMemoryBytes,
			DiskBytes: record.HostDiskBytes, CollectedAt: copyTime(record.HostSpecsCollectedAt),
		},
	}
}

// copyTime 复制一个可选时间戳,避免记录与领域对象共享可变状态。
func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func sshCredentialToRecord(credential *model.SSHCredential) SSHCredentialRecord {
	return SSHCredentialRecord{
		ID: credential.ID.String(), AuthType: credential.AuthType.String(),
		Secret: append([]byte(nil), credential.Secret...), Passphrase: append([]byte(nil), credential.Passphrase...),
		CreatedAt: credential.CreatedAt, UpdatedAt: credential.UpdatedAt,
	}
}

func recordToSSHCredential(record SSHCredentialRecord) model.SSHCredential {
	return model.SSHCredential{
		ID: model.ID(record.ID), AuthType: enums.SSHAuthType(record.AuthType),
		Secret: append([]byte(nil), record.Secret...), Passphrase: append([]byte(nil), record.Passphrase...),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func knownHostToRecord(knownHost *model.KnownHost) KnownHostRecord {
	var replacedByID *string
	if knownHost.ReplacedByID != nil {
		value := knownHost.ReplacedByID.String()
		replacedByID = &value
	}
	return KnownHostRecord{
		ID: knownHost.ID.String(), HostIdentifier: knownHost.HostIdentifier, Host: knownHost.Host,
		Port: knownHost.Port, Algorithm: knownHost.Algorithm, PublicKey: append([]byte(nil), knownHost.PublicKey...),
		Fingerprint: knownHost.Fingerprint, FirstSeenAt: knownHost.FirstSeenAt, LastSeenAt: knownHost.LastSeenAt,
		ReplacedAt: knownHost.ReplacedAt, ReplacedByID: replacedByID,
	}
}

func recordToKnownHost(record KnownHostRecord) model.KnownHost {
	var replacedByID *model.ID
	if record.ReplacedByID != nil {
		value := model.ID(*record.ReplacedByID)
		replacedByID = &value
	}
	return model.KnownHost{
		ID: model.ID(record.ID), HostIdentifier: record.HostIdentifier, Host: record.Host,
		Port: record.Port, Algorithm: record.Algorithm, PublicKey: append([]byte(nil), record.PublicKey...),
		Fingerprint: record.Fingerprint, FirstSeenAt: record.FirstSeenAt, LastSeenAt: record.LastSeenAt,
		ReplacedAt: record.ReplacedAt, ReplacedByID: replacedByID,
	}
}
