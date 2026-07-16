package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// DesktopUpdateStatusResult contains one Desktop update state or a stable error.
type DesktopUpdateStatusResult struct {
	Status service.DesktopUpdateStatus `json:"status"`
	Error  *apperror.DTO               `json:"error,omitempty"`
}

// DesktopUpdateService exposes Desktop release checks, preparation, cancellation and restart application.
type DesktopUpdateService struct {
	manager *service.DesktopUpdateManager
	logger  *applog.Logger
}

// NewDesktopUpdateService creates the Desktop release-check facade.
func NewDesktopUpdateService(manager *service.DesktopUpdateManager, logger *applog.Logger) *DesktopUpdateService {
	return &DesktopUpdateService{manager: manager, logger: logger}
}

// Status returns the latest Desktop release-check state without network activity.
func (s *DesktopUpdateService) Status(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Status", &result)
	return DesktopUpdateStatusResult{Status: s.manager.Status()}
}

// Check immediately resolves the configured Stable/Beta channel without installing an update.
func (s *DesktopUpdateService) Check(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Check", &result)
	status, err := s.manager.CheckNow(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return DesktopUpdateStatusResult{Status: status, Error: &dto}
	}
	return DesktopUpdateStatusResult{Status: status}
}

// Download downloads, verifies and prepares the available Desktop update.
func (s *DesktopUpdateService) Download(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Download", &result)
	status, err := s.manager.DownloadNow(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return DesktopUpdateStatusResult{Status: status, Error: &dto}
	}
	return DesktopUpdateStatusResult{Status: status}
}

// Cancel cancels the active Desktop update download.
func (s *DesktopUpdateService) Cancel(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Cancel", &result)
	s.manager.CancelDownload()
	return DesktopUpdateStatusResult{Status: s.manager.Status()}
}

// Restart restarts MineOps into the verified update after optional dirty-state confirmation.
func (s *DesktopUpdateService) Restart(ctx context.Context, discardUnsaved bool) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Restart", &result)
	status, err := s.manager.RestartAndApply(ctx, discardUnsaved)
	if err != nil {
		dto := apperror.ToDTO(err)
		return DesktopUpdateStatusResult{Status: status, Error: &dto}
	}
	return DesktopUpdateStatusResult{Status: status}
}

func (s *DesktopUpdateService) recover(ctx context.Context, boundary string, result *DesktopUpdateStatusResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
