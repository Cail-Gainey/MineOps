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

// OperationResult 承载一条 Operation 或稳定的桌面错误。
type OperationResult struct {
	Operation *model.Operation `json:"operation,omitempty"`
	Error     *apperror.DTO    `json:"error,omitempty"`
}

// OperationListResult 承载 Operation 列表或稳定的桌面错误。
type OperationListResult struct {
	Operations []model.Operation `json:"operations"`
	Error      *apperror.DTO     `json:"error,omitempty"`
}

// OperationMutationResult 承载已删除的终态 Operation 行数。
type OperationMutationResult struct {
	Deleted int64         `json:"deleted"`
	Error   *apperror.DTO `json:"error,omitempty"`
}

// ActionResult 承载无数据负载命令的稳定错误结果。
type ActionResult struct {
	Error *apperror.DTO `json:"error,omitempty"`
}

// OperationService 通过显式 DTO 暴露持久化 Operation 的查询、取消与终态历史维护。
type OperationService struct {
	runner *service.OperationRunner
	store  repository.Store
	logger *applog.Logger
}

// NewOperationService 创建桌面侧的 Operation 门面。
func NewOperationService(runner *service.OperationRunner, store repository.Store, logger *applog.Logger) *OperationService {
	return &OperationService{runner: runner, store: store, logger: logger}
}

// ServiceStartup 恢复上一个进程遗留为活动状态的 Operation。
func (s *OperationService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return apperror.Guard(ctx, s.logger, "OperationService.ServiceStartup", func() error {
		return s.runner.RecoverInterrupted(ctx)
	})
}

// ServiceShutdown 取消活动 Operation 并等待其最终状态落盘。
func (s *OperationService) ServiceShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(constants.DefaultShutdownTimeoutSec)*time.Second)
	defer cancel()
	return apperror.Guard(ctx, s.logger, "OperationService.ServiceShutdown", func() error {
		return s.runner.Shutdown(ctx)
	})
}

// Get 按 ID 返回一条持久化 Operation。
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

// ListActive 返回待执行与运行中的 Operation,可按目标 ID 过滤。
func (s *OperationService) ListActive(ctx context.Context, targetID string) (result OperationListResult) {
	defer s.recoverList(ctx, "OperationService.ListActive", &result)
	operations, err := s.store.Operations().ListActive(ctx, model.ID(targetID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return OperationListResult{Error: &dto}
	}
	return OperationListResult{Operations: operations}
}

// ListHistory 分页返回持久化的 Operation 历史。
func (s *OperationService) ListHistory(ctx context.Context, targetID string, limit int, offset int) (result OperationListResult) {
	defer s.recoverList(ctx, "OperationService.ListHistory", &result)
	operations, err := s.store.Operations().ListHistory(ctx, repository.OperationQuery{TargetID: model.ID(targetID), Limit: limit, Offset: offset})
	if err != nil {
		dto := apperror.ToDTO(err)
		return OperationListResult{Error: &dto}
	}
	return OperationListResult{Operations: operations}
}

// Cancel 幂等地请求取消一条活动 Operation。
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

// DeleteHistory 删除一条已完成、失败或已取消的 Operation。
func (s *OperationService) DeleteHistory(ctx context.Context, id string) (result OperationMutationResult) {
	defer s.recoverMutation(ctx, "OperationService.DeleteHistory", &result)
	if err := s.store.Operations().DeleteHistory(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return OperationMutationResult{Error: &dto}
	}
	return OperationMutationResult{Deleted: 1}
}

// ClearHistory 删除全部终态 Operation,保留进行中的工作。
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
