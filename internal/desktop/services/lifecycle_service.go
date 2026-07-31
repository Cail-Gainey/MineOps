package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// LifecycleStateResult 承载已校验的 Server 与远端进程状态或稳定错误。
type LifecycleStateResult struct {
	State    enums.LifecycleState         `json:"state"`
	Identity *model.RemoteProcessIdentity `json:"identity,omitempty"`
	Error    *apperror.DTO                `json:"error,omitempty"`
}

// LifecycleOperationResult 承载立即返回的 Operation ID 或稳定错误。
type LifecycleOperationResult struct {
	OperationID string        `json:"operationID,omitempty"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// LifecycleService 向 Wails 暴露 GetState、Start、Stop、Restart 与 Recover。
type LifecycleService struct {
	manager *service.LifecycleManager
	logger  *applog.Logger
}

// NewLifecycleService 创建桌面侧的生命周期门面。
func NewLifecycleService(manager *service.LifecycleManager, logger *applog.Logger) *LifecycleService {
	return &LifecycleService{manager: manager, logger: logger}
}

// GetState 先探测远端进程,再返回生命周期状态。
func (s *LifecycleService) GetState(ctx context.Context, serverID string) (result LifecycleStateResult) {
	defer s.recoverState(ctx, &result)
	state, identity, err := s.manager.GetState(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleStateResult{State: state, Identity: identity, Error: &dto}
	}
	return LifecycleStateResult{State: state, Identity: identity}
}

// Start 异步启动 Server 并返回 Operation ID。
func (s *LifecycleService) Start(ctx context.Context, serverID string, firewallConfirmed, tmuxInstallConfirmed bool) (result LifecycleOperationResult) {
	defer s.recoverOperation(ctx, "LifecycleService.Start", &result)
	operationID, err := s.manager.Start(ctx, model.ID(serverID), firewallConfirmed, tmuxInstallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleOperationResult{Error: &dto}
	}
	return LifecycleOperationResult{OperationID: operationID.String()}
}

// Stop 优雅停止 Server,必要时强制结束整个进程组。
func (s *LifecycleService) Stop(ctx context.Context, serverID string, force bool) (result LifecycleOperationResult) {
	defer s.recoverOperation(ctx, "LifecycleService.Stop", &result)
	operationID, err := s.manager.Stop(ctx, model.ID(serverID), force)
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleOperationResult{Error: &dto}
	}
	return LifecycleOperationResult{OperationID: operationID.String()}
}

// Restart 在同一把 Server 资源锁下执行可跟踪的停止与启动阶段。
func (s *LifecycleService) Restart(ctx context.Context, serverID string, tmuxInstallConfirmed bool) (result LifecycleOperationResult) {
	defer s.recoverOperation(ctx, "LifecycleService.Restart", &result)
	operationID, err := s.manager.Restart(ctx, model.ID(serverID), tmuxInstallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleOperationResult{Error: &dto}
	}
	return LifecycleOperationResult{OperationID: operationID.String()}
}

// Recover 探测全部活动身份并校正 Server 状态。
func (s *LifecycleService) Recover(ctx context.Context) (result ActionResult) {
	defer s.recoverAction(ctx, &result)
	if err := s.manager.Recover(ctx); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *LifecycleService) recoverState(ctx context.Context, result *LifecycleStateResult) {
	var err error
	apperror.Recover(ctx, s.logger, "LifecycleService.GetState", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *LifecycleService) recoverOperation(ctx context.Context, boundary string, result *LifecycleOperationResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *LifecycleService) recoverAction(ctx context.Context, result *ActionResult) {
	var err error
	apperror.Recover(ctx, s.logger, "LifecycleService.Recover", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
