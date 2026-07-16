package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// LogStatusResult contains runtime log status or a stable desktop error.
type LogStatusResult struct {
	Status *service.LogStatus `json:"status,omitempty"`
	Error  *apperror.DTO      `json:"error,omitempty"`
}

// LoggingService exposes actual runtime log paths and safe cleanup actions.
type LoggingService struct {
	manager *service.LogManager
	logger  *applog.Logger
}

// NewLoggingService creates the desktop logging facade.
func NewLoggingService(manager *service.LogManager, logger *applog.Logger) *LoggingService {
	return &LoggingService{manager: manager, logger: logger}
}

// GetStatus returns the active log location and bounded usage.
func (s *LoggingService) GetStatus(ctx context.Context) (result LogStatusResult) {
	defer s.recover(ctx, "LoggingService.GetStatus", &result)
	status, err := s.manager.Status()
	if err != nil {
		dto := apperror.ToDTO(err)
		return LogStatusResult{Error: &dto}
	}
	return LogStatusResult{Status: &status}
}

// ClearArchived removes rotated logs while preserving the active process file.
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

// OpenDirectory opens the actual runtime log directory.
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
