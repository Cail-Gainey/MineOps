package sqlcipher

import (
	"context"
	"sort"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"gorm.io/gorm"
)

type migrationRecord struct {
	Version   int `gorm:"primaryKey;autoIncrement:false"`
	Name      string
	AppliedAt time.Time
}

// Migration 是一个不可变的、带版本的 GORM 迁移。
type Migration struct {
	Version int
	Name    string
	Apply   func(*gorm.DB) error
}

// MigrationRunner 在事务内应用待执行的 Go 迁移并记录其版本。
type MigrationRunner struct {
	database   *gorm.DB
	migrations []Migration
}

// NewMigrationRunner 在校验迁移版本唯一且为正之后创建执行器。
func NewMigrationRunner(database *gorm.DB, migrations []Migration) (*MigrationRunner, error) {
	if database == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Migration 数据库不能为空")
	}
	copied := append([]Migration(nil), migrations...)
	sort.Slice(copied, func(left, right int) bool { return copied[left].Version < copied[right].Version })
	seen := make(map[int]struct{}, len(copied))
	for _, migration := range copied {
		if migration.Version <= 0 || migration.Name == "" || migration.Apply == nil {
			return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Migration 定义无效")
		}
		if _, exists := seen[migration.Version]; exists {
			return nil, apperror.New(apperror.CodeValidationConflict, "Migration 版本重复")
		}
		seen[migration.Version] = struct{}{}
	}
	return &MigrationRunner{database: database, migrations: copied}, nil
}

// Run 在各自独立的事务中应用每个待执行迁移,遇到首个失败即停止。
func (r *MigrationRunner) Run(ctx context.Context) error {
	if err := r.database.WithContext(ctx).AutoMigrate(&migrationRecord{}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Migration 版本表失败", err)
	}
	var applied []migrationRecord
	if err := r.database.WithContext(ctx).Find(&applied).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "读取 Migration 版本失败", err)
	}
	appliedVersions := make(map[int]struct{}, len(applied))
	for _, record := range applied {
		appliedVersions[record.Version] = struct{}{}
	}
	for _, migration := range r.migrations {
		if _, exists := appliedVersions[migration.Version]; exists {
			continue
		}
		if err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
			if err := migration.Apply(transaction); err != nil {
				return err
			}
			return transaction.Create(&migrationRecord{
				Version:   migration.Version,
				Name:      migration.Name,
				AppliedAt: time.Now().UTC(),
			}).Error
		}); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "执行数据库 Migration 失败", err).WithDetails(map[string]any{
				"version": migration.Version,
				"name":    migration.Name,
			})
		}
	}
	return nil
}

// CurrentVersion 返回已成功应用的最大迁移版本号。
func (r *MigrationRunner) CurrentVersion(ctx context.Context) (int, error) {
	var record migrationRecord
	result := r.database.WithContext(ctx).Order("version desc").Limit(1).Find(&record)
	if result.Error != nil {
		return 0, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Migration 版本失败", result.Error)
	}
	return record.Version, nil
}
