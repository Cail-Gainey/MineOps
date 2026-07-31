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

// Group 在一个 Pool 上运行一批相关任务,并等待它们整体完成。
//
// 每个任务共享该组的 Context 与线程池的并发预算。错误与被恢复的 panic 会被收集,
// 由 Wait 一并返回。Wait 之后不得复用同一个 Group。
type Group struct {
	pool *Pool
	ctx  context.Context

	wg  sync.WaitGroup
	mu  sync.Mutex
	err []error
}

// NewGroup 启动一批绑定到 ctx 与该线程池并发预算的任务。
func (p *Pool) NewGroup(ctx context.Context) *Group {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Group{pool: p, ctx: ctx}
}

// Go 在组内调度一个具名任务。它绝不阻塞调用方;并发在 goroutine 内部由线程池限制。
// 任务的错误与 panic 会被捕获,供 Wait 汇总。
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

// Wait 阻塞至全部已调度任务结束,并返回合并后的错误(若有)。
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

// Map 在线程池上对每个元素并发执行 fn,并按输入顺序返回结果。
//
// 它是主要的扇出辅助函数:fn 对每个元素恰好调用一次,同时最多 Pool.Limit 个并发,
// 调用方拿到的结果与输入一一对应。线程池为 nil(或输入为空)时回落到顺序执行,
// 因此调用方总能拿到完整填充的切片。fn 内部的 panic 会被恢复,
// 对应位置保留零值。
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

// ForEach 在线程池上对每个元素并发执行 fn,并返回合并后的错误。
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
