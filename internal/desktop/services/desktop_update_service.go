package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// DesktopUpdateStatusResult 承载一份桌面更新状态或稳定错误。
type DesktopUpdateStatusResult struct {
	Status service.DesktopUpdateStatus `json:"status"`
	Error  *apperror.DTO               `json:"error,omitempty"`
}

// DesktopUpdateService 对外暴露桌面发行版检查、准备、取消与重启安装。
type DesktopUpdateService struct {
	manager *service.DesktopUpdateManager
	logger  *applog.Logger
}

// NewDesktopUpdateService 创建桌面发行版检查门面。
func NewDesktopUpdateService(manager *service.DesktopUpdateManager, logger *applog.Logger) *DesktopUpdateService {
	return &DesktopUpdateService{manager: manager, logger: logger}
}

// Status 返回最近一次桌面发行版检查状态,不发起网络请求。
func (s *DesktopUpdateService) Status(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Status", &result)
	return DesktopUpdateStatusResult{Status: s.manager.Status()}
}

// Check 立即解析已配置的 Stable/Beta 通道,不安装任何更新。
func (s *DesktopUpdateService) Check(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Check", &result)
	status, err := s.manager.CheckNow(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return DesktopUpdateStatusResult{Status: status, Error: &dto}
	}
	return DesktopUpdateStatusResult{Status: status}
}

// Download 下载、校验并准备好可用的桌面更新。
func (s *DesktopUpdateService) Download(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Download", &result)
	status, err := s.manager.DownloadNow(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return DesktopUpdateStatusResult{Status: status, Error: &dto}
	}
	return DesktopUpdateStatusResult{Status: status}
}

// Cancel 取消进行中的桌面更新下载。
func (s *DesktopUpdateService) Cancel(ctx context.Context) (result DesktopUpdateStatusResult) {
	defer s.recover(ctx, "DesktopUpdateService.Cancel", &result)
	s.manager.CancelDownload()
	return DesktopUpdateStatusResult{Status: s.manager.Status()}
}

// Restart 在可选的未保存内容确认后,重启进入已校验的更新。
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
