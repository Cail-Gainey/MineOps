package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// BackgroundResourceResult 承载受控背景资源状态或稳定的桌面错误。
type BackgroundResourceResult struct {
	Resource *service.BackgroundResource `json:"resource,omitempty"`
	Settings *model.SettingsSnapshot     `json:"settings,omitempty"`
	Error    *apperror.DTO               `json:"error,omitempty"`
}

// BackgroundService 对外暴露受控背景的导入、解析与重置操作。
type BackgroundService struct {
	manager *service.BackgroundManager
	logger  *applog.Logger
}

// NewBackgroundService 创建桌面侧的背景资源门面。
func NewBackgroundService(manager *service.BackgroundManager, logger *applog.Logger) *BackgroundService {
	return &BackgroundService{manager: manager, logger: logger}
}

// Import 把本地图片复制进受控数据目录并选为当前背景。
func (s *BackgroundService) Import(ctx context.Context, sourcePath string) (result BackgroundResourceResult) {
	defer s.recover(ctx, "BackgroundService.Import", &result)
	snapshot, err := s.manager.Import(ctx, sourcePath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return BackgroundResourceResult{Error: &dto}
	}
	resource, err := s.manager.Resolve()
	if err != nil {
		dto := apperror.ToDTO(err)
		return BackgroundResourceResult{Error: &dto}
	}
	return BackgroundResourceResult{Resource: &resource, Settings: &snapshot}
}

// Get 返回当前受控背景资源及有界的 Data URL。
func (s *BackgroundService) Get(ctx context.Context) (result BackgroundResourceResult) {
	defer s.recover(ctx, "BackgroundService.Get", &result)
	resource, err := s.manager.Resolve()
	if err != nil {
		dto := apperror.ToDTO(err)
		return BackgroundResourceResult{Error: &dto}
	}
	return BackgroundResourceResult{Resource: &resource}
}

// Reset 清除受控背景并恢复语义主题背景。
func (s *BackgroundService) Reset(ctx context.Context) (result BackgroundResourceResult) {
	defer s.recover(ctx, "BackgroundService.Reset", &result)
	snapshot, err := s.manager.Reset(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return BackgroundResourceResult{Error: &dto}
	}
	resource := service.BackgroundResource{}
	return BackgroundResourceResult{Resource: &resource, Settings: &snapshot}
}

func (s *BackgroundService) recover(ctx context.Context, boundary string, result *BackgroundResourceResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
