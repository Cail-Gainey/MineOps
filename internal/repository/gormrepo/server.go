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

// ServerRecord 是一台 Minecraft Server 在加密 SQLite 中的表示。
type ServerRecord struct {
	ID               string   `gorm:"primaryKey;size:36"`
	SSHSessionID     string   `gorm:"uniqueIndex:idx_server_ssh_name,priority:1;uniqueIndex:idx_server_ssh_path,priority:1;index;size:36"`
	JavaRuntimeID    *string  `gorm:"index;size:36"`
	Name             string   `gorm:"uniqueIndex:idx_server_ssh_name,priority:2;size:160"`
	DirectoryName    string   `gorm:"size:100"`
	Type             string   `gorm:"index;size:32"`
	Version          string   `gorm:"size:80"`
	RemotePath       string   `gorm:"uniqueIndex:idx_server_ssh_path,priority:2;size:1024"`
	Group            string   `gorm:"index;size:120"`
	Tags             []string `gorm:"serializer:json"`
	Favourite        bool     `gorm:"index"`
	State            string   `gorm:"index;size:32"`
	XmsMiB           int
	XmxMiB           int
	JVMArguments     []string `gorm:"serializer:json"`
	JarPath          string   `gorm:"size:512"`
	WorkingDirectory string   `gorm:"size:1024"`
	ServerArguments  []string `gorm:"serializer:json"`
	FirewallPolicy   string   `gorm:"size:24"`
	EULAAccepted     bool
	CreatedAt        time.Time
	UpdatedAt        time.Time  `gorm:"index"`
	DeletedAt        *time.Time `gorm:"index"`
}

// TableName 让表名契约与跨仓储的引用查询保持一致。
func (ServerRecord) TableName() string { return "server_records" }

type minecraftServerRepository struct{ database *gorm.DB }

// Create 新增一台 Minecraft Server 登记。
func (r *minecraftServerRepository) Create(ctx context.Context, server *model.MinecraftServer) error {
	if server == nil {
		return apperror.New(apperror.CodeValidationRequired, "Minecraft Server 不能为空")
	}
	if err := server.Validate(); err != nil {
		return err
	}
	record := serverToRecord(server)
	if err := r.database.WithContext(ctx).Create(&record).Error; err != nil {
		return minecraftServerWriteError("创建 Minecraft Server 失败", err)
	}
	return nil
}

// Update 更新一台 Minecraft Server 登记。
func (r *minecraftServerRepository) Update(ctx context.Context, server *model.MinecraftServer) error {
	if server == nil {
		return apperror.New(apperror.CodeValidationRequired, "Minecraft Server 不能为空")
	}
	if err := server.Validate(); err != nil {
		return err
	}
	record := serverToRecord(server)
	result := r.database.WithContext(ctx).Model(&ServerRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return minecraftServerWriteError("更新 Minecraft Server 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Minecraft Server 不存在")
	}
	return nil
}

// Get 按 ID 返回一台 Server,可选择是否允许读取已软删除项。
func (r *minecraftServerRepository) Get(ctx context.Context, id model.ID, includeDeleted bool) (*model.MinecraftServer, error) {
	var record ServerRecord
	query := r.database.WithContext(ctx).Where("id = ?", id.String())
	if !includeDeleted {
		query = query.Where("deleted_at IS NULL")
	}
	if err := query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.New(apperror.CodeIONotFound, "Minecraft Server 不存在")
		}
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Minecraft Server 失败", err)
	}
	server := recordToServer(record)
	return &server, nil
}

// List 按查询条件分页列出 Minecraft Server。
func (r *minecraftServerRepository) List(ctx context.Context, query repository.MinecraftServerQuery) ([]model.MinecraftServer, error) {
	database := r.database.WithContext(ctx).Order("favourite desc, name asc")
	if !query.IncludeDeleted {
		database = database.Where("deleted_at IS NULL")
	}
	if query.SSHSessionID != "" {
		database = database.Where("ssh_session_id = ?", query.SSHSessionID.String())
	}
	if query.Group != "" {
		database = database.Where("`group` = ?", query.Group)
	}
	if query.State != "" {
		database = database.Where("state = ?", query.State.String())
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		pattern := "%" + search + "%"
		database = database.Where("name LIKE ? OR remote_path LIKE ? OR version LIKE ?", pattern, pattern, pattern)
	}
	if query.Tag != "" {
		database = database.Where("tags LIKE ?", "%\""+query.Tag+"\"%")
	}
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	var records []ServerRecord
	if err := database.Limit(limit).Offset(query.Offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Minecraft Server 列表失败", err)
	}
	result := make([]model.MinecraftServer, len(records))
	for index, record := range records {
		result[index] = recordToServer(record)
	}
	return result, nil
}

// SoftDelete 软删除一台 Server 并记录删除时间,保留远端文件。
func (r *minecraftServerRepository) SoftDelete(ctx context.Context, id model.ID, clock model.Clock) error {
	if clock == nil {
		return apperror.New(apperror.CodeValidationRequired, "Clock 不能为空")
	}
	now := clock.Now().UTC()
	result := r.database.WithContext(ctx).Model(&ServerRecord{}).Where("id = ? AND deleted_at IS NULL", id.String()).Updates(map[string]any{
		"deleted_at": now, "state": enums.LifecycleDeleted.String(), "updated_at": now,
	})
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "软删除 Minecraft Server 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Minecraft Server 不存在")
	}
	return nil
}

// Restore 恢复一台已软删除的 Server。
func (r *minecraftServerRepository) Restore(ctx context.Context, id model.ID, clock model.Clock) error {
	if clock == nil {
		return apperror.New(apperror.CodeValidationRequired, "Clock 不能为空")
	}
	result := r.database.WithContext(ctx).Model(&ServerRecord{}).Where("id = ? AND deleted_at IS NOT NULL", id.String()).Updates(map[string]any{
		"deleted_at": nil, "state": enums.LifecycleStopped.String(), "updated_at": clock.Now().UTC(),
	})
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "恢复 Minecraft Server 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "已删除 Minecraft Server 不存在")
	}
	return nil
}

// HardDelete 永久删除一台 Server 的登记行。
func (r *minecraftServerRepository) HardDelete(ctx context.Context, id model.ID) error {
	result := r.database.WithContext(ctx).Delete(&ServerRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "硬删除 Minecraft Server 记录失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Minecraft Server 不存在")
	}
	return nil
}

func serverToRecord(server *model.MinecraftServer) ServerRecord {
	var javaRuntimeID *string
	if server.JavaRuntimeID != nil {
		value := server.JavaRuntimeID.String()
		javaRuntimeID = &value
	}
	return ServerRecord{
		ID: server.ID.String(), SSHSessionID: server.SSHSessionID.String(), JavaRuntimeID: javaRuntimeID,
		Name: server.Name, DirectoryName: server.DirectoryName, Type: server.Type.String(), Version: server.Version,
		RemotePath: server.RemotePath, Group: server.Group, Tags: append([]string(nil), server.Tags...),
		Favourite: server.Favourite, State: server.State.String(), XmsMiB: server.LaunchProfile.XmsMiB,
		XmxMiB: server.LaunchProfile.XmxMiB, JVMArguments: append([]string(nil), server.LaunchProfile.JVMArguments...),
		JarPath: server.LaunchProfile.JarPath, WorkingDirectory: server.LaunchProfile.WorkingDirectory,
		ServerArguments: append([]string(nil), server.LaunchProfile.ServerArguments...),
		FirewallPolicy:  server.FirewallPolicy.String(), EULAAccepted: server.EULAAccepted,
		CreatedAt: server.CreatedAt, UpdatedAt: server.UpdatedAt, DeletedAt: server.DeletedAt,
	}
}

func recordToServer(record ServerRecord) model.MinecraftServer {
	var javaRuntimeID *model.ID
	if record.JavaRuntimeID != nil {
		value := model.ID(*record.JavaRuntimeID)
		javaRuntimeID = &value
	}
	return model.MinecraftServer{
		ID: model.ID(record.ID), SSHSessionID: model.ID(record.SSHSessionID), JavaRuntimeID: javaRuntimeID,
		Name: record.Name, DirectoryName: record.DirectoryName, Type: enums.MinecraftServerType(record.Type),
		Version: record.Version, RemotePath: record.RemotePath, Group: record.Group,
		Tags: append([]string(nil), record.Tags...), Favourite: record.Favourite, State: enums.LifecycleState(record.State),
		LaunchProfile: model.LaunchProfile{
			XmsMiB: record.XmsMiB, XmxMiB: record.XmxMiB, JVMArguments: append([]string(nil), record.JVMArguments...),
			JarPath: record.JarPath, WorkingDirectory: record.WorkingDirectory,
			ServerArguments: append([]string(nil), record.ServerArguments...),
		},
		FirewallPolicy: enums.FirewallPolicy(record.FirewallPolicy), EULAAccepted: record.EULAAccepted,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, DeletedAt: record.DeletedAt,
	}
}

func minecraftServerWriteError(fallback string, err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicated key") {
		conflict := "同一 SSH Session 下已存在相同名称或远程路径的 Minecraft Server"
		field := "name_or_remote_path"
		if strings.Contains(message, "remote_path") || strings.Contains(message, "idx_server_ssh_path") {
			conflict = "同一 SSH Session 下已存在使用该远程路径的 Minecraft Server"
			field = "remotePath"
		} else if strings.Contains(message, ".name") || strings.Contains(message, "idx_server_ssh_name") {
			conflict = "同一 SSH Session 下已存在同名 Minecraft Server"
			field = "name"
		}
		return apperror.Wrap(apperror.CodeValidationConflict, conflict, err).WithDetails(map[string]any{
			"field": field,
		})
	}
	return apperror.Wrap(apperror.CodeIOWriteFailed, fallback, err)
}
