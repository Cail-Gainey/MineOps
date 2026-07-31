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

// OperationHandler 执行一个持久化任务并上报可取消的进度。
type OperationHandler func(context.Context, OperationReporter) error

// OperationReporter 持久化并发布进行中任务的归一化进度。
type OperationReporter interface {
	SetProgress(stage string, progress float64, message string) error
}

// OperationPublisher 发布经节流的进度事件与必发的终态事件。
type OperationPublisher interface {
	PublishOperation(context.Context, model.Operation) error
}

// OperationRequest 描述一个新的持久化任务及其执行 handler。
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

// OperationRunner 先落盘再执行,并负责取消、资源锁、panic 恢复与等待。
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

// NewOperationRunner 创建持久化任务执行的唯一所有者。
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

// Start 落盘一个待执行任务、预留其资源,然后异步启动 handler。
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

// Retry 创建一个由 RetryOf 关联的新任务,绝不复用此前的任务标识。
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

// Cancel 幂等地请求取消一个进行中的任务。
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

// Wait 阻塞至任务完成,或直接返回其已落盘的终态。
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

// RecoverInterrupted 把上一个进程遗留为活动状态的任务标记为失败,供后续重试。
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

// Shutdown 取消全部活动任务,并等待其停止或关闭上下文超时。
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

// SetProgress 回报当前阶段、进度与说明,并持久化到 Operation。
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
