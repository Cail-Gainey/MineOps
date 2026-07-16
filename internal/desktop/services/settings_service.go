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

// SettingsResult contains the current Settings Snapshot or a stable desktop error.
type SettingsResult struct {
	Settings *model.SettingsSnapshot `json:"settings,omitempty"`
	Error    *apperror.DTO           `json:"error,omitempty"`
}

// SettingsChangedEvent identifies one committed category that consumers should reload.
type SettingsChangedEvent struct {
	Category string `json:"category"`
}

// SettingsService exposes encrypted Settings loading, saving, and category reset.
type SettingsService struct {
	manager *appsettings.Manager
	logger  *applog.Logger
}

// NewSettingsService creates the desktop Settings facade.
func NewSettingsService(manager *appsettings.Manager, logger *applog.Logger) *SettingsService {
	return &SettingsService{manager: manager, logger: logger}
}

// Get returns the immutable current Settings Snapshot.
func (s *SettingsService) Get(ctx context.Context) (result SettingsResult) {
	defer s.recover(ctx, "SettingsService.Get", &result)
	snapshot := s.manager.Snapshot()
	return SettingsResult{Settings: &snapshot}
}

// Save validates and transactionally persists the complete Settings Snapshot.
func (s *SettingsService) Save(ctx context.Context, snapshot model.SettingsSnapshot) (result SettingsResult) {
	defer s.recover(ctx, "SettingsService.Save", &result)
	if err := s.manager.Save(ctx, snapshot); err != nil {
		dto := apperror.ToDTO(err)
		return SettingsResult{Error: &dto}
	}
	committed := s.manager.Snapshot()
	return SettingsResult{Settings: &committed}
}

// ResetCategory restores one Settings category to its embedded default.
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
