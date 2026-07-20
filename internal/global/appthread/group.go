package appthread

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

// Group runs a batch of related tasks on a Pool and waits for their combined completion.
//
// Every task shares the group's Context and the pool's concurrency budget. Errors and recovered
// panics are collected and returned together by Wait. A Group must not be reused after Wait.
type Group struct {
	pool *Pool
	ctx  context.Context

	wg  sync.WaitGroup
	mu  sync.Mutex
	err []error
}

// NewGroup starts a task batch bound to ctx and this pool's concurrency budget.
func (p *Pool) NewGroup(ctx context.Context) *Group {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Group{pool: p, ctx: ctx}
}

// Go schedules one named task in the group. It never blocks the caller; concurrency is bounded
// inside the goroutine by the pool. Task errors and panics are captured for Wait.
func (g *Group) Go(name string, task func(context.Context) error) {
	if task == nil {
		return
	}
	if err := g.pool.begin(); err != nil {
		g.record(err)
		return
	}
	g.wg.Add(1)
	go func() {
		defer g.pool.finish()
		defer g.wg.Done()
		release, err := g.pool.gate(g.ctx)
		if err != nil {
			g.record(err)
			return
		}
		defer release()
		defer func() {
			if recovered := recover(); recovered != nil {
				wrapped := apperror.Wrap(apperror.CodeInternal, "线程池任务发生 panic", fmt.Errorf("%v\n%s", recovered, debug.Stack()))
				g.pool.logger.Error(g.ctx, "线程池任务发生 panic", wrapped, applog.Fields{"pool": g.pool.name, "task": name})
				g.record(wrapped)
			}
		}()
		if err := task(g.ctx); err != nil {
			g.record(err)
		}
	}()
}

// Wait blocks until every scheduled task has finished and returns their joined error, if any.
func (g *Group) Wait() error {
	g.wg.Wait()
	g.mu.Lock()
	defer g.mu.Unlock()
	return errors.Join(g.err...)
}

func (g *Group) record(err error) {
	if err == nil {
		return
	}
	g.mu.Lock()
	g.err = append(g.err, err)
	g.mu.Unlock()
}

// Map applies fn to every item concurrently on the pool and returns the outputs in input order.
//
// It is the primary fan-out helper: fn is invoked exactly once per item, at most Pool.Limit at a
// time, and the caller receives results aligned with items. A nil pool (or empty input) falls back
// to sequential execution so callers always get a fully populated slice. Panics inside fn are
// recovered and leave that slot's zero value.
func Map[Input any, Output any](ctx context.Context, pool *Pool, items []Input, fn func(context.Context, Input) Output) []Output {
	results := make([]Output, len(items))
	if fn == nil || len(items) == 0 {
		return results
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		for index, item := range items {
			results[index] = fn(ctx, item)
		}
		return results
	}
	group := pool.NewGroup(ctx)
	for index := range items {
		index, item := index, items[index]
		group.Go(fmt.Sprintf("map#%d", index), func(taskCtx context.Context) error {
			results[index] = fn(taskCtx, item)
			return nil
		})
	}
	_ = group.Wait()
	return results
}

// ForEach runs fn over every item concurrently on the pool and returns their joined error.
func ForEach[Input any](ctx context.Context, pool *Pool, items []Input, fn func(context.Context, Input) error) error {
	if fn == nil || len(items) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		var joined []error
		for _, item := range items {
			if err := fn(ctx, item); err != nil {
				joined = append(joined, err)
			}
		}
		return errors.Join(joined...)
	}
	group := pool.NewGroup(ctx)
	for index := range items {
		item := items[index]
		group.Go(fmt.Sprintf("foreach#%d", index), func(taskCtx context.Context) error {
			return fn(taskCtx, item)
		})
	}
	return group.Wait()
}
