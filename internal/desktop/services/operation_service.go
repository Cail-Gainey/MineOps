package services

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// OperationResult contains one Operation or a stable desktop error.
type OperationResult struct {
	Operation *model.Operation `json:"operation,omitempty"`
	Error     *apperror.DTO    `json:"error,omitempty"`
}

// OperationListResult contains Operation rows or a stable desktop error.
type OperationListResult struct {
	Operations []model.Operation `json:"operations"`
	Error      *apperror.DTO     `json:"error,omitempty"`
}

// OperationMutationResult contains the number of terminal Operation rows deleted.
type OperationMutationResult struct {
	Deleted int64         `json:"deleted"`
	Error   *apperror.DTO `json:"error,omitempty"`
}

// ActionResult contains the stable error result of a command without a data payload.
type ActionResult struct {
	Error *apperror.DTO `json:"error,omitempty"`
}

// OperationService exposes durable Operation queries, cancellation, and terminal-history maintenance through explicit DTOs.
type OperationService struct {
	runner *service.OperationRunner
	store  repository.Store
	logger *applog.Logger
}

// NewOperationService creates the desktop Operation facade.
func NewOperationService(runner *service.OperationRunner, store repository.Store, logger *applog.Logger) *OperationService {
	return &OperationService{runner: runner, store: store, logger: logger}
}

// ServiceStartup recovers Operations left active by an earlier process.
func (s *OperationService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return apperror.Guard(ctx, s.logger, "OperationService.ServiceStartup", func() error {
		return s.runner.RecoverInterrupted(ctx)
	})
}

// ServiceShutdown cancels active Operations and waits for their final persisted state.
func (s *OperationService) ServiceShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(constants.DefaultShutdownTimeoutSec)*time.Second)
	defer cancel()
	return apperror.Guard(ctx, s.logger, "OperationService.ServiceShutdown", func() error {
		return s.runner.Shutdown(ctx)
	})
}

// Get returns one durable Operation by ID.
func (s *OperationService) Get(ctx context.Context, id string) (result OperationResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "OperationService.Get", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	operation, err := s.store.Operations().Get(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return OperationResult{Error: &dto}
	}
	return OperationResult{Operation: operation}
}

// ListActive returns pending and running Operations, optionally filtered by target ID.
func (s *OperationService) ListActive(ctx context.Context, targetID string) (result OperationListResult) {
	defer s.recoverList(ctx, "OperationService.ListActive", &result)
	operations, err := s.store.Operations().ListActive(ctx, model.ID(targetID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return OperationListResult{Error: &dto}
	}
	return OperationListResult{Operations: operations}
}

// ListHistory returns paginated durable Operation history.
func (s *OperationService) ListHistory(ctx context.Context, targetID string, limit int, offset int) (result OperationListResult) {
	defer s.recoverList(ctx, "OperationService.ListHistory", &result)
	operations, err := s.store.Operations().ListHistory(ctx, repository.OperationQuery{TargetID: model.ID(targetID), Limit: limit, Offset: offset})
	if err != nil {
		dto := apperror.ToDTO(err)
		return OperationListResult{Error: &dto}
	}
	return OperationListResult{Operations: operations}
}

// Cancel requests idempotent cancellation of an active Operation.
func (s *OperationService) Cancel(ctx context.Context, id string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "OperationService.Cancel", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.runner.Cancel(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// DeleteHistory deletes one completed, failed, or cancelled Operation.
func (s *OperationService) DeleteHistory(ctx context.Context, id string) (result OperationMutationResult) {
	defer s.recoverMutation(ctx, "OperationService.DeleteHistory", &result)
	if err := s.store.Operations().DeleteHistory(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return OperationMutationResult{Error: &dto}
	}
	return OperationMutationResult{Deleted: 1}
}

// ClearHistory deletes every terminal Operation while preserving active work.
func (s *OperationService) ClearHistory(ctx context.Context) (result OperationMutationResult) {
	defer s.recoverMutation(ctx, "OperationService.ClearHistory", &result)
	deleted, err := s.store.Operations().ClearHistory(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return OperationMutationResult{Error: &dto}
	}
	return OperationMutationResult{Deleted: deleted}
}

func (s *OperationService) recoverList(ctx context.Context, boundary string, result *OperationListResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *OperationService) recoverMutation(ctx context.Context, boundary string, result *OperationMutationResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
