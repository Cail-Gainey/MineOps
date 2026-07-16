package service

import (
	"context"
	"errors"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// InstallationStepReporter persists per-step progress, messages, and log cursors.
type InstallationStepReporter interface {
	SetProgress(float64, string, int64) error
}

// InstallationStepHandler executes one idempotent installation step and returns its durable checkpoint.
type InstallationStepHandler func(context.Context, model.InstallationTask, model.InstallationStep, InstallationStepReporter) (map[string]any, error)

// InstallationStartResult contains both durable task and operation identities returned immediately to Wails.
type InstallationStartResult struct {
	TaskID      model.ID `json:"taskID"`
	OperationID model.ID `json:"operationID"`
}

// InstallationRunner coordinates durable checkpoints with OperationRunner cancellation and resource locking.
type InstallationRunner struct {
	clock      model.Clock
	store      repository.Store
	operations *OperationRunner

	mu       sync.RWMutex
	handlers map[string]InstallationStepHandler
}

// NewInstallationRunner creates the durable installation queue owner.
func NewInstallationRunner(clock model.Clock, store repository.Store, operations *OperationRunner) (*InstallationRunner, error) {
	if clock == nil || store == nil || operations == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "InstallationRunner 依赖不能为空")
	}
	return &InstallationRunner{clock: clock, store: store, operations: operations, handlers: make(map[string]InstallationStepHandler)}, nil
}

// RegisterStep replaces one named idempotent step implementation.
func (r *InstallationRunner) RegisterStep(name string, handler InstallationStepHandler) error {
	if name == "" || handler == nil {
		return apperror.New(apperror.CodeValidationRequired, "Installation Step 名称和 Handler 不能为空")
	}
	r.mu.Lock()
	r.handlers[name] = handler
	r.mu.Unlock()
	return nil
}

// Start persists the task and steps, starts the asynchronous Operation, and returns immediately.
func (r *InstallationRunner) Start(ctx context.Context, serverID model.ID) (InstallationStartResult, error) {
	task, steps, err := model.NewInstallationTask(r.clock, serverID)
	if err != nil {
		return InstallationStartResult{}, err
	}
	if err := r.store.Installations().Create(ctx, task, steps); err != nil {
		return InstallationStartResult{}, err
	}
	ready := make(chan struct{})
	operationID, err := r.operations.Start(ctx, OperationRequest{
		Type: enums.OperationInstall, TargetType: enums.OperationTargetServer, TargetID: serverID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			select {
			case <-operationCtx.Done():
				return operationCtx.Err()
			case <-ready:
				return r.execute(operationCtx, task.ID, reporter)
			}
		},
	})
	if err != nil {
		return InstallationStartResult{}, err
	}
	task.OperationID = &operationID
	if err := r.store.Installations().UpdateTask(ctx, task); err != nil {
		_ = r.operations.Cancel(ctx, operationID)
		close(ready)
		return InstallationStartResult{}, err
	}
	close(ready)
	return InstallationStartResult{TaskID: task.ID, OperationID: operationID}, nil
}

// Retry starts a new Operation while executing only Failed, Cancelled, or Waiting steps.
func (r *InstallationRunner) Retry(ctx context.Context, taskID model.ID) (InstallationStartResult, error) {
	task, steps, err := r.store.Installations().Get(ctx, taskID)
	if err != nil {
		return InstallationStartResult{}, err
	}
	if err := task.PrepareRetry(r.clock); err != nil {
		return InstallationStartResult{}, err
	}
	for index := range steps {
		if steps[index].Retryable() {
			if err := steps[index].PrepareRetry(r.clock); err != nil {
				return InstallationStartResult{}, err
			}
			if err := r.store.Installations().UpdateStep(ctx, &steps[index]); err != nil {
				return InstallationStartResult{}, err
			}
		}
	}
	previousOperationID := task.OperationID
	if previousOperationID == nil {
		return InstallationStartResult{}, apperror.New(apperror.CodeValidationConflict, "Installation Task 缺少可重试 Operation")
	}
	ready := make(chan struct{})
	handler := func(operationCtx context.Context, reporter OperationReporter) error {
		select {
		case <-operationCtx.Done():
			return operationCtx.Err()
		case <-ready:
			return r.execute(operationCtx, task.ID, reporter)
		}
	}
	operationID, err := r.operations.Retry(ctx, *previousOperationID, handler)
	if err != nil && apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
		operationID, err = r.operations.Start(ctx, OperationRequest{
			Type: enums.OperationInstall, TargetType: enums.OperationTargetServer, TargetID: task.ServerID,
			Handler: handler,
		})
	}
	if err != nil {
		return InstallationStartResult{}, err
	}
	task.OperationID = &operationID
	if err := r.store.Installations().UpdateTask(ctx, task); err != nil {
		_ = r.operations.Cancel(ctx, operationID)
		close(ready)
		return InstallationStartResult{}, err
	}
	close(ready)
	return InstallationStartResult{TaskID: task.ID, OperationID: operationID}, nil
}

// Cancel propagates cancellation through the active Operation context.
func (r *InstallationRunner) Cancel(ctx context.Context, operationID model.ID) error {
	return r.operations.Cancel(ctx, operationID)
}

// RecoverInterrupted converts process-interrupted tasks and running steps into retryable failed checkpoints.
func (r *InstallationRunner) RecoverInterrupted(ctx context.Context) error {
	tasks, err := r.store.Installations().ListRecoverable(ctx)
	if err != nil {
		return err
	}
	for index := range tasks {
		if tasks[index].State != enums.InstallationRunning {
			continue
		}
		task, steps, err := r.store.Installations().Get(ctx, tasks[index].ID)
		if err != nil {
			return err
		}
		failure := apperror.New(apperror.CodeProcessExitFailed, "应用异常退出，安装任务已中断").WithRetryable(true)
		for stepIndex := range steps {
			if steps[stepIndex].State == enums.InstallationStepRunning {
				if err := steps[stepIndex].Complete(r.clock, enums.InstallationStepFailed, steps[stepIndex].Checkpoint, failure); err != nil {
					return err
				}
				if err := r.store.Installations().UpdateStep(ctx, &steps[stepIndex]); err != nil {
					return err
				}
			}
		}
		if err := task.Complete(r.clock, enums.InstallationFailed); err != nil {
			return err
		}
		if err := r.store.Installations().UpdateTask(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

func (r *InstallationRunner) execute(ctx context.Context, taskID model.ID, operationReporter OperationReporter) error {
	task, steps, err := r.store.Installations().Get(ctx, taskID)
	if err != nil {
		return err
	}
	if err := task.Start(r.clock); err != nil {
		return err
	}
	if err := r.store.Installations().UpdateTask(ctx, task); err != nil {
		return err
	}
	for index := range steps {
		step := &steps[index]
		if step.State == enums.InstallationStepSuccess || step.State == enums.InstallationStepSkipped {
			continue
		}
		if ctx.Err() != nil {
			return r.finishCancelled(task, step, ctx.Err())
		}
		if step.State != enums.InstallationStepWaiting && step.State != enums.InstallationStepFailed {
			return apperror.New(apperror.CodeValidationConflict, "Installation Step 状态不能执行").WithDetails(map[string]any{
				"step": step.Name, "state": step.State,
			})
		}
		if err := step.Start(r.clock); err != nil {
			return err
		}
		task.CurrentStep = step.Order
		task.UpdatedAt = r.clock.Now().UTC()
		if err := r.store.Installations().UpdateTask(ctx, task); err != nil {
			return err
		}
		if err := r.store.Installations().UpdateStep(ctx, step); err != nil {
			return err
		}
		r.mu.RLock()
		handler := r.handlers[step.Name]
		r.mu.RUnlock()
		if handler == nil {
			err := apperror.New(apperror.CodeValidationConflict, "Installation Step 尚未注册实现").WithDetails(map[string]any{"step": step.Name})
			return r.finishFailed(task, step, err)
		}
		stepReporter := &installationStepReporter{runner: r, task: task, step: step, operation: operationReporter, totalSteps: len(steps)}
		checkpoint, executionErr := handler(ctx, *task, *step, stepReporter)
		if executionErr != nil {
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(executionErr, context.Canceled) {
				return r.finishCancelled(task, step, executionErr)
			}
			return r.finishFailed(task, step, executionErr)
		}
		if index == len(steps)-1 {
			completedStep := *step
			completedTask := *task
			if err := completedStep.Complete(r.clock, enums.InstallationStepSuccess, checkpoint, nil); err != nil {
				return err
			}
			if err := completedTask.Complete(r.clock, enums.InstallationSucceeded); err != nil {
				return err
			}
			if err := r.store.Transaction(ctx, func(registry repository.Registry) error {
				server, err := registry.MinecraftServers().Get(ctx, completedTask.ServerID, false)
				if err != nil {
					return err
				}
				server.State = enums.LifecycleStopped
				server.UpdatedAt = r.clock.Now().UTC()
				if err := registry.MinecraftServers().Update(ctx, server); err != nil {
					return err
				}
				if err := registry.Installations().UpdateStep(ctx, &completedStep); err != nil {
					return err
				}
				return registry.Installations().UpdateTask(ctx, &completedTask)
			}); err != nil {
				return r.finishFailed(task, step, err)
			}
			*step, *task = completedStep, completedTask
		} else {
			if err := step.Complete(r.clock, enums.InstallationStepSuccess, checkpoint, nil); err != nil {
				return err
			}
			if err := r.store.Installations().UpdateStep(ctx, step); err != nil {
				return err
			}
		}
		if err := operationReporter.SetProgress(step.Name, float64(step.Order)/float64(len(steps)), "安装步骤已完成"); err != nil {
			return err
		}
		if index == len(steps)-1 {
			return nil
		}
	}
	if err := task.Complete(r.clock, enums.InstallationSucceeded); err != nil {
		return err
	}
	return r.store.Installations().UpdateTask(context.WithoutCancel(ctx), task)
}

func (r *InstallationRunner) finishFailed(task *model.InstallationTask, step *model.InstallationStep, failure error) error {
	_ = step.Complete(r.clock, enums.InstallationStepFailed, step.Checkpoint, failure)
	_ = r.store.Installations().UpdateStep(context.Background(), step)
	_ = task.Complete(r.clock, enums.InstallationFailed)
	_ = r.store.Installations().UpdateTask(context.Background(), task)
	r.markServerFailed(task.ServerID)
	return failure
}

func (r *InstallationRunner) finishCancelled(task *model.InstallationTask, step *model.InstallationStep, failure error) error {
	_ = step.Complete(r.clock, enums.InstallationStepCancelled, step.Checkpoint, failure)
	_ = r.store.Installations().UpdateStep(context.Background(), step)
	_ = task.Complete(r.clock, enums.InstallationCancelled)
	_ = r.store.Installations().UpdateTask(context.Background(), task)
	r.markServerFailed(task.ServerID)
	return failure
}

func (r *InstallationRunner) markServerFailed(serverID model.ID) {
	server, err := r.store.MinecraftServers().Get(context.Background(), serverID, false)
	if err != nil {
		return
	}
	server.State = enums.LifecycleFailed
	server.UpdatedAt = r.clock.Now().UTC()
	_ = r.store.MinecraftServers().Update(context.Background(), server)
}

type installationStepReporter struct {
	runner     *InstallationRunner
	task       *model.InstallationTask
	step       *model.InstallationStep
	operation  OperationReporter
	totalSteps int
}

func (r *installationStepReporter) SetProgress(progress float64, message string, logCursor int64) error {
	if logCursor <= r.task.LogCursor {
		logCursor = r.task.LogCursor + 1
	}
	if err := r.step.SetProgress(r.runner.clock, progress, message, logCursor); err != nil {
		return err
	}
	if logCursor > r.task.LogCursor {
		r.task.LogCursor = logCursor
		r.task.UpdatedAt = r.runner.clock.Now().UTC()
		if err := r.runner.store.Installations().UpdateTask(context.Background(), r.task); err != nil {
			return err
		}
	}
	if err := r.runner.store.Installations().UpdateStep(context.Background(), r.step); err != nil {
		return err
	}
	overall := (float64(r.step.Order-1) + progress) / float64(r.totalSteps)
	return r.operation.SetProgress(r.step.Name, overall, message)
}
