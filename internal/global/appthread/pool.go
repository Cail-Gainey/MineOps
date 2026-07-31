package appthread

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

const (
	minConcurrency = 1
	maxConcurrency = 256
)

// Options 配置线程池的标识、并发上限与日志边界。
type Options struct {
	// Name 在日志与任务边界中标识该线程池,默认为 "appthread"。
	Name string
	// MaxConcurrency 限制同时运行的任务数,非正值表示按 CPU 数取默认值。
	MaxConcurrency int
	// Logger 接收 panic 与生命周期记录,默认为 applog.Default()。
	Logger *applog.Logger
}

// Stats 是用于诊断的线程池占用瞬时快照。
type Stats struct {
	Limit   int    `json:"limit"`
	Running int64  `json:"running"`
	Started uint64 `json:"started"`
}

// Pool 是 MineOps 全局共用、有界且 panic 安全的 goroutine 执行边界。
//
// 零值不可用,必须用 NewPool 构造。Pool 可安全并发使用。
type Pool struct {
	name   string
	logger *applog.Logger
	tokens chan struct{}

	mu        sync.Mutex
	accepting bool

	wait    sync.WaitGroup
	running atomic.Int64
	started atomic.Uint64
}

// NewPool 创建一个可直接使用的有界线程池,并把 MaxConcurrency 收敛到安全区间。
func NewPool(options Options) *Pool {
	limit := options.MaxConcurrency
	if limit <= 0 {
		limit = defaultConcurrency()
	}
	if limit < minConcurrency {
		limit = minConcurrency
	}
	if limit > maxConcurrency {
		limit = maxConcurrency
	}
	logger := options.Logger
	if logger == nil {
		logger = applog.Default()
	}
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = "appthread"
	}
	return &Pool{
		name:      name,
		logger:    logger,
		tokens:    make(chan struct{}, limit),
		accepting: true,
	}
}

func defaultConcurrency() int {
	limit := runtime.GOMAXPROCS(0) * 4
	if limit < 8 {
		limit = 8
	}
	if limit > maxConcurrency {
		limit = maxConcurrency
	}
	return limit
}

// Name 返回日志与任务边界中使用的线程池标签。
func (p *Pool) Name() string {
	if p == nil {
		return ""
	}
	return p.name
}

// Limit 返回同时运行任务数的上限。
func (p *Pool) Limit() int {
	if p == nil {
		return 0
	}
	return cap(p.tokens)
}

// Stats 汇报当前线程池占用,供诊断与监控界面使用。
func (p *Pool) Stats() Stats {
	if p == nil {
		return Stats{}
	}
	return Stats{Limit: cap(p.tokens), Running: p.running.Load(), Started: p.started.Load()}
}

// Go 调度一个发后不理的任务,仅在登记期间短暂阻塞调用方。
//
// 任务在有空闲并发槽位后开始执行。任务内部的 panic 会在 "<pool>.<name>" 边界被恢复并记录,
// 因此绝不会让进程崩溃。Go 只在线程池正在关闭或参数非法时返回错误,
// 不汇报任务自身的执行结果。
func (p *Pool) Go(ctx context.Context, name string, task func(context.Context)) error {
	if p == nil {
		return apperror.New(apperror.CodeValidationRequired, "线程池不能为空")
	}
	if task == nil {
		return apperror.New(apperror.CodeValidationRequired, "线程池任务不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := p.begin(); err != nil {
		return err
	}
	go func() {
		defer p.finish()
		release, err := p.gate(ctx)
		if err != nil {
			p.logger.Warn(ctx, "线程池任务在获取配额前已取消", applog.Fields{"pool": p.name, "task": name})
			return
		}
		defer release()
		var recovered error
		defer apperror.Recover(ctx, p.logger, p.name+"."+name, &recovered)
		task(ctx)
	}()
	return nil
}

// Close 停止接受新任务,并等待进行中的任务结束或 ctx 超时。
//
// Close 是幂等的,即使并发调用也始终会等待未完成的任务。
func (p *Pool) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	p.accepting = false
	p.mu.Unlock()

	done := make(chan struct{})
	go func() {
		p.wait.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return apperror.Wrap(apperror.CodeProcessCancelled, "等待线程池任务退出超时", ctx.Err())
	}
}

// begin 在线程池仍接受任务时,于其生命周期中预留该任务。
func (p *Pool) begin() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.accepting {
		return apperror.New(apperror.CodeValidationConflict, "线程池正在关闭，不再接受任务")
	}
	p.wait.Add(1)
	return nil
}

// finish 释放 begin 占用的生命周期预留。
func (p *Pool) finish() {
	p.wait.Done()
}

// gate 阻塞至有空闲并发槽位,并返回一次性的释放函数。
func (p *Pool) gate(ctx context.Context) (func(), error) {
	select {
	case p.tokens <- struct{}{}:
		p.running.Add(1)
		p.started.Add(1)
		var once sync.Once
		return func() {
			once.Do(func() {
				p.running.Add(-1)
				<-p.tokens
			})
		}, nil
	case <-ctx.Done():
		return nil, apperror.Wrap(apperror.CodeProcessCancelled, "等待线程池配额时已取消", ctx.Err())
	}
}
