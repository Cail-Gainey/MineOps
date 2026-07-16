package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// LifecycleStateResult contains verified Server and remote process state or a stable error.
type LifecycleStateResult struct {
	State    enums.LifecycleState         `json:"state"`
	Identity *model.RemoteProcessIdentity `json:"identity,omitempty"`
	Error    *apperror.DTO                `json:"error,omitempty"`
}

// LifecycleOperationResult contains the immediately returned Operation ID or a stable error.
type LifecycleOperationResult struct {
	OperationID string        `json:"operationID,omitempty"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// LifecycleService exposes GetState, Start, Stop, Restart, and Recover to Wails.
type LifecycleService struct {
	manager *service.LifecycleManager
	logger  *applog.Logger
}

// NewLifecycleService creates the desktop lifecycle facade.
func NewLifecycleService(manager *service.LifecycleManager, logger *applog.Logger) *LifecycleService {
	return &LifecycleService{manager: manager, logger: logger}
}

// GetState probes the remote process before returning lifecycle state.
func (s *LifecycleService) GetState(ctx context.Context, serverID string) (result LifecycleStateResult) {
	defer s.recoverState(ctx, &result)
	state, identity, err := s.manager.GetState(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleStateResult{State: state, Identity: identity, Error: &dto}
	}
	return LifecycleStateResult{State: state, Identity: identity}
}

// Start starts the Server asynchronously and returns an Operation ID.
func (s *LifecycleService) Start(ctx context.Context, serverID string, firewallConfirmed, tmuxInstallConfirmed bool) (result LifecycleOperationResult) {
	defer s.recoverOperation(ctx, "LifecycleService.Start", &result)
	operationID, err := s.manager.Start(ctx, model.ID(serverID), firewallConfirmed, tmuxInstallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleOperationResult{Error: &dto}
	}
	return LifecycleOperationResult{OperationID: operationID.String()}
}

// Stop gracefully stops the Server and optionally forces process-group termination.
func (s *LifecycleService) Stop(ctx context.Context, serverID string, force bool) (result LifecycleOperationResult) {
	defer s.recoverOperation(ctx, "LifecycleService.Stop", &result)
	operationID, err := s.manager.Stop(ctx, model.ID(serverID), force)
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleOperationResult{Error: &dto}
	}
	return LifecycleOperationResult{OperationID: operationID.String()}
}

// Restart performs tracked Stop and Start stages under one Server resource lock.
func (s *LifecycleService) Restart(ctx context.Context, serverID string, tmuxInstallConfirmed bool) (result LifecycleOperationResult) {
	defer s.recoverOperation(ctx, "LifecycleService.Restart", &result)
	operationID, err := s.manager.Restart(ctx, model.ID(serverID), tmuxInstallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return LifecycleOperationResult{Error: &dto}
	}
	return LifecycleOperationResult{OperationID: operationID.String()}
}

// Recover probes every active identity and reconciles Server state.
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
