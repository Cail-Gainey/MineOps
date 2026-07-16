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

// Migration is one immutable versioned GORM migration.
type Migration struct {
	Version int
	Name    string
	Apply   func(*gorm.DB) error
}

// MigrationRunner applies pending Go migrations transactionally and records their versions.
type MigrationRunner struct {
	database   *gorm.DB
	migrations []Migration
}

// NewMigrationRunner creates a runner after validating unique positive migration versions.
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

// Run applies every pending migration in an isolated transaction and stops on the first failure.
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

// CurrentVersion returns the greatest successfully applied migration version.
func (r *MigrationRunner) CurrentVersion(ctx context.Context) (int, error) {
	var record migrationRecord
	result := r.database.WithContext(ctx).Order("version desc").Limit(1).Find(&record)
	if result.Error != nil {
		return 0, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Migration 版本失败", result.Error)
	}
	return record.Version, nil
}
