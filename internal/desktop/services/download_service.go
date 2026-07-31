package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// ProxyCredentialInput 承载只写的代理认证字段。
type ProxyCredentialInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ProxyCredentialResult 承载非机密的凭据状态或稳定错误。
type ProxyCredentialResult struct {
	Status service.ProxyCredentialStatus `json:"status"`
	Error  *apperror.DTO                 `json:"error,omitempty"`
}

// DownloadSourceStatusResult 承载下载源实时探测结果或稳定错误。
type DownloadSourceStatusResult struct {
	Sources []service.DownloadSourceStatus `json:"sources"`
	Error   *apperror.DTO                  `json:"error,omitempty"`
}

// DownloadCacheStatusResult 承载当前缓存占用或稳定错误。
type DownloadCacheStatusResult struct {
	Status service.DownloadCacheStatus `json:"status"`
	Error  *apperror.DTO               `json:"error,omitempty"`
}

// DownloadService 对外暴露加密代理凭据、源探测、HTTP 重载与缓存管理。
type DownloadService struct {
	manager *service.DownloadManager
	logger  *applog.Logger
}

// NewDownloadService 创建桌面侧的下载设置门面。
func NewDownloadService(manager *service.DownloadManager, logger *applog.Logger) *DownloadService {
	return &DownloadService{manager: manager, logger: logger}
}

// GetProxyCredentialStatus 返回非机密的代理凭据元信息。
func (s *DownloadService) GetProxyCredentialStatus(ctx context.Context) (result ProxyCredentialResult) {
	defer s.recoverProxy(ctx, "DownloadService.GetProxyCredentialStatus", &result)
	status, err := s.manager.ProxyCredentialStatus(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ProxyCredentialResult{Error: &dto}
	}
	return ProxyCredentialResult{Status: status}
}

// SaveProxyCredential 仅把口令字节写入 SQLCipher 加密的 SQLite。
func (s *DownloadService) SaveProxyCredential(ctx context.Context, input ProxyCredentialInput) (result ProxyCredentialResult) {
	defer s.recoverProxy(ctx, "DownloadService.SaveProxyCredential", &result)
	password := []byte(input.Password)
	status, err := s.manager.SaveProxyCredential(ctx, input.Username, password)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ProxyCredentialResult{Error: &dto}
	}
	return ProxyCredentialResult{Status: status}
}

// ClearProxyCredential 删除加密凭据并重载 HTTP 传输层。
func (s *DownloadService) ClearProxyCredential(ctx context.Context) (result ActionResult) {
	defer s.recoverAction(ctx, &result)
	if err := s.manager.ClearProxyCredential(ctx); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// CheckSources 按当前代理设置执行实时连通性探测。
func (s *DownloadService) CheckSources(ctx context.Context) (result DownloadSourceStatusResult) {
	defer s.recoverSources(ctx, &result)
	return DownloadSourceStatusResult{Sources: s.manager.CheckSources(ctx)}
}

// GetCacheStatus 返回本地缓存体积与已配置容量。
func (s *DownloadService) GetCacheStatus(ctx context.Context) (result DownloadCacheStatusResult) {
	defer s.recoverCache(ctx, &result)
	status, err := s.manager.CacheStatus()
	if err != nil {
		dto := apperror.ToDTO(err)
		return DownloadCacheStatusResult{Error: &dto}
	}
	return DownloadCacheStatusResult{Status: status}
}

// ClearCache 删除本地缓存构件,不改动任何设置。
func (s *DownloadService) ClearCache(ctx context.Context) (result ActionResult) {
	defer s.recoverAction(ctx, &result)
	if err := s.manager.ClearCache(); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *DownloadService) recoverProxy(ctx context.Context, boundary string, result *ProxyCredentialResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *DownloadService) recoverSources(ctx context.Context, result *DownloadSourceStatusResult) {
	var err error
	apperror.Recover(ctx, s.logger, "DownloadService.CheckSources", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *DownloadService) recoverCache(ctx context.Context, result *DownloadCacheStatusResult) {
	var err error
	apperror.Recover(ctx, s.logger, "DownloadService.GetCacheStatus", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *DownloadService) recoverAction(ctx context.Context, result *ActionResult) {
	var err error
	apperror.Recover(ctx, s.logger, "DownloadService.Action", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
