package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// DiagnosticExportResult contains one completed diagnostic package or a stable desktop error.
type DiagnosticExportResult struct {
	Package *service.DiagnosticPackage `json:"package,omitempty"`
	Error   *apperror.DTO              `json:"error,omitempty"`
}

// DiagnosticService exposes privacy-bounded diagnostic archive export to Wails.
type DiagnosticService struct {
	manager *service.DiagnosticManager
	logger  *applog.Logger
}

// NewDiagnosticService creates the desktop diagnostic export facade.
func NewDiagnosticService(manager *service.DiagnosticManager, logger *applog.Logger) *DiagnosticService {
	return &DiagnosticService{manager: manager, logger: logger}
}

// Export writes one redacted diagnostic archive to the path selected by the user.
func (s *DiagnosticService) Export(ctx context.Context, destination string) (result DiagnosticExportResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "DiagnosticService.Export", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	packageResult, err := s.manager.Export(ctx, destination)
	if err != nil {
		dto := apperror.ToDTO(err)
		return DiagnosticExportResult{Error: &dto}
	}
	return DiagnosticExportResult{Package: &packageResult}
}
