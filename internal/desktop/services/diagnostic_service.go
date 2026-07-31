package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// DiagnosticExportResult 承载一份已完成的诊断包或稳定的桌面错误。
type DiagnosticExportResult struct {
	Package *service.DiagnosticPackage `json:"package,omitempty"`
	Error   *apperror.DTO              `json:"error,omitempty"`
}

// DiagnosticService 向 Wails 暴露隐私受限的诊断归档导出。
type DiagnosticService struct {
	manager *service.DiagnosticManager
	logger  *applog.Logger
}

// NewDiagnosticService 创建桌面侧的诊断导出门面。
func NewDiagnosticService(manager *service.DiagnosticManager, logger *applog.Logger) *DiagnosticService {
	return &DiagnosticService{manager: manager, logger: logger}
}

// Export 把一份脱敏诊断归档写入用户选择的路径。
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
