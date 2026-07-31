package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

// ShutdownStep 是一个有序的组件关闭动作,带独立超时。
type ShutdownStep struct {
	Name    string
	Timeout time.Duration
	Action  func(context.Context) error
}

// ShutdownStepResult 记录一次已完成、失败或超时的关闭动作。
type ShutdownStepResult struct {
	Name       string        `json:"name"`
	Duration   time.Duration `json:"duration"`
	TimedOut   bool          `json:"timedOut"`
	Error      string        `json:"error,omitempty"`
	FinishedAt time.Time     `json:"finishedAt"`
}

// ShutdownCoordinator 顺序执行组件关闭,绝不为某个组件无限等待。
type ShutdownCoordinator struct {
	logger *applog.Logger
	steps  []ShutdownStep

	mu      sync.Mutex
	running bool
	done    chan struct{}
	results []ShutdownStepResult
}

// NewShutdownCoordinator 创建一条有序且有界的关闭流水线。
func NewShutdownCoordinator(logger *applog.Logger, steps ...ShutdownStep) (*ShutdownCoordinator, error) {
	if logger == nil {
		logger = applog.Default()
	}
	for _, step := range steps {
		if step.Name == "" || step.Action == nil {
			return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Shutdown Step 名称和操作不能为空")
		}
	}
	return &ShutdownCoordinator{logger: logger, steps: append([]ShutdownStep(nil), steps...)}, nil
}

// Shutdown 只运行一次流水线;并发或重复调用者等待并复用同一结果。
func (c *ShutdownCoordinator) Shutdown(ctx context.Context) ([]ShutdownStepResult, error) {
	c.mu.Lock()
	if c.done != nil {
		done := c.done
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return c.Results(), ctx.Err()
		case <-done:
			return c.Results(), shutdownResultsError(c.Results())
		}
	}
	c.running = true
	c.done = make(chan struct{})
	done := c.done
	c.mu.Unlock()

	results := make([]ShutdownStepResult, 0, len(c.steps))
	for _, step := range c.steps {
		if err := ctx.Err(); err != nil {
			results = append(results, ShutdownStepResult{Name: step.Name, TimedOut: true, Error: err.Error(), FinishedAt: time.Now().UTC()})
			continue
		}
		timeout := step.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		started := time.Now()
		stepContext, cancel := context.WithTimeout(ctx, timeout)
		resultChannel := make(chan error, 1)
		go func(action func(context.Context) error) { resultChannel <- action(stepContext) }(step.Action)
		result := ShutdownStepResult{Name: step.Name}
		select {
		case err := <-resultChannel:
			if err != nil && !errors.Is(err, context.Canceled) {
				result.Error = err.Error()
				c.logger.Error(context.WithoutCancel(ctx), "关闭组件失败", err, applog.Fields{"component": step.Name})
			}
		case <-stepContext.Done():
			result.TimedOut = true
			result.Error = stepContext.Err().Error()
			c.logger.Warn(context.WithoutCancel(ctx), "关闭组件超时，继续后续关闭阶段", applog.Fields{"component": step.Name, "timeout": timeout.String()})
		}
		cancel()
		result.Duration = time.Since(started)
		result.FinishedAt = time.Now().UTC()
		results = append(results, result)
	}

	c.mu.Lock()
	c.results = results
	c.running = false
	close(done)
	c.mu.Unlock()
	return append([]ShutdownStepResult(nil), results...), shutdownResultsError(results)
}

// Results 返回最近一次稳定的关闭证据。
func (c *ShutdownCoordinator) Results() []ShutdownStepResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ShutdownStepResult(nil), c.results...)
}

func shutdownResultsError(results []ShutdownStepResult) error {
	var failures []error
	for _, result := range results {
		if result.Error != "" {
			failures = append(failures, errors.New(result.Name+": "+result.Error))
		}
	}
	return errors.Join(failures...)
}
