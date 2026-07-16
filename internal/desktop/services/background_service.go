package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// BackgroundResourceResult contains the controlled background status or a stable desktop error.
type BackgroundResourceResult struct {
	Resource *service.BackgroundResource `json:"resource,omitempty"`
	Settings *model.SettingsSnapshot     `json:"settings,omitempty"`
	Error    *apperror.DTO               `json:"error,omitempty"`
}

// BackgroundService exposes controlled background import, resolution, and reset operations.
type BackgroundService struct {
	manager *service.BackgroundManager
	logger  *applog.Logger
}

// NewBackgroundService creates the desktop background resource facade.
func NewBackgroundService(manager *service.BackgroundManager, logger *applog.Logger) *BackgroundService {
	return &BackgroundService{manager: manager, logger: logger}
}

// Import copies a local image into the controlled data directory and selects it.
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

// Get returns the current controlled background resource and bounded Data URL.
func (s *BackgroundService) Get(ctx context.Context) (result BackgroundResourceResult) {
	defer s.recover(ctx, "BackgroundService.Get", &result)
	resource, err := s.manager.Resolve()
	if err != nil {
		dto := apperror.ToDTO(err)
		return BackgroundResourceResult{Error: &dto}
	}
	return BackgroundResourceResult{Resource: &resource}
}

// Reset clears the controlled background and restores the semantic theme background.
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
