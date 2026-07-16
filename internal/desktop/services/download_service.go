package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// ProxyCredentialInput contains write-only proxy authentication fields.
type ProxyCredentialInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ProxyCredentialResult contains non-secret credential status or a stable error.
type ProxyCredentialResult struct {
	Status service.ProxyCredentialStatus `json:"status"`
	Error  *apperror.DTO                 `json:"error,omitempty"`
}

// DownloadSourceStatusResult contains live source checks or a stable error.
type DownloadSourceStatusResult struct {
	Sources []service.DownloadSourceStatus `json:"sources"`
	Error   *apperror.DTO                  `json:"error,omitempty"`
}

// DownloadCacheStatusResult contains current cache usage or a stable error.
type DownloadCacheStatusResult struct {
	Status service.DownloadCacheStatus `json:"status"`
	Error  *apperror.DTO               `json:"error,omitempty"`
}

// DownloadService exposes encrypted proxy credentials, source checks, HTTP reload, and cache management.
type DownloadService struct {
	manager *service.DownloadManager
	logger  *applog.Logger
}

// NewDownloadService creates the desktop download settings facade.
func NewDownloadService(manager *service.DownloadManager, logger *applog.Logger) *DownloadService {
	return &DownloadService{manager: manager, logger: logger}
}

// GetProxyCredentialStatus returns non-secret proxy credential metadata.
func (s *DownloadService) GetProxyCredentialStatus(ctx context.Context) (result ProxyCredentialResult) {
	defer s.recoverProxy(ctx, "DownloadService.GetProxyCredentialStatus", &result)
	status, err := s.manager.ProxyCredentialStatus(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ProxyCredentialResult{Error: &dto}
	}
	return ProxyCredentialResult{Status: status}
}

// SaveProxyCredential writes password bytes only to SQLCipher-encrypted SQLite.
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

// ClearProxyCredential removes the encrypted credential and reloads the HTTP transport.
func (s *DownloadService) ClearProxyCredential(ctx context.Context) (result ActionResult) {
	defer s.recoverAction(ctx, &result)
	if err := s.manager.ClearProxyCredential(ctx); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// CheckSources runs live connectivity checks through current proxy settings.
func (s *DownloadService) CheckSources(ctx context.Context) (result DownloadSourceStatusResult) {
	defer s.recoverSources(ctx, &result)
	return DownloadSourceStatusResult{Sources: s.manager.CheckSources(ctx)}
}

// GetCacheStatus returns local cache size and configured capacity.
func (s *DownloadService) GetCacheStatus(ctx context.Context) (result DownloadCacheStatusResult) {
	defer s.recoverCache(ctx, &result)
	status, err := s.manager.CacheStatus()
	if err != nil {
		dto := apperror.ToDTO(err)
		return DownloadCacheStatusResult{Error: &dto}
	}
	return DownloadCacheStatusResult{Status: status}
}

// ClearCache deletes local cached artifacts without changing settings.
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
