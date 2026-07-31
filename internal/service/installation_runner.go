package service

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appthread"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// InstallationStepReporter 持久化每个步骤的进度、消息与日志游标。
type InstallationStepReporter interface {
	SetProgress(float64, string, int64) error
}

// InstallationStepHandler 执行一个幂等安装步骤并返回其持久化检查点。
type InstallationStepHandler func(context.Context, *InstallationSession, model.InstallationTask, model.InstallationStep, InstallationStepReporter) (map[string]any, error)

// InstallationStartResult 承载立即返回给 Wails 的持久化任务与 Operation 标识。
type InstallationStartResult struct {
	TaskID      model.ID `json:"taskID"`
	OperationID model.ID `json:"operationID"`
}

// InstallationRunner 把持久化检查点与 OperationRunner 的取消及资源锁协调起来。
type InstallationRunner struct {
	clock      model.Clock
	store      repository.Store
	operations *OperationRunner
	threads    *appthread.Pool

	mu       sync.RWMutex
	handlers map[string]InstallationStepHandler
	sessions InstallationSessionFactory

	// progressMu 串行化并发步骤的进度上报:reporter 共享同一个 *InstallationTask,
	// 且 UpdateTask 是整行覆盖,不加锁会同时产生数据竞争与字段互相冲掉。
	progressMu sync.Mutex
}

// NewInstallationRunner 创建持久化安装队列的所有者。
//
// threads 限制单个安装波次内的并发步骤数;池为 nil 时回落到 appthread.Default()。
func NewInstallationRunner(clock model.Clock, store repository.Store, operations *OperationRunner, threads *appthread.Pool) (*InstallationRunner, error) {
	if clock == nil || store == nil || operations == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "InstallationRunner 依赖不能为空")
	}
	if threads == nil {
		threads = appthread.Default()
	}
	return &InstallationRunner{
		clock: clock, store: store, operations: operations, threads: threads,
		handlers: make(map[string]InstallationStepHandler),
	}, nil
}

// SetSessionFactory 安装 execute 所用的、按任务共享的 SSH 会话工厂。
func (r *InstallationRunner) SetSessionFactory(factory InstallationSessionFactory) error {
	if factory == nil {
		return apperror.New(apperror.CodeValidationRequired, "Installation Session Factory 不能为空")
	}
	r.mu.Lock()
	r.sessions = factory
	r.mu.Unlock()
	return nil
}

// RegisterStep 替换一个具名幂等步骤的实现。
func (r *InstallationRunner) RegisterStep(name string, handler InstallationStepHandler) error {
	if name == "" || handler == nil {
		return apperror.New(apperror.CodeValidationRequired, "Installation Step 名称和 Handler 不能为空")
	}
	r.mu.Lock()
	r.handlers[name] = handler
	r.mu.Unlock()
	return nil
}

// Start 持久化任务与步骤、启动异步 Operation 并立即返回。
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

// Retry 启动一个新的 Operation,只执行失败、已取消或等待中的步骤。
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

// Cancel 通过进行中 Operation 的上下文传播取消。
func (r *InstallationRunner) Cancel(ctx context.Context, operationID model.ID) error {
	return r.operations.Cancel(ctx, operationID)
}

// RecoverInterrupted 把被进程中断的任务与运行中步骤转换成可重试的失败检查点。
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

// installationWaves 把可以并发执行的步骤分组。
//
// 每个波次内的步骤彼此没有依赖,波次之间严格有序。划分同时满足两类约束:
//   - checkpoint 前驱边:install_java 依赖 resolve_java、install_server 依赖 download_server 等
//   - 隐式边:目录必须先建、jar 必须先到、eula 与端口必须先就绪才能首启
//
// 另有一条容易忽略的约束:MinecraftServer 行是整行覆盖更新(Select("*")),
// 而 resolve_java、install_java、write_eula 都写这一行,因此每个波次里最多只允许一个行写者。
var installationWaves = map[string]int{
	"connect_ssh":             1,
	"initialize_directories":  2,
	"create_server_directory": 3,
	"resolve_java":            3,
	"install_java":            4,
	"download_server":         4,
	"install_server":          5,
	"write_eula":              5,
	// 放在首启前一个波次,而不是更早:提前放行端口会让"端口已开但没有进程监听"的窗口覆盖整段下载。
	"configure_firewall": 5,
	"first_start":        6,
	"register_server":    7,
}

// installationWaveFor 返回步骤所属波次;未知步骤回落到其顺序号,从而保持完全串行。
func installationWaveFor(step model.InstallationStep) int {
	if wave, ok := installationWaves[step.Name]; ok {
		return wave
	}
	return len(installationWaves) + step.Order
}

// stepOutcome 把一个并发步骤的结果回传给波次调度器。
//
// done 只在任务体跑到最后一行时置位。handler panic、线程池拒绝配额、或波次在取得配额前被取消时,
// appthread.Group 会把错误记在 Wait 的返回值里而任务体一行都不执行 —— 此时 err 仍是 nil,
// 只有 done 能区分"成功"和"根本没跑"。
type stepOutcome struct {
	step       *model.InstallationStep
	checkpoint map[string]any
	err        error
	done       bool
}

func (r *InstallationRunner) execute(ctx context.Context, taskID model.ID, operationReporter OperationReporter) error {
	task, steps, err := r.store.Installations().Get(ctx, taskID)
	if err != nil {
		return err
	}
	r.mu.RLock()
	factory := r.sessions
	r.mu.RUnlock()
	if factory == nil {
		return apperror.New(apperror.CodeValidationConflict, "Installation Session Factory 尚未注册")
	}
	if err := task.Start(r.clock); err != nil {
		return err
	}
	if err := r.store.Installations().UpdateTask(ctx, task); err != nil {
		return err
	}
	remote := factory(task.ServerID)
	defer func() { _ = remote.Close() }()

	for _, wave := range r.pendingWaves(steps) {
		pending := make([]*model.InstallationStep, 0, len(steps))
		for index := range steps {
			step := &steps[index]
			if installationWaveFor(*step) != wave {
				continue
			}
			if step.State == enums.InstallationStepSuccess || step.State == enums.InstallationStepSkipped {
				continue
			}
			if step.State != enums.InstallationStepWaiting && step.State != enums.InstallationStepFailed {
				return apperror.New(apperror.CodeValidationConflict, "Installation Step 状态不能执行").WithDetails(map[string]any{
					"step": step.Name, "state": step.State,
				})
			}
			pending = append(pending, step)
		}
		if len(pending) == 0 {
			continue
		}
		if ctx.Err() != nil {
			return r.finishCancelled(task, nil, ctx.Err())
		}
		if err := r.assertHandlers(pending); err != nil {
			return r.finishFailed(task, pending, err)
		}
		if err := r.startWave(ctx, task, steps, pending); err != nil {
			return r.finishFailed(task, pending, err)
		}
		outcomes, waveErr := r.runWave(ctx, remote, task, steps, pending, operationReporter)
		if waveErr != nil {
			return r.finishWaveFailure(ctx, task, steps, outcomes, waveErr)
		}
		if err := r.commitWave(ctx, task, steps, outcomes, operationReporter); err != nil {
			return r.finishFailed(task, pending, err)
		}
	}

	// task.Complete 先作用在副本上:只有收尾事务成功才提交,失败时 task 仍是 Running,finishFailed 才能生效。
	completed := *task
	if err := completed.Complete(r.clock, enums.InstallationSucceeded); err != nil {
		return r.finishFailed(task, nil, err)
	}
	if err := r.finishServerInstalled(ctx, &completed); err != nil {
		return r.finishFailed(task, nil, err)
	}
	*task = completed
	return nil
}

// assertHandlers 在把任何步骤标记为运行中之前,先确认该波次每一步都有实现。
func (r *InstallationRunner) assertHandlers(pending []*model.InstallationStep) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, step := range pending {
		if r.handlers[step.Name] == nil {
			return apperror.New(apperror.CodeValidationConflict, "Installation Step 尚未注册实现").WithDetails(map[string]any{"step": step.Name})
		}
	}
	return nil
}

// pendingWaves 按升序返回仍含可执行步骤的波次号。
func (r *InstallationRunner) pendingWaves(steps []model.InstallationStep) []int {
	seen := make(map[int]bool, len(steps))
	waves := make([]int, 0, len(steps))
	for index := range steps {
		step := steps[index]
		if step.State == enums.InstallationStepSuccess || step.State == enums.InstallationStepSkipped {
			continue
		}
		wave := installationWaveFor(step)
		if seen[wave] {
			continue
		}
		seen[wave] = true
		waves = append(waves, wave)
	}
	sort.Ints(waves)
	return waves
}

// startWave 把某个波次的全部步骤标记为运行中,并重新发布推导出的任务进度。
func (r *InstallationRunner) startWave(ctx context.Context, task *model.InstallationTask, steps []model.InstallationStep, pending []*model.InstallationStep) error {
	r.progressMu.Lock()
	defer r.progressMu.Unlock()
	for _, step := range pending {
		if err := step.Start(r.clock); err != nil {
			return err
		}
		if err := r.store.Installations().UpdateStep(ctx, step); err != nil {
			return err
		}
	}
	task.CurrentStep = pendingStepOrder(steps)
	task.UpdatedAt = r.clock.Now().UTC()
	return r.store.Installations().UpdateTask(ctx, task)
}

// runWave 在共享线程池上并发执行一个波次,并收集每个步骤的结果。
func (r *InstallationRunner) runWave(ctx context.Context, remote *InstallationSession, task *model.InstallationTask, steps []model.InstallationStep, pending []*model.InstallationStep, operationReporter OperationReporter) ([]stepOutcome, error) {
	waveCtx, cancelWave := context.WithCancel(ctx)
	defer cancelWave()
	outcomes := make([]stepOutcome, len(pending))
	group := r.threads.NewGroup(waveCtx)
	// handler 接收的是 task/step 的值拷贝。这份拷贝必须在锁内取:
	// 并发兄弟步骤的 reporter 会通过同一个 *task 指针写 LogCursor/UpdatedAt,锁外解引用就是数据竞争。
	r.progressMu.Lock()
	taskSnapshot := *task
	stepSnapshots := make([]model.InstallationStep, len(pending))
	for index, step := range pending {
		stepSnapshots[index] = snapshotStep(*step)
	}
	r.progressMu.Unlock()
	for index := range pending {
		index, step := index, pending[index]
		r.mu.RLock()
		handler := r.handlers[step.Name]
		r.mu.RUnlock()
		outcomes[index] = stepOutcome{step: step}
		if handler == nil {
			outcomes[index].err = apperror.New(apperror.CodeValidationConflict, "Installation Step 尚未注册实现").WithDetails(map[string]any{"step": step.Name})
			cancelWave()
			continue
		}
		reporter := &installationStepReporter{runner: r, task: task, step: step, steps: steps, operation: operationReporter}
		group.Go("installation."+step.Name, func(taskCtx context.Context) error {
			checkpoint, executionErr := handler(taskCtx, remote, taskSnapshot, stepSnapshots[index], reporter)
			outcomes[index].checkpoint = checkpoint
			outcomes[index].err = executionErr
			outcomes[index].done = true
			if executionErr != nil {
				// 同波次的兄弟步骤立即取消;成功的兄弟仍会按 Success 落库,不会白跑。
				cancelWave()
			}
			return executionErr
		})
	}
	if err := group.Wait(); err != nil {
		return outcomes, err
	}
	for _, outcome := range outcomes {
		if outcome.err != nil {
			return outcomes, outcome.err
		}
		if !outcome.done {
			return outcomes, apperror.New(apperror.CodeInternal, "Installation Step 未执行完成").WithDetails(map[string]any{"step": outcome.step.Name})
		}
	}
	return outcomes, nil
}

// snapshotStep 为 handler 输入复制步骤,并克隆检查点映射,避免 handler 别名到活动状态。
func snapshotStep(step model.InstallationStep) model.InstallationStep {
	if step.Checkpoint == nil {
		return step
	}
	cloned := make(map[string]any, len(step.Checkpoint))
	for key, value := range step.Checkpoint {
		cloned[key] = value
	}
	step.Checkpoint = cloned
	return step
}

// commitWave 持久化一个波次中全部成功的步骤,并重新发布推导出的任务进度。
func (r *InstallationRunner) commitWave(ctx context.Context, task *model.InstallationTask, steps []model.InstallationStep, outcomes []stepOutcome, operationReporter OperationReporter) error {
	r.progressMu.Lock()
	for _, outcome := range outcomes {
		if err := outcome.step.Complete(r.clock, enums.InstallationStepSuccess, outcome.checkpoint, nil); err != nil {
			r.progressMu.Unlock()
			return err
		}
		if err := r.store.Installations().UpdateStep(ctx, outcome.step); err != nil {
			r.progressMu.Unlock()
			return err
		}
	}
	task.CurrentStep = pendingStepOrder(steps)
	task.UpdatedAt = r.clock.Now().UTC()
	if err := r.store.Installations().UpdateTask(ctx, task); err != nil {
		r.progressMu.Unlock()
		return err
	}
	stage := outcomes[0].step.Name
	highest := outcomes[0].step.Order
	for _, outcome := range outcomes {
		if outcome.step.Order > highest {
			stage, highest = outcome.step.Name, outcome.step.Order
		}
	}
	overall := overallStepProgress(steps)
	r.progressMu.Unlock()
	return operationReporter.SetProgress(stage, overall, "安装步骤已完成")
}

// finishWaveFailure 保留同波次中已成功的步骤,其余记为失败或已取消。
func (r *InstallationRunner) finishWaveFailure(ctx context.Context, task *model.InstallationTask, steps []model.InstallationStep, outcomes []stepOutcome, waveErr error) error {
	taskCancelled := errors.Is(ctx.Err(), context.Canceled)
	succeeded := make([]stepOutcome, 0, len(outcomes))
	failed := make([]*model.InstallationStep, 0, len(outcomes))
	cancelled := make([]*model.InstallationStep, 0, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.step == nil {
			continue
		}
		switch {
		case outcome.err == nil && outcome.done:
			succeeded = append(succeeded, outcome)
		case outcome.err != nil && !errors.Is(outcome.err, context.Canceled) && !taskCancelled:
			failed = append(failed, outcome.step)
		default:
			// 被连带取消、被线程池拒绝、或 handler panic 的步骤都归这里:标 Cancelled 保持 Retryable,
			// 绝不能当成 Success —— Success 不可重试,会让整个任务永远卡在证据不完整上。
			cancelled = append(cancelled, outcome.step)
		}
	}
	// 已经跑完的兄弟按 Success 落库并保留 checkpoint:它可能是一次刚下完的 JDK,
	// 标成 Cancelled 会让 Retry 重新下载几百 MB(install_java 的脚本没有缓存短路)。
	r.progressMu.Lock()
	for _, outcome := range succeeded {
		if err := outcome.step.Complete(r.clock, enums.InstallationStepSuccess, outcome.checkpoint, nil); err != nil {
			continue
		}
		_ = r.store.Installations().UpdateStep(context.Background(), outcome.step)
	}
	if len(succeeded) > 0 {
		task.CurrentStep = pendingStepOrder(steps)
		task.UpdatedAt = r.clock.Now().UTC()
		_ = r.store.Installations().UpdateTask(context.Background(), task)
	}
	r.progressMu.Unlock()
	// 被连带取消的兄弟没有自己的失败原因,标记为 Cancelled 以保持 Retryable。
	for _, step := range cancelled {
		_ = step.Complete(r.clock, enums.InstallationStepCancelled, step.Checkpoint, waveErr)
		_ = r.store.Installations().UpdateStep(context.Background(), step)
	}
	if len(failed) == 0 && taskCancelled {
		return r.finishCancelled(task, nil, waveErr)
	}
	return r.finishFailed(task, failed, waveErr)
}

// finishServerInstalled 在同一事务内提交成功的任务与处于已停止状态的 Minecraft Server。
//
// 两者必须原子:任务成功而服务器仍停留在 Installing 是没有恢复路径的状态。
func (r *InstallationRunner) finishServerInstalled(ctx context.Context, task *model.InstallationTask) error {
	finishCtx := context.WithoutCancel(ctx)
	return r.store.Transaction(finishCtx, func(registry repository.Registry) error {
		server, err := registry.MinecraftServers().Get(finishCtx, task.ServerID, false)
		if err != nil {
			return err
		}
		server.State = enums.LifecycleStopped
		server.UpdatedAt = r.clock.Now().UTC()
		if err := registry.MinecraftServers().Update(finishCtx, server); err != nil {
			return err
		}
		return registry.Installations().UpdateTask(finishCtx, task)
	})
}

// pendingStepOrder 返回尚未完成的最小顺序号;全部完成时返回步骤总数。
//
// 取"最小未完成"而不是"最小运行中":一个波次里同时有多个步骤在跑,
// 而"最大已完成序号"会随波次内步骤的 Order 分布来回跳。未完成集合只会收缩,
// 所以这个值单调不减,前端可以直接拿它算百分比。
func pendingStepOrder(steps []model.InstallationStep) int {
	lowest := 0
	for _, step := range steps {
		if step.State == enums.InstallationStepSuccess || step.State == enums.InstallationStepSkipped {
			continue
		}
		if lowest == 0 || step.Order < lowest {
			lowest = step.Order
		}
	}
	if lowest > 0 {
		return lowest
	}
	return len(steps)
}

// overallStepProgress 对各步骤进度取平均,使交错的并发上报保持单调。
func overallStepProgress(steps []model.InstallationStep) float64 {
	if len(steps) == 0 {
		return 0
	}
	total := float64(0)
	for _, step := range steps {
		switch step.State {
		case enums.InstallationStepSuccess, enums.InstallationStepSkipped:
			total += 1
		default:
			total += min(max(step.Progress, 0), 1)
		}
	}
	return total / float64(len(steps))
}

func (r *InstallationRunner) finishFailed(task *model.InstallationTask, failed []*model.InstallationStep, failure error) error {
	for _, step := range failed {
		_ = step.Complete(r.clock, enums.InstallationStepFailed, step.Checkpoint, failure)
		_ = r.store.Installations().UpdateStep(context.Background(), step)
	}
	_ = task.Complete(r.clock, enums.InstallationFailed)
	_ = r.store.Installations().UpdateTask(context.Background(), task)
	r.markServerFailed(task.ServerID)
	return failure
}

func (r *InstallationRunner) finishCancelled(task *model.InstallationTask, cancelled []*model.InstallationStep, failure error) error {
	for _, step := range cancelled {
		_ = step.Complete(r.clock, enums.InstallationStepCancelled, step.Checkpoint, failure)
		_ = r.store.Installations().UpdateStep(context.Background(), step)
	}
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
	runner    *InstallationRunner
	task      *model.InstallationTask
	step      *model.InstallationStep
	steps     []model.InstallationStep
	operation OperationReporter
}

// SetProgress 在 runner 锁保护下持久化单个步骤的进度,避免并发步骤竞态。
//
// 锁覆盖整段:task.LogCursor 是所有 reporter 共享的字段,而 UpdateTask/UpdateStep 都是整行覆盖。
// 总进度取所有步骤进度的均值,与并发上报的交错顺序无关,因此不会倒退。
func (r *installationStepReporter) SetProgress(progress float64, message string, logCursor int64) error {
	r.runner.progressMu.Lock()
	if logCursor <= r.task.LogCursor {
		logCursor = r.task.LogCursor + 1
	}
	if err := r.step.SetProgress(r.runner.clock, progress, message, logCursor); err != nil {
		r.runner.progressMu.Unlock()
		return err
	}
	if logCursor > r.task.LogCursor {
		r.task.LogCursor = logCursor
		r.task.UpdatedAt = r.runner.clock.Now().UTC()
		if err := r.runner.store.Installations().UpdateTask(context.Background(), r.task); err != nil {
			r.runner.progressMu.Unlock()
			return err
		}
	}
	if err := r.runner.store.Installations().UpdateStep(context.Background(), r.step); err != nil {
		r.runner.progressMu.Unlock()
		return err
	}
	stage := r.step.Name
	overall := overallStepProgress(r.steps)
	r.runner.progressMu.Unlock()
	return r.operation.SetProgress(stage, overall, message)
}
