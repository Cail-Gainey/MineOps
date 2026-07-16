package service

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// OperationHandler executes one durable operation and reports cancellable progress.
type OperationHandler func(context.Context, OperationReporter) error

// OperationReporter persists and publishes normalized progress for the active operation.
type OperationReporter interface {
	SetProgress(stage string, progress float64, message string) error
}

// OperationPublisher publishes throttled progress and mandatory final-state events.
type OperationPublisher interface {
	PublishOperation(context.Context, model.Operation) error
}

// OperationRequest describes a new durable operation and its execution handler.
type OperationRequest struct {
	Type       enums.OperationType
	TargetType enums.OperationTargetType
	TargetID   model.ID
	Prepare    func(model.ID) error
	Handler    OperationHandler
}

type operationJob struct {
	mu            sync.Mutex
	operation     *model.Operation
	cancel        context.CancelFunc
	done          chan struct{}
	lastPublished time.Time
}

// OperationRunner persists before execution, owns cancellation, resource locks, panic recovery, and waits.
type OperationRunner struct {
	rootCtx   context.Context
	clock     model.Clock
	store     repository.Store
	logger    *applog.Logger
	publisher OperationPublisher

	mu        sync.Mutex
	jobs      map[model.ID]*operationJob
	resources map[string]model.ID
	wait      sync.WaitGroup
}

// NewOperationRunner creates the single owner of durable operation execution.
func NewOperationRunner(rootCtx context.Context, clock model.Clock, store repository.Store, logger *applog.Logger, publisher OperationPublisher) (*OperationRunner, error) {
	if rootCtx == nil || clock == nil || store == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "OperationRunner 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &OperationRunner{
		rootCtx: rootCtx, clock: clock, store: store, logger: logger, publisher: publisher,
		jobs: make(map[model.ID]*operationJob), resources: make(map[string]model.ID),
	}, nil
}

// Start persists a pending operation, reserves its resource, and then starts the handler asynchronously.
func (r *OperationRunner) Start(ctx context.Context, request OperationRequest) (model.ID, error) {
	if request.Handler == nil {
		return "", apperror.New(apperror.CodeValidationRequired, "Operation Handler 不能为空")
	}
	operation, err := model.NewOperation(r.clock, request.Type, request.TargetType, request.TargetID)
	if err != nil {
		return "", err
	}
	return r.startPersisted(ctx, operation, request.Prepare, request.Handler)
}

// Retry creates a new operation linked by RetryOf and never reuses the previous operation identity.
func (r *OperationRunner) Retry(ctx context.Context, previousID model.ID, handler OperationHandler) (model.ID, error) {
	previous, err := r.store.Operations().Get(ctx, previousID)
	if err != nil {
		return "", err
	}
	if previous.State != enums.OperationFailed && previous.State != enums.OperationCancelled {
		return "", apperror.New(apperror.CodeValidationConflict, "只有失败或已取消的 Operation 可以重试")
	}
	retry, err := model.NewOperation(r.clock, previous.Type, previous.TargetType, previous.TargetID)
	if err != nil {
		return "", err
	}
	retry.RetryOf = &previous.ID
	return r.startPersisted(ctx, retry, nil, handler)
}

func (r *OperationRunner) startPersisted(ctx context.Context, operation *model.Operation, prepare func(model.ID) error, handler OperationHandler) (model.ID, error) {
	resourceKey := operation.TargetType.String() + ":" + operation.TargetID.String()
	r.mu.Lock()
	if activeID, exists := r.resources[resourceKey]; exists {
		r.mu.Unlock()
		return "", apperror.New(apperror.CodeValidationConflict, "目标资源已有冲突 Operation").WithDetails(map[string]any{
			"activeOperationID": activeID,
		})
	}
	if err := r.store.Operations().Create(ctx, operation); err != nil {
		r.mu.Unlock()
		return "", err
	}
	jobCtx, cancel := context.WithCancel(r.rootCtx)
	job := &operationJob{operation: operation, cancel: cancel, done: make(chan struct{})}
	r.jobs[operation.ID] = job
	r.resources[resourceKey] = operation.ID
	r.mu.Unlock()
	if prepare != nil {
		if err := prepare(operation.ID); err != nil {
			cancel()
			r.mu.Lock()
			delete(r.jobs, operation.ID)
			delete(r.resources, resourceKey)
			r.mu.Unlock()
			failure := apperror.Wrap(apperror.CodeIOWriteFailed, "准备 Operation 关联状态失败", err)
			_ = operation.Transition(r.clock, enums.OperationFailed, failure)
			_ = r.store.Operations().Update(context.WithoutCancel(ctx), operation)
			return "", failure
		}
	}

	r.wait.Add(1)
	go r.execute(jobCtx, resourceKey, job, handler)
	return operation.ID, nil
}

func (r *OperationRunner) execute(ctx context.Context, resourceKey string, job *operationJob, handler OperationHandler) {
	defer r.wait.Done()
	defer close(job.done)
	defer func() {
		r.mu.Lock()
		delete(r.jobs, job.operation.ID)
		delete(r.resources, resourceKey)
		r.mu.Unlock()
	}()

	job.mu.Lock()
	transitionErr := job.operation.Transition(r.clock, enums.OperationRunning, nil)
	if transitionErr == nil {
		transitionErr = r.store.Operations().Update(ctx, job.operation)
	}
	if transitionErr == nil {
		r.publish(ctx, job.operation)
	}
	job.mu.Unlock()
	if transitionErr != nil {
		r.logger.Error(ctx, "启动 Operation 失败", transitionErr, applog.Fields{"operation_id": job.operation.ID})
		return
	}

	var executionError error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				executionError = apperror.Wrap(apperror.CodeInternal, "Operation 执行发生 panic", fmt.Errorf("%v\n%s", recovered, debug.Stack()))
			}
		}()
		executionError = handler(ctx, &jobReporter{runner: r, job: job, ctx: ctx})
	}()

	job.mu.Lock()
	next := enums.OperationSucceeded
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(executionError, context.Canceled) {
		next = enums.OperationCancelled
		if executionError == nil {
			executionError = apperror.Wrap(apperror.CodeProcessCancelled, "Operation 已取消", context.Canceled)
		}
	} else if executionError != nil {
		next = enums.OperationFailed
	}
	if err := job.operation.Transition(r.clock, next, executionError); err != nil {
		executionError = errors.Join(executionError, err)
	}
	if err := r.store.Operations().Update(context.WithoutCancel(ctx), job.operation); err != nil {
		executionError = errors.Join(executionError, err)
	}
	r.publish(context.WithoutCancel(ctx), job.operation)
	finalOperation := *job.operation
	job.mu.Unlock()

	if executionError != nil && next != enums.OperationCancelled {
		r.logger.Error(ctx, "Operation 执行失败", executionError, applog.Fields{
			"operation_id": finalOperation.ID,
			"target_id":    finalOperation.TargetID,
			"type":         finalOperation.Type,
		})
	}
}

// Cancel requests idempotent cancellation of an active operation.
func (r *OperationRunner) Cancel(ctx context.Context, id model.ID) error {
	r.mu.Lock()
	job := r.jobs[id]
	r.mu.Unlock()
	if job != nil {
		job.cancel()
		return nil
	}
	operation, err := r.store.Operations().Get(ctx, id)
	if err != nil {
		return err
	}
	if operation.State == enums.OperationSucceeded || operation.State == enums.OperationFailed || operation.State == enums.OperationCancelled {
		return nil
	}
	return apperror.New(apperror.CodeValidationConflict, "Operation 当前不在本进程执行，无法直接取消")
}

// Wait blocks until an active operation completes or returns its already persisted final state.
func (r *OperationRunner) Wait(ctx context.Context, id model.ID) (*model.Operation, error) {
	r.mu.Lock()
	job := r.jobs[id]
	r.mu.Unlock()
	if job == nil {
		return r.store.Operations().Get(ctx, id)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-job.done:
		return r.store.Operations().Get(context.WithoutCancel(ctx), id)
	}
}

// RecoverInterrupted marks operations left active by an earlier process as failed for later retry.
func (r *OperationRunner) RecoverInterrupted(ctx context.Context) error {
	operations, err := r.store.Operations().ListActive(ctx, "")
	if err != nil {
		return err
	}
	for index := range operations {
		failure := apperror.New(apperror.CodeProcessExitFailed, "应用异常退出，Operation 已中断").WithRetryable(true)
		if err := operations[index].Transition(r.clock, enums.OperationFailed, failure); err != nil {
			return err
		}
		if err := r.store.Operations().Update(ctx, &operations[index]); err != nil {
			return err
		}
		r.publish(ctx, &operations[index])
	}
	return nil
}

// Shutdown cancels all active operations and waits until they stop or the shutdown context expires.
func (r *OperationRunner) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	for _, job := range r.jobs {
		job.cancel()
	}
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.wait.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (r *OperationRunner) publish(ctx context.Context, operation *model.Operation) {
	if r.publisher == nil || operation == nil {
		return
	}
	if err := r.publisher.PublishOperation(ctx, *operation); err != nil {
		r.logger.Error(ctx, "发布 Operation 事件失败", err, applog.Fields{"operation_id": operation.ID})
	}
}

type jobReporter struct {
	runner *OperationRunner
	job    *operationJob
	ctx    context.Context
}

func (r *jobReporter) SetProgress(stage string, progress float64, message string) error {
	r.job.mu.Lock()
	defer r.job.mu.Unlock()
	if err := r.job.operation.SetProgress(stage, progress, message); err != nil {
		return err
	}
	if err := r.runner.store.Operations().Update(r.ctx, r.job.operation); err != nil {
		return err
	}
	now := r.runner.clock.Now()
	if now.Sub(r.job.lastPublished) >= 100*time.Millisecond || progress >= 1 {
		r.runner.publish(r.ctx, r.job.operation)
		r.job.lastPublished = now
	}
	return nil
}
