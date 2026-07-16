package services

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[QuitBlockedEvent](constants.QuitBlockedEventName)
}

// QuitBlockedEvent contains the editors and forms preventing application exit.
type QuitBlockedEvent struct {
	Items []service.UnsavedItem `json:"items"`
}

// ExitGuardStateResult contains the current unsaved-content registry.
type ExitGuardStateResult struct {
	Items []service.UnsavedItem `json:"items"`
	Error *apperror.DTO         `json:"error,omitempty"`
}

// ExitGuardService exposes dirty-state registration and confirmed quit continuation.
type ExitGuardService struct {
	guard  *service.ExitGuard
	logger *applog.Logger
}

// NewExitGuardService creates the Wails exit protection facade.
func NewExitGuardService(guard *service.ExitGuard, logger *applog.Logger) *ExitGuardService {
	return &ExitGuardService{guard: guard, logger: logger}
}

// SetDirty registers or clears one editor or form dirty state.
func (s *ExitGuardService) SetDirty(ctx context.Context, owner, label string, dirty bool) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "ExitGuardService.SetDirty", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.guard.SetDirty(owner, label, dirty); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// State returns the current unsaved-content registry.
func (s *ExitGuardService) State(ctx context.Context) (result ExitGuardStateResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "ExitGuardService.State", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	return ExitGuardStateResult{Items: s.guard.Items()}
}

// ConfirmQuit authorizes one native quit request and resumes shutdown after the binding response is delivered.
func (s *ExitGuardService) ConfirmQuit(ctx context.Context) (result ActionResult) {
	s.guard.ConfirmQuit()
	app := application.Get()
	if app == nil {
		dto := apperror.ToDTO(apperror.New(apperror.CodeValidationConflict, "Wails Application 当前不可用"))
		return ActionResult{Error: &dto}
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		app.Quit()
	}()
	return ActionResult{}
}
