package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
)

// JavaRuntimeRecord is the encrypted SQLite representation of a remote Java installation.
type JavaRuntimeRecord struct {
	ID           string `gorm:"primaryKey;size:36"`
	SSHSessionID string `gorm:"uniqueIndex:idx_java_runtime_path,priority:1;index;size:36"`
	Version      string `gorm:"size:80"`
	MajorVersion int    `gorm:"index"`
	Vendor       string `gorm:"size:160"`
	Architecture string `gorm:"size:80"`
	Source       string `gorm:"index;size:32"`
	InstallPath  string `gorm:"uniqueIndex:idx_java_runtime_path,priority:2;size:1024"`
	JavaHome     string `gorm:"size:1024"`
	Managed      bool   `gorm:"index"`
	IsDefault    bool   `gorm:"index"`
	Reusable     bool   `gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time `gorm:"index"`
}

type javaRuntimeRepository struct{ store *Store }

func (r *javaRuntimeRepository) Create(ctx context.Context, javaRuntime *model.JavaRuntime) error {
	if javaRuntime == nil {
		return apperror.New(apperror.CodeValidationRequired, "Java Runtime 不能为空")
	}
	if err := javaRuntime.Validate(); err != nil {
		return err
	}
	record := javaRuntimeToRecord(javaRuntime)
	if err := r.store.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Java Runtime 失败", err)
	}
	return nil
}

func (r *javaRuntimeRepository) Update(ctx context.Context, javaRuntime *model.JavaRuntime) error {
	if javaRuntime == nil {
		return apperror.New(apperror.CodeValidationRequired, "Java Runtime 不能为空")
	}
	if err := javaRuntime.Validate(); err != nil {
		return err
	}
	record := javaRuntimeToRecord(javaRuntime)
	result := r.store.database.WithContext(ctx).Model(&JavaRuntimeRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Java Runtime 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Java Runtime 不存在")
	}
	return nil
}

func (r *javaRuntimeRepository) Get(ctx context.Context, id model.ID) (*model.JavaRuntime, error) {
	var record JavaRuntimeRecord
	if err := r.store.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		return nil, mapJavaReadError(err)
	}
	javaRuntime := recordToJavaRuntime(record)
	return &javaRuntime, nil
}

func (r *javaRuntimeRepository) GetByPath(ctx context.Context, sshSessionID model.ID, installPath string) (*model.JavaRuntime, error) {
	var record JavaRuntimeRecord
	if err := r.store.database.WithContext(ctx).Where("ssh_session_id = ? AND install_path = ?", sshSessionID.String(), installPath).First(&record).Error; err != nil {
		return nil, mapJavaReadError(err)
	}
	javaRuntime := recordToJavaRuntime(record)
	return &javaRuntime, nil
}

func (r *javaRuntimeRepository) List(ctx context.Context, query repository.JavaRuntimeQuery) ([]model.JavaRuntime, error) {
	database := r.store.database.WithContext(ctx).Order("is_default desc, major_version desc, install_path asc")
	if query.SSHSessionID != "" {
		database = database.Where("ssh_session_id = ?", query.SSHSessionID.String())
	}
	if query.MajorVersion > 0 {
		database = database.Where("major_version = ?", query.MajorVersion)
	}
	if query.Source != "" {
		database = database.Where("source = ?", string(query.Source))
	}
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	var records []JavaRuntimeRecord
	if err := database.Limit(limit).Offset(query.Offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Java Runtime 列表失败", err)
	}
	result := make([]model.JavaRuntime, len(records))
	for index, record := range records {
		result[index] = recordToJavaRuntime(record)
	}
	return result, nil
}

func (r *javaRuntimeRepository) SetDefault(ctx context.Context, sshSessionID, id model.ID) error {
	return r.store.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Model(&JavaRuntimeRecord{}).Where("ssh_session_id = ?", sshSessionID.String()).Update("is_default", false).Error; err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "清除 Java Runtime 默认项失败", err)
		}
		result := transaction.Model(&JavaRuntimeRecord{}).Where("id = ? AND ssh_session_id = ?", id.String(), sshSessionID.String()).Update("is_default", true)
		if result.Error != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "设置 Java Runtime 默认项失败", result.Error)
		}
		if result.RowsAffected != 1 {
			return apperror.New(apperror.CodeIONotFound, "Java Runtime 不存在")
		}
		return nil
	})
}

func (r *javaRuntimeRepository) CountServerReferences(ctx context.Context, id model.ID) (int64, error) {
	if !r.store.database.Migrator().HasTable("server_records") {
		return 0, nil
	}
	var count int64
	if err := r.store.database.WithContext(ctx).Table("server_records").Where("java_runtime_id = ?", id.String()).Count(&count).Error; err != nil {
		return 0, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Java Runtime 引用失败", err)
	}
	return count, nil
}

func (r *javaRuntimeRepository) Delete(ctx context.Context, id model.ID) error {
	result := r.store.database.WithContext(ctx).Delete(&JavaRuntimeRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Java Runtime 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Java Runtime 不存在")
	}
	return nil
}

func mapJavaReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New(apperror.CodeIONotFound, "Java Runtime 不存在")
	}
	return apperror.Wrap(apperror.CodeIOReadFailed, "查询 Java Runtime 失败", err)
}

func javaRuntimeToRecord(javaRuntime *model.JavaRuntime) JavaRuntimeRecord {
	return JavaRuntimeRecord{
		ID: javaRuntime.ID.String(), SSHSessionID: javaRuntime.SSHSessionID.String(), Version: javaRuntime.Version,
		MajorVersion: javaRuntime.MajorVersion, Vendor: javaRuntime.Vendor, Architecture: javaRuntime.Architecture,
		Source: string(javaRuntime.Source), InstallPath: javaRuntime.InstallPath, JavaHome: javaRuntime.JavaHome,
		Managed: javaRuntime.Managed, IsDefault: javaRuntime.Default, Reusable: javaRuntime.Reusable,
		CreatedAt: javaRuntime.CreatedAt, UpdatedAt: javaRuntime.UpdatedAt,
	}
}

func recordToJavaRuntime(record JavaRuntimeRecord) model.JavaRuntime {
	return model.JavaRuntime{
		ID: model.ID(record.ID), SSHSessionID: model.ID(record.SSHSessionID), Version: record.Version,
		MajorVersion: record.MajorVersion, Vendor: record.Vendor, Architecture: record.Architecture,
		Source: model.JavaRuntimeSource(record.Source), InstallPath: record.InstallPath, JavaHome: record.JavaHome,
		Managed: record.Managed, Default: record.IsDefault, Reusable: record.Reusable,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
