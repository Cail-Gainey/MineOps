package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// LogStatusResult 承载运行期日志状态或稳定的桌面错误。
type LogStatusResult struct {
	Status *service.LogStatus `json:"status,omitempty"`
	Error  *apperror.DTO      `json:"error,omitempty"`
}

// LoggingService 对外暴露真实的运行期日志路径与安全清理操作。
type LoggingService struct {
	manager *service.LogManager
	logger  *applog.Logger
}

// NewLoggingService 创建桌面侧的日志门面。
func NewLoggingService(manager *service.LogManager, logger *applog.Logger) *LoggingService {
	return &LoggingService{manager: manager, logger: logger}
}

// GetStatus 返回当前日志位置与有界的占用量。
func (s *LoggingService) GetStatus(ctx context.Context) (result LogStatusResult) {
	defer s.recover(ctx, "LoggingService.GetStatus", &result)
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return LogStatusResult{Error: &dto}
	}
	return LogStatusResult{Status: &status}
}

// ClearArchived 删除已轮转的日志,保留当前进程正在写的文件。
func (s *LoggingService) ClearArchived(ctx context.Context) (result LogStatusResult) {
	defer s.recover(ctx, "LoggingService.ClearArchived", &result)
	if err := s.manager.ClearArchived(); err != nil {
		dto := apperror.ToDTO(err)
		return LogStatusResult{Error: &dto}
	}
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return LogStatusResult{Error: &dto}
	}
	return LogStatusResult{Status: &status}
}

// OpenDirectory 打开真实的运行期日志目录。
func (s *LoggingService) OpenDirectory(ctx context.Context) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "LoggingService.OpenDirectory", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.OpenDirectory(ctx); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *LoggingService) recover(ctx context.Context, boundary string, result *LogStatusResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
