package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// StorageResult 承载存储状态、已完成的备份或稳定的桌面错误。
type StorageResult struct {
	Status *service.StorageStatus `json:"status,omitempty"`
	Backup *sqlcipher.BackupInfo  `json:"backup,omitempty"`
	Error  *apperror.DTO          `json:"error,omitempty"`
}

// StorageService 对外暴露加密数据库状态、备份、恢复与密钥轮换流程。
type StorageService struct {
	manager *service.StorageManager
	logger  *applog.Logger
}

// NewStorageService 创建桌面侧的存储与安全门面。
func NewStorageService(manager *service.StorageManager, logger *applog.Logger) *StorageService {
	return &StorageService{manager: manager, logger: logger}
}

// GetStatus 返回当前加密数据库状态与已排队的维护任务。
func (s *StorageService) GetStatus(ctx context.Context) (result StorageResult) {
	defer s.recover(ctx, "StorageService.GetStatus", &result)
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Status: &status}
}

// CreateBackup 在线创建一份一致的 .mineops-backup 备份文件。
func (s *StorageService) CreateBackup(ctx context.Context, destination string) (result StorageResult) {
	defer s.recover(ctx, "StorageService.CreateBackup", &result)
	backup, err := s.manager.CreateBackup(ctx, destination)
	if err != nil {
		dto := apperror.ToDTO(err)
		return StorageResult{Error: &dto}
	}
	return StorageResult{Backup: &backup}
}

// ScheduleRestore 校验备份并排队到下次启动时恢复。
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

// ScheduleKeyRotation 排队一次下次启动执行的 SQLCipher 密钥轮换。
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

// ScheduleVacuum 排队一次下次启动执行的 VACUUM,释放 SQLite 空闲页占用的文件空间。
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

// ScheduleDatabaseReset 排队一次下次启动执行的清空数据库:删除两个库文件与系统密钥。
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

// CancelPendingMaintenance 取消全部已排队的下次启动维护任务。
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

// OpenDataDirectory 打开受控的 MineOps 数据目录。
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
