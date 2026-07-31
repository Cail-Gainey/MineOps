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

// QuitBlockedEvent 承载阻止应用退出的编辑器与表单。
type QuitBlockedEvent struct {
	Items []service.UnsavedItem `json:"items"`
}

// ExitGuardStateResult 承载当前的未保存内容登记表。
type ExitGuardStateResult struct {
	Items []service.UnsavedItem `json:"items"`
	Error *apperror.DTO         `json:"error,omitempty"`
}

// ExitGuardService 对外暴露未保存状态登记与确认后的退出续行。
type ExitGuardService struct {
	guard  *service.ExitGuard
	logger *applog.Logger
}

// NewExitGuardService 创建 Wails 退出保护门面。
func NewExitGuardService(guard *service.ExitGuard, logger *applog.Logger) *ExitGuardService {
	return &ExitGuardService{guard: guard, logger: logger}
}

// SetDirty 登记或清除一个编辑器或表单的未保存状态。
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

// State 返回当前的未保存内容登记表。
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

// ConfirmQuit 放行一次原生退出请求,并在绑定响应送达后继续关闭流程。
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
