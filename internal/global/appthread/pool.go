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

// Options configures a Pool's identity, concurrency ceiling, and logging boundary.
type Options struct {
	// Name labels the pool in logs and task boundaries. Defaults to "appthread".
	Name string
	// MaxConcurrency bounds simultaneously running tasks. Non-positive selects a CPU-based default.
	MaxConcurrency int
	// Logger receives panic and lifecycle records. Defaults to applog.Default().
	Logger *applog.Logger
}

// Stats is a point-in-time snapshot of pool utilization for diagnostics.
type Stats struct {
	Limit   int    `json:"limit"`
	Running int64  `json:"running"`
	Started uint64 `json:"started"`
}

// Pool is a bounded, panic-safe goroutine execution boundary shared across MineOps.
//
// The zero value is not usable; construct one with NewPool. Pool is safe for concurrent use.
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

// NewPool creates a ready-to-use bounded pool, clamping MaxConcurrency into a safe range.
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

// Name returns the pool label used in logs and task boundaries.
func (p *Pool) Name() string {
	if p == nil {
		return ""
	}
	return p.name
}

// Limit returns the maximum number of simultaneously running tasks.
func (p *Pool) Limit() int {
	if p == nil {
		return 0
	}
	return cap(p.tokens)
}

// Stats reports current pool utilization for diagnostics and monitoring surfaces.
func (p *Pool) Stats() Stats {
	if p == nil {
		return Stats{}
	}
	return Stats{Limit: cap(p.tokens), Running: p.running.Load(), Started: p.started.Load()}
}

// Go schedules one fire-and-forget task, blocking only long enough to register it.
//
// The task runs once a concurrency slot is free. A panic inside task is recovered and logged
// at the "<pool>.<name>" boundary so it can never crash the process. Go returns an error only
// when the pool is closing or its arguments are invalid; the task's own outcome is not reported.
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

// Close stops accepting new work and waits for in-flight tasks to finish or ctx to expire.
//
// Close is idempotent and always waits for outstanding tasks even when called concurrently.
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

// begin reserves the task in the pool lifecycle if the pool still accepts work.
func (p *Pool) begin() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.accepting {
		return apperror.New(apperror.CodeValidationConflict, "线程池正在关闭，不再接受任务")
	}
	p.wait.Add(1)
	return nil
}

// finish releases the lifecycle reservation taken by begin.
func (p *Pool) finish() {
	p.wait.Done()
}

// gate blocks until a concurrency slot is free and returns a single-use release function.
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
