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

// ShutdownGroup owns a root Context, registered goroutines, panic recovery, and bounded shutdown.
type ShutdownGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *applog.Logger

	mu        sync.Mutex
	accepting bool
	errors    []error
	wait      sync.WaitGroup
}

// NewShutdownGroup creates the application root Context and goroutine ownership boundary.
func NewShutdownGroup(parent context.Context, logger *applog.Logger) *ShutdownGroup {
	ctx, cancel := context.WithCancel(parent)
	if logger == nil {
		logger = applog.Default()
	}
	return &ShutdownGroup{ctx: ctx, cancel: cancel, logger: logger, accepting: true}
}

// Context returns the root Context inherited by owned services and goroutines.
func (g *ShutdownGroup) Context() context.Context {
	return g.ctx
}

// Go starts one named owned goroutine and records returned errors or panics.
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

// Shutdown stops new goroutines, cancels the root Context, and waits for every owner to exit.
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
