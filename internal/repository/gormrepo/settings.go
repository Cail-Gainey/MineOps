package gormrepo

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// SettingsRecord is one encrypted, versioned settings category payload.
type SettingsRecord struct {
	Category      string `gorm:"primaryKey;size:32"`
	SchemaVersion int
	Payload       []byte
	UpdatedAt     time.Time
}

type settingsRepository struct {
	store *Store
}

func (r *settingsRepository) Load(ctx context.Context) (map[enums.SettingsCategory][]byte, error) {
	var records []SettingsRecord
	if err := r.store.database.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "加载 Settings 失败", err)
	}
	result := make(map[enums.SettingsCategory][]byte, len(records))
	for _, record := range records {
		category := enums.SettingsCategory(record.Category)
		if category.Valid() {
			result[category] = append([]byte(nil), record.Payload...)
		}
	}
	return result, nil
}

func (r *settingsRepository) Save(ctx context.Context, category enums.SettingsCategory, schemaVersion int, payload []byte) error {
	if !category.Valid() || schemaVersion <= 0 || len(payload) == 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Settings Category 保存参数无效")
	}
	record := SettingsRecord{Category: category.String(), SchemaVersion: schemaVersion, Payload: append([]byte(nil), payload...), UpdatedAt: time.Now().UTC()}
	if err := r.store.database.WithContext(ctx).Save(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "保存 Settings Category 失败", err)
	}
	return nil
}

func (r *settingsRepository) Delete(ctx context.Context, category enums.SettingsCategory) error {
	if !category.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Settings Category 无效")
	}
	if err := r.store.database.WithContext(ctx).Delete(&SettingsRecord{}, "category = ?", category.String()).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "重置 Settings Category 失败", err)
	}
	return nil
}
