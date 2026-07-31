package service

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

// ShutdownGroup 持有根 Context、已登记的 goroutine、panic 恢复与有界关闭。
type ShutdownGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *applog.Logger

	mu        sync.Mutex
	accepting bool
	errors    []error
	wait      sync.WaitGroup
}

// NewShutdownGroup 创建应用根 Context 与 goroutine 归属边界。
func NewShutdownGroup(parent context.Context, logger *applog.Logger) *ShutdownGroup {
	ctx, cancel := context.WithCancel(parent)
	if logger == nil {
		logger = applog.Default()
	}
	return &ShutdownGroup{ctx: ctx, cancel: cancel, logger: logger, accepting: true}
}

// Context 返回被其持有的服务与 goroutine 所继承的根 Context。
func (g *ShutdownGroup) Context() context.Context {
	return g.ctx
}

// Go 启动一个具名的受管 goroutine,并记录其返回的错误或 panic。
func (g *ShutdownGroup) Go(name string, action func(context.Context) error) error {
	if name == "" || action == nil {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Goroutine 名称和操作不能为空")
	}
	g.mu.Lock()
	if !g.accepting {
		g.mu.Unlock()
		return apperror.New(apperror.CodeValidationConflict, "应用正在关闭，不再接受 Goroutine")
	}
	g.wait.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.wait.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				g.record(name, apperror.Wrap(apperror.CodeInternal, "后台任务发生 panic", fmt.Errorf("%v\n%s", recovered, debug.Stack())))
			}
		}()
		if err := action(g.ctx); err != nil && !errors.Is(err, context.Canceled) {
			g.record(name, err)
		}
	}()
	return nil
}

// Shutdown 停止接受新 goroutine、取消根 Context,并等待每个持有者退出。
func (g *ShutdownGroup) Shutdown(ctx context.Context) error {
	g.mu.Lock()
	g.accepting = false
	g.cancel()
	g.mu.Unlock()
	done := make(chan struct{})
	go func() {
		g.wait.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), g.recordedErrors())
	case <-done:
		return g.recordedErrors()
	}
}

func (g *ShutdownGroup) record(name string, err error) {
	g.mu.Lock()
	g.errors = append(g.errors, err)
	g.mu.Unlock()
	g.logger.Error(context.WithoutCancel(g.ctx), "后台任务退出", err, applog.Fields{"component": name})
}

func (g *ShutdownGroup) recordedErrors() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return errors.Join(g.errors...)
}
