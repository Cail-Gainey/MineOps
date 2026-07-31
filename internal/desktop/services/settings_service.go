package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[SettingsChangedEvent](constants.SettingsChangedEventName)
}

// SettingsResult 承载当前 Settings Snapshot 或稳定的桌面错误。
type SettingsResult struct {
	Settings *model.SettingsSnapshot `json:"settings,omitempty"`
	Error    *apperror.DTO           `json:"error,omitempty"`
}

// SettingsChangedEvent 标识一个已提交、消费方需要重载的设置分类。
type SettingsChangedEvent struct {
	Category string `json:"category"`
}

// SettingsService 对外暴露加密设置的加载、保存与分类重置。
type SettingsService struct {
	manager *appsettings.Manager
	logger  *applog.Logger
}

// NewSettingsService 创建桌面侧的设置门面。
func NewSettingsService(manager *appsettings.Manager, logger *applog.Logger) *SettingsService {
	return &SettingsService{manager: manager, logger: logger}
}

// Get 返回当前不可变的 Settings Snapshot。
func (s *SettingsService) Get(ctx context.Context) (result SettingsResult) {
	defer s.recover(ctx, "SettingsService.Get", &result)
	snapshot := s.manager.Snapshot()
	return SettingsResult{Settings: &snapshot}
}

// Save 校验并在事务内持久化完整的 Settings Snapshot。
func (s *SettingsService) Save(ctx context.Context, snapshot model.SettingsSnapshot) (result SettingsResult) {
	defer s.recover(ctx, "SettingsService.Save", &result)
	if err := s.manager.Save(ctx, snapshot); err != nil {
		dto := apperror.ToDTO(err)
		return SettingsResult{Error: &dto}
	}
	committed := s.manager.Snapshot()
	return SettingsResult{Settings: &committed}
}

// ResetCategory 把一个设置分类恢复为内置默认值。
func (s *SettingsService) ResetCategory(ctx context.Context, category string) (result SettingsResult) {
	defer s.recover(ctx, "SettingsService.ResetCategory", &result)
	if err := s.manager.ResetCategory(ctx, enums.SettingsCategory(category)); err != nil {
		dto := apperror.ToDTO(err)
		return SettingsResult{Error: &dto}
	}
	committed := s.manager.Snapshot()
	return SettingsResult{Settings: &committed}
}

func (s *SettingsService) recover(ctx context.Context, boundary string, result *SettingsResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
