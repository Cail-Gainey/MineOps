package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"gorm.io/gorm"
)

// ProxyCredentialRecord stores proxy authentication only inside SQLCipher-encrypted SQLite.
type ProxyCredentialRecord struct {
	ID        string `gorm:"primaryKey;size:36"`
	Username  string `gorm:"size:255"`
	Password  []byte
	CreatedAt time.Time
	UpdatedAt time.Time `gorm:"index"`
}

type proxyCredentialRepository struct{ database *gorm.DB }

func (r *proxyCredentialRepository) Create(ctx context.Context, credential *model.ProxyCredential) error {
	if credential == nil || len(credential.Password) == 0 {
		return apperror.New(apperror.CodeValidationRequired, "Proxy Credential 不能为空")
	}
	if err := r.database.WithContext(ctx).Create(&ProxyCredentialRecord{
		ID: credential.ID.String(), Username: credential.Username, Password: append([]byte(nil), credential.Password...),
		CreatedAt: credential.CreatedAt, UpdatedAt: credential.UpdatedAt,
	}).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Proxy Credential 失败", err)
	}
	return nil
}

func (r *proxyCredentialRepository) Update(ctx context.Context, credential *model.ProxyCredential) error {
	if credential == nil || len(credential.Password) == 0 {
		return apperror.New(apperror.CodeValidationRequired, "Proxy Credential 不能为空")
	}
	result := r.database.WithContext(ctx).Model(&ProxyCredentialRecord{}).Where("id = ?", credential.ID.String()).Updates(map[string]any{
		"username": credential.Username, "password": append([]byte(nil), credential.Password...), "updated_at": credential.UpdatedAt,
	})
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Proxy Credential 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Proxy Credential 不存在")
	}
	return nil
}

func (r *proxyCredentialRepository) Get(ctx context.Context, id model.ID) (*model.ProxyCredential, error) {
	var record ProxyCredentialRecord
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.New(apperror.CodeIONotFound, "Proxy Credential 不存在")
		}
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Proxy Credential 失败", err)
	}
	return &model.ProxyCredential{
		ID: model.ID(record.ID), Username: record.Username, Password: append([]byte(nil), record.Password...),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func (r *proxyCredentialRepository) Delete(ctx context.Context, id model.ID) error {
	result := r.database.WithContext(ctx).Delete(&ProxyCredentialRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Proxy Credential 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Proxy Credential 不存在")
	}
	return nil
}
