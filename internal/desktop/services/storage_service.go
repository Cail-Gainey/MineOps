package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// StorageResult contains storage state, a completed backup, or a stable desktop error.
type StorageResult struct {
	Status *service.StorageStatus `json:"status,omitempty"`
	Backup *sqlcipher.BackupInfo  `json:"backup,omitempty"`
	Error  *apperror.DTO          `json:"error,omitempty"`
}

// StorageService exposes encrypted database status, backup, restore, and key-rotation workflows.
type StorageService struct {
	manager *service.StorageManager
	logger  *applog.Logger
}

// NewStorageService creates the desktop storage and security facade.
func NewStorageService(manager *service.StorageManager, logger *applog.Logger) *StorageService {
	return &StorageService{manager: manager, logger: logger}
}

// GetStatus returns the current encrypted database and pending maintenance state.
func (s *StorageService) GetStatus(ctx context.Context) (result StorageResult) {
	defer s.recover(ctx, "StorageService.GetStatus", &result)
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Status: &status}
}

// CreateBackup creates one consistent online .mineops-backup file.
func (s *StorageService) CreateBackup(ctx context.Context, destination string) (result StorageResult) {
	defer s.recover(ctx, "StorageService.CreateBackup", &result)
	backup, err := s.manager.CreateBackup(ctx, destination)
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Backup: &backup}
}

// ScheduleRestore verifies and stages a backup for restoration on the next application start.
func (s *StorageService) ScheduleRestore(ctx context.Context, backupPath string) (result StorageResult) {
	defer s.recover(ctx, "StorageService.ScheduleRestore", &result)
	backup, err := s.manager.ScheduleRestore(backupPath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Backup: &backup, Status: &status}
}

// ScheduleKeyRotation stages a SQLCipher key rotation for the next application start.
func (s *StorageService) ScheduleKeyRotation(ctx context.Context) (result StorageResult) {
	defer s.recover(ctx, "StorageService.ScheduleKeyRotation", &result)
	if err := s.manager.ScheduleKeyRotation(); err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Status: &status}
}

// ScheduleVacuum stages a VACUUM for the next application start to release SQLite free pages.
func (s *StorageService) ScheduleVacuum(ctx context.Context) (result StorageResult) {
	defer s.recover(ctx, "StorageService.ScheduleVacuum", &result)
	if err := s.manager.ScheduleVacuum(); err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Status: &status}
}

// ScheduleDatabaseReset stages deleting both databases and the stored key for the next application start.
func (s *StorageService) ScheduleDatabaseReset(ctx context.Context) (result StorageResult) {
	defer s.recover(ctx, "StorageService.ScheduleDatabaseReset", &result)
	if err := s.manager.ScheduleDatabaseReset(); err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Status: &status}
}

// CancelPendingMaintenance clears staged restore, key-rotation, and vacuum work.
func (s *StorageService) CancelPendingMaintenance(ctx context.Context) (result StorageResult) {
	defer s.recover(ctx, "StorageService.CancelPendingMaintenance", &result)
	if err := s.manager.CancelPendingMaintenance(); err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Status: &status}
}

// OpenDataDirectory opens the controlled MineOps data directory.
func (s *StorageService) OpenDataDirectory(ctx context.Context) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "StorageService.OpenDataDirectory", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.OpenDataDirectory(ctx); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *StorageService) recover(ctx context.Context, boundary string, result *StorageResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
