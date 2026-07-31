package service

import (
	"context"
	"errors"
	"path"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// LifecycleManager 把持久化 Operation 与远端进程身份、已校验的 Server 状态协调起来。
type LifecycleManager struct {
	clock      model.Clock
	store      repository.Store
	operations *OperationRunner
	processes  *RemoteProcessController
	firewall   *FirewallManager
	observerMu sync.RWMutex
	observers  []serverLifecycleObserver
}

type serverLifecycleObserver interface {
	ServerStarted(model.ID)
	ServerStopped(model.ID)
}

type serverLifecycleFailureObserver interface{ ServerFailed(model.ID) }

// NewLifecycleManager 创建启动、停止、重启与恢复的应用服务。
func NewLifecycleManager(clock model.Clock, store repository.Store, operations *OperationRunner, processes *RemoteProcessController, firewall *FirewallManager) (*LifecycleManager, error) {
	if clock == nil || store == nil || operations == nil || processes == nil || firewall == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "LifecycleManager 依赖不能为空")
	}
	return &LifecycleManager{clock: clock, store: store, operations: operations, processes: processes, firewall: firewall}, nil
}

// SetObserver 在运行时组装完成后挂接采集侧的生命周期响应。
func (m *LifecycleManager) SetObserver(observer serverLifecycleObserver) {
	if observer != nil {
		m.observerMu.Lock()
		m.observers = append(m.observers, observer)
		m.observerMu.Unlock()
	}
}

// GetState 先探测持久化的进程身份,再返回 Server 生命周期状态。
func (m *LifecycleManager) GetState(ctx context.Context, serverID model.ID) (enums.LifecycleState, *model.RemoteProcessIdentity, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return "", nil, err
	}
	identity, err := m.store.ProcessIdentities().GetByServer(ctx, serverID)
	if err != nil {
		if apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
			wasRunning := server.State == enums.LifecycleRunning
			if server.State == enums.LifecycleRunning || server.State == enums.LifecycleStarting || server.State == enums.LifecycleStopping {
				server.State = enums.LifecycleFailed
				server.UpdatedAt = m.clock.Now().UTC()
				_ = m.store.MinecraftServers().Update(ctx, server)
			}
			if wasRunning {
				m.notifyServerFailed(serverID)
			}
			return server.State, nil, nil
		}
		return "", nil, err
	}
	probe, err := m.processes.Probe(ctx, *identity)
	if err != nil {
		return server.State, identity, err
	}
	identity = &probe.Identity
	previousState := server.State
	switch identity.State {
	case enums.RemoteProcessRunning:
		if server.State != enums.LifecycleRunning {
			server.State = enums.LifecycleRunning
			server.UpdatedAt = m.clock.Now().UTC()
			if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
				return "", identity, err
			}
		}
	case enums.RemoteProcessExited:
		switch server.State {
		case enums.LifecycleStopping:
			server.State = enums.LifecycleStopped
		case enums.LifecycleRunning, enums.LifecycleStarting:
			server.State = enums.LifecycleFailed
		}
		server.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
			return "", identity, err
		}
		if previousState == enums.LifecycleRunning {
			m.notifyServerFailed(serverID)
		}
	case enums.RemoteProcessMismatched:
		server.State = enums.LifecycleFailed
		server.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
			return "", identity, err
		}
		if previousState == enums.LifecycleRunning {
			m.notifyServerFailed(serverID)
		}
	}
	return server.State, identity, nil
}

func (m *LifecycleManager) notifyServerFailed(serverID model.ID) {
	for _, observer := range m.lifecycleObservers() {
		if failureObserver, ok := observer.(serverLifecycleFailureObserver); ok {
			failureObserver.ServerFailed(serverID)
		}
	}
}

func (m *LifecycleManager) lifecycleObservers() []serverLifecycleObserver {
	m.observerMu.RLock()
	defer m.observerMu.RUnlock()
	return append([]serverLifecycleObserver(nil), m.observers...)
}

// Start 异步启动一台 Server 并立即返回其 Operation ID。
func (m *LifecycleManager) Start(ctx context.Context, serverID model.ID, firewallConfirmed, tmuxInstallConfirmed bool) (model.ID, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return "", err
	}
	if server.State != enums.LifecycleStopped && server.State != enums.LifecycleReady && server.State != enums.LifecycleFailed {
		return "", apperror.New(apperror.CodeValidationConflict, "Server 当前状态不能启动")
	}
	if !server.EULAAccepted || server.JavaRuntimeID == nil {
		return "", apperror.New(apperror.CodeValidationConflict, "Server 安装、EULA 或 Java Runtime 未就绪")
	}
	if _, err := m.processes.EnsureTmux(ctx, server.SSHSessionID, tmuxInstallConfirmed); err != nil {
		return "", err
	}
	if _, err := m.firewall.PrepareStart(ctx, serverID, firewallConfirmed); err != nil {
		return "", err
	}
	return m.operations.Start(ctx, OperationRequest{
		Type: enums.OperationStart, TargetType: enums.OperationTargetServer, TargetID: serverID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.startNow(operationCtx, serverID, reporter)
		},
	})
}

// Stop 异步停止一台 Server;已停止的 Server 调用是幂等的。
func (m *LifecycleManager) Stop(ctx context.Context, serverID model.ID, force bool) (model.ID, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return "", err
	}
	if server.State == enums.LifecycleStopped {
		return "", nil
	}
	return m.operations.Start(ctx, OperationRequest{
		Type: enums.OperationStop, TargetType: enums.OperationTargetServer, TargetID: serverID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.stopNow(operationCtx, serverID, force, reporter)
		},
	})
}

// Restart 在同一个持有资源锁的 Operation 内执行可跟踪的停止与启动阶段。
func (m *LifecycleManager) Restart(ctx context.Context, serverID model.ID, tmuxInstallConfirmed bool) (model.ID, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return "", err
	}
	if _, err := m.processes.EnsureTmux(ctx, server.SSHSessionID, tmuxInstallConfirmed); err != nil {
		return "", err
	}
	return m.operations.Start(ctx, OperationRequest{
		Type: enums.OperationRestart, TargetType: enums.OperationTargetServer, TargetID: serverID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			if err := reporter.SetProgress("stop", 0.1, "正在优雅停止 Server"); err != nil {
				return err
			}
			if err := m.stopNow(operationCtx, serverID, false, reporter); err != nil {
				return err
			}
			if err := reporter.SetProgress("start", 0.55, "正在重新启动 Server"); err != nil {
				return err
			}
			return m.startNow(operationCtx, serverID, reporter)
		},
	})
}

// Recover 在应用启动后探测每个持久化的活动身份并校正 Server 状态。
func (m *LifecycleManager) Recover(ctx context.Context) error {
	identities, err := m.store.ProcessIdentities().ListActive(ctx)
	if err != nil {
		return err
	}
	for index := range identities {
		if _, _, err := m.GetState(ctx, identities[index].ServerID); err != nil {
			return err
		}
	}
	return nil
}

// Monitor 周期性校正活动身份,使异常退出无需界面打开也能转为失败状态。
func (m *LifecycleManager) Monitor(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	failures := make(map[model.ID]int)
	nextAttempts := make(map[model.ID]time.Time)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		identities, err := m.store.ProcessIdentities().ListActive(ctx)
		if err == nil {
			active := make(map[model.ID]struct{}, len(identities))
			for index := range identities {
				serverID := identities[index].ServerID
				active[serverID] = struct{}{}
				if time.Now().Before(nextAttempts[serverID]) {
					continue
				}
				if _, _, probeErr := m.GetState(ctx, serverID); probeErr != nil {
					failures[serverID]++
					delay := interval
					if failures[serverID] >= 3 {
						delay = 5 * time.Minute
					} else {
						for attempt := 1; attempt < failures[serverID]; attempt++ {
							delay *= 2
						}
					}
					nextAttempts[serverID] = time.Now().Add(delay)
					continue
				}
				delete(failures, serverID)
				delete(nextAttempts, serverID)
			}
			for serverID := range failures {
				if _, found := active[serverID]; !found {
					delete(failures, serverID)
					delete(nextAttempts, serverID)
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *LifecycleManager) startNow(ctx context.Context, serverID model.ID, reporter OperationReporter) error {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	if server.State != enums.LifecycleStopped && server.State != enums.LifecycleReady && server.State != enums.LifecycleFailed {
		return apperror.New(apperror.CodeValidationConflict, "Server 当前状态不能启动")
	}
	if err := server.Transition(m.clock, enums.LifecycleStarting); err != nil {
		return err
	}
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return err
	}
	if err := reporter.SetProgress("starting", 0.2, "远程进程正在启动并等待就绪"); err != nil {
		return err
	}
	javaRuntime, err := m.store.JavaRuntimes().Get(ctx, *server.JavaRuntimeID)
	if err != nil {
		return m.failServer(ctx, server, err)
	}
	launch := launchCommandForServer(*server, path.Join(javaRuntime.JavaHome, "bin/java"))
	profile := runtimeProfileForServerType(server.Type)
	process, err := m.processes.Start(ctx, port.ProcessLaunchSpec{
		ServerID: server.ID, SSHSessionID: server.SSHSessionID, Executable: launch.executable,
		Arguments: launch.arguments, WorkingDirectory: server.RemotePath,
		ReadyPatterns: profile.readyPatterns,
	})
	if err != nil {
		return m.failServer(ctx, server, err)
	}
	_ = process.Close()
	if err := server.Transition(m.clock, enums.LifecycleRunning); err != nil {
		return err
	}
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return err
	}
	_ = m.firewall.CleanupPending(context.WithoutCancel(ctx), serverID)
	for _, observer := range m.lifecycleObservers() {
		observer.ServerStarted(serverID)
	}
	return reporter.SetProgress("running", 1, "Server 已就绪")
}

func (m *LifecycleManager) stopNow(ctx context.Context, serverID model.ID, force bool, reporter OperationReporter) error {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	identity, err := m.store.ProcessIdentities().GetByServer(ctx, serverID)
	if err != nil {
		if apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
			server.State = enums.LifecycleStopped
			server.UpdatedAt = m.clock.Now().UTC()
			if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
				return err
			}
			for _, observer := range m.lifecycleObservers() {
				observer.ServerStopped(serverID)
			}
			return nil
		}
		return err
	}
	if server.State != enums.LifecycleStopping {
		if server.State == enums.LifecycleFailed {
			server.State = enums.LifecycleStopping
			server.UpdatedAt = m.clock.Now().UTC()
		} else if err := server.Transition(m.clock, enums.LifecycleStopping); err != nil {
			return err
		}
		if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
			return err
		}
	}
	if err := reporter.SetProgress("stopping", 0.4, "正在发送 Console stop 并等待退出"); err != nil {
		return err
	}
	if err := m.processes.Stop(ctx, *identity, force); err != nil {
		return m.failServer(ctx, server, err)
	}
	probe, err := m.processes.Probe(ctx, *identity)
	if err != nil {
		return m.failServer(ctx, server, err)
	}
	if probe.Identity.State != enums.RemoteProcessExited {
		return m.failServer(ctx, server, apperror.New(apperror.CodeProcessExitFailed, "远程进程未确认退出"))
	}
	server.State = enums.LifecycleStopped
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return err
	}
	for _, observer := range m.lifecycleObservers() {
		observer.ServerStopped(serverID)
	}
	return reporter.SetProgress("stopped", 1, "Server 已停止")
}

func (m *LifecycleManager) failServer(ctx context.Context, server *model.MinecraftServer, failure error) error {
	server.State = enums.LifecycleFailed
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(context.WithoutCancel(ctx), server); err != nil {
		return errors.Join(failure, err)
	}
	return failure
}
