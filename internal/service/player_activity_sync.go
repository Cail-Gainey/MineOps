package service

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// ServerStarted 调度远端玩家活动守护进程的替换与同步。
func (m *PlayerActivityManager) ServerStarted(serverID model.ID) {
	if m == nil || !serverID.Valid() {
		return
	}
	select {
	case m.started <- serverID:
	default:
	}
}

// ServerStopped 调度守护进程停止与一次服务器边界的会话校正。
func (m *PlayerActivityManager) ServerStopped(serverID model.ID) {
	if m == nil || !serverID.Valid() {
		return
	}
	select {
	case m.stopped <- serverID:
	default:
	}
}

// ServerFailed 在进程异常退出后调度一次估算式的会话校正。
func (m *PlayerActivityManager) ServerFailed(serverID model.ID) {
	if m == nil || !serverID.Valid() {
		return
	}
	select {
	case m.failed <- serverID:
	default:
	}
}

// Run 持续同步运行中的 Server,无需桌面页面处于打开状态。
func (m *PlayerActivityManager) Run(ctx context.Context) error {
	if err := m.Bootstrap(ctx); err != nil && ctx.Err() == nil {
		m.logger.Warn(ctx, "玩家活动启动 Bootstrap 失败", applog.Fields{"error": err.Error()})
	}
	ticker := time.NewTicker(playerSynchronizationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case serverID := <-m.started:
			m.dispatchSync(ctx, serverID, true)
		case serverID := <-m.stopped:
			m.dispatchBoundary(ctx, serverID, enums.PlayerCloseServerStopped, enums.PlayerAccuracyServerBoundary)
		case serverID := <-m.failed:
			m.dispatchBoundary(ctx, serverID, enums.PlayerCloseUnexpectedExit, enums.PlayerAccuracyEstimated)
		case <-ticker.C:
			m.syncRunning(ctx)
		}
	}
}

// Bootstrap 为桌面启动时已在运行的 Server 同步当前的进程身份证据。
func (m *PlayerActivityManager) Bootstrap(ctx context.Context) error {
	servers, err := m.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{State: enums.LifecycleRunning, Limit: 500})
	if err != nil {
		return err
	}
	var first error
	for _, server := range servers {
		if err := m.SynchronizeDirectory(ctx, server.ID); err != nil && first == nil {
			first = err
		}
		if err := m.syncServer(ctx, server.ID); err != nil && first == nil {
			first = err
		}
		if err := m.expireServerBans(ctx, server.ID); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// SynchronizeServer 立即认领并应用某台 Server 的待处理活动数据。
func (m *PlayerActivityManager) SynchronizeServer(ctx context.Context, serverID model.ID) error {
	if !serverID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家活动 Server ID 无效")
	}
	if !m.beginSync(serverID) {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 的玩家活动正在同步").WithRetryable(true)
	}
	defer m.endSync(serverID)
	select {
	case m.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-m.slots }()
	return m.syncServer(ctx, serverID)
}

func (m *PlayerActivityManager) syncRunning(ctx context.Context) {
	servers, err := m.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{State: enums.LifecycleRunning, Limit: 500})
	if err != nil {
		m.logger.Warn(ctx, "查询运行中 Server 以同步玩家活动失败", applog.Fields{"error": err.Error()})
		return
	}
	for _, server := range servers {
		m.dispatchSync(ctx, server.ID, false)
	}
}

func (m *PlayerActivityManager) dispatchSync(ctx context.Context, serverID model.ID, force bool) {
	if !m.beginSync(serverID) {
		return
	}
	go func() {
		defer m.endSync(serverID)
		select {
		case m.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-m.slots }()
		if !force && !m.retryAllowed(serverID) {
			return
		}
		if err := m.syncServer(ctx, serverID); err != nil {
			m.noteSyncFailure(serverID)
			if ctx.Err() == nil {
				m.logger.Warn(ctx, "同步玩家活动失败", applog.Fields{"server_id": serverID.String(), "error": err.Error()})
			}
		} else if err := m.expireServerBans(ctx, serverID); err != nil {
			m.noteSyncFailure(serverID)
			if ctx.Err() == nil {
				m.logger.Warn(ctx, "自动解除到期玩家封禁失败", applog.Fields{"server_id": serverID.String(), "error": err.Error()})
			}
		} else {
			m.noteSyncSuccess(serverID)
		}
	}()
}

func (m *PlayerActivityManager) dispatchBoundary(ctx context.Context, serverID model.ID, reason enums.PlayerSessionCloseReason, accuracy enums.PlayerActivityAccuracy) {
	if !m.beginSync(serverID) {
		return
	}
	go func() {
		defer m.endSync(serverID)
		select {
		case m.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-m.slots }()
		identity, err := m.store.ProcessIdentities().GetByServer(context.WithoutCancel(ctx), serverID)
		if err != nil {
			return
		}
		_ = m.stopRemoteActivityDaemon(context.WithoutCancel(ctx), *identity)
		if reconcileErr := m.ReconcileServer(context.WithoutCancel(ctx), serverID, identity.ID, m.clock.Now().UTC(), reason, accuracy); reconcileErr != nil {
			m.logger.Warn(context.WithoutCancel(ctx), "玩家活动生命周期结算失败", applog.Fields{"server_id": serverID.String(), "error": reconcileErr.Error()})
		}
	}()
}

func (m *PlayerActivityManager) beginSync(serverID model.ID) bool {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	if m.syncing[serverID] {
		return false
	}
	m.syncing[serverID] = true
	return true
}
func (m *PlayerActivityManager) endSync(serverID model.ID) {
	m.syncMu.Lock()
	delete(m.syncing, serverID)
	m.syncMu.Unlock()
}
func (m *PlayerActivityManager) retryAllowed(serverID model.ID) bool {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	return time.Now().UTC().After(m.nextAttempt[serverID])
}
func (m *PlayerActivityManager) noteSyncFailure(serverID model.ID) {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	m.failures[serverID]++
	delay := playerSynchronizationInterval
	for i := 1; i < m.failures[serverID] && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	m.nextAttempt[serverID] = time.Now().UTC().Add(delay)
}
func (m *PlayerActivityManager) noteSyncSuccess(serverID model.ID) {
	m.syncMu.Lock()
	delete(m.failures, serverID)
	delete(m.nextAttempt, serverID)
	m.syncMu.Unlock()
}

func (m *PlayerActivityManager) syncServer(ctx context.Context, serverID model.ID) error {
	if m.clients == nil || m.processes == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家活动远端同步依赖不能为空")
	}
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	if server.State != enums.LifecycleRunning {
		return nil
	}
	identity, err := m.store.ProcessIdentities().GetByServer(ctx, serverID)
	if err != nil {
		return err
	}
	probe, err := m.processes.Probe(ctx, *identity)
	if err != nil {
		return err
	}
	if probe.Identity.State != enums.RemoteProcessRunning {
		_ = m.stopRemoteActivityDaemon(context.WithoutCancel(ctx), *identity)
		return m.ReconcileServer(ctx, serverID, identity.ID, m.clock.Now().UTC(), enums.PlayerCloseUnexpectedExit, enums.PlayerAccuracyEstimated)
	}
	if status, statusErr := m.store.Players().GetCollectorStatus(ctx, serverID); statusErr == nil && status.ProcessIdentityID != nil && *status.ProcessIdentityID != identity.ID {
		if err := m.ReconcileServer(ctx, serverID, *status.ProcessIdentityID, identity.StartedAt, enums.PlayerCloseProcessReplaced, enums.PlayerAccuracyEstimated); err != nil {
			return err
		}
	}
	// 升级后可能缺少历史检查点;此时改为检查未结束会话以施加同样的边界。
	if openSessions, listErr := m.store.Players().ListSessions(ctx, repository.PlayerSessionQuery{ServerID: serverID, OpenOnly: true, Limit: 200}); listErr == nil {
		seen := make(map[model.ID]struct{})
		for _, session := range openSessions {
			if session.ProcessIdentityID == identity.ID {
				continue
			}
			if _, found := seen[session.ProcessIdentityID]; found {
				continue
			}
			seen[session.ProcessIdentityID] = struct{}{}
			if err := m.ReconcileServer(ctx, serverID, session.ProcessIdentityID, identity.StartedAt, enums.PlayerCloseProcessReplaced, enums.PlayerAccuracyEstimated); err != nil {
				return err
			}
		}
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	sshSettings := model.SSHSettings{}
	if m.settings != nil {
		sshSettings = m.settings.Snapshot().SSH
	}
	client, err := m.clients.Connect(ctx, session, sshSettings)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	dataDirectory, daemonPath, spoolPath, claimPath := playerActivityRemotePaths(*server)
	logPath := path.Join(server.RemotePath, "logs", "latest.log")
	if err := uploadRemoteCollectorFile(ctx, client, dataDirectory, daemonPath, remotePlayerActivityDaemonScript, 20*time.Second); err != nil {
		return err
	}
	interval := int(playerActivityDaemonInterval / time.Second)
	digest := playerActivityCollectorConfigDigest(*server, identity.ID.String(), strconv.Itoa(identity.PID), strconv.Itoa(identity.ProcessGroupID), logPath, interval)
	startScript := `set -eu
data_dir=$1; daemon_path=$2; server_dir=$3; log_path=$4; process_identity_id=$5; managed_pid=$6; managed_pgid=$7; interval=$8; maximum_spool_bytes=$9; config_digest=${10}
pid_path="$data_dir/player-activity.pid"; config_path="$data_dir/player-activity.config"; umask 077; mkdir -p "$data_dir"; running=0; old_pid=""
if [ -f "$pid_path" ]; then old_pid=$(cat "$pid_path" 2>/dev/null || true); fi
if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then command_line=$(tr '\000' ' ' < "/proc/$old_pid/cmdline" 2>/dev/null || true); current_config=$(cat "$config_path" 2>/dev/null || true); case "$command_line" in *"$daemon_path"*) [ "$current_config" = "$config_digest" ] && running=1 ;; esac; fi
if [ "$running" -eq 0 ]; then
  if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then command_line=$(tr '\000' ' ' < "/proc/$old_pid/cmdline" 2>/dev/null || true); case "$command_line" in *"$daemon_path"*) kill "$old_pid" 2>/dev/null || true ;; esac; fi
  config_tmp="$config_path.tmp.$$"; printf '%s\n' "$config_digest" > "$config_tmp"; mv -f "$config_tmp" "$config_path"
  if command -v nohup >/dev/null 2>&1; then
    nohup sh "$daemon_path" "$data_dir" "$server_dir" "$log_path" "$process_identity_id" "$managed_pid" "$managed_pgid" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
  else
    sh "$daemon_path" "$data_dir" "$server_dir" "$log_path" "$process_identity_id" "$managed_pid" "$managed_pgid" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
  fi
  printf '%s\n' "$!" > "$pid_path"
fi`
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", startScript, "mineops", dataDirectory, daemonPath, server.RemotePath, logPath, identity.ID.String(), strconv.Itoa(identity.PID), strconv.Itoa(identity.ProcessGroupID), strconv.Itoa(interval), strconv.Itoa(remotePlayerActivitySpoolBytes), digest}, Timeout: 20 * time.Second, MaximumOutput: playerSynchronizationMaximumOutput}); err != nil {
		return err
	}
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", playerActivityCollectorClaimScript(), "mineops", claimPath, spoolPath}, Timeout: 30 * time.Second, MaximumOutput: playerSynchronizationMaximumOutput})
	if err != nil {
		return err
	}
	if result.Truncated {
		return apperror.New(apperror.CodeProcessExitFailed, "远端玩家活动 Claim 超过安全读取上限")
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return nil
	}
	events, _, err := NewPlayerActivityParser().ParseRemoteBatch(result.Stdout, server.ID, identity.ID, m.clock.Now().UTC())
	if err != nil {
		return err
	}
	adjustPlayerEventClockSkew(ctx, client, events, m.clock.Now().UTC())
	dropped := playerActivityDroppedCount(result.Stdout)
	if len(events) == 0 && dropped > 0 {
		if err := m.MarkIncomplete(ctx, server.ID, &identity.ID, dropped, "远端玩家活动 Spool 发生裁剪"); err != nil {
			return err
		}
	} else if err := m.Ingest(ctx, events, dropped); err != nil {
		return err
	}
	// 恢复出的 Claim 可能属于上一个进程周期;把该证据保留在旧身份上,
	// 但把采集器检查点推进到当前受管周期,避免反复校正。
	if status, statusErr := m.store.Players().GetCollectorStatus(ctx, server.ID); statusErr == nil {
		status.ProcessIdentityID = &identity.ID
		status.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.Players().SaveCollectorStatus(ctx, status); err != nil {
			return err
		}
	}
	_, err = client.RunCommand(ctx, RemoteCommand{Executable: "rm", Arguments: []string{"-f", claimPath}, Timeout: 10 * time.Second, MaximumOutput: 4096})
	return err
}

func adjustPlayerEventClockSkew(ctx context.Context, client *SSHClient, events []model.PlayerActivityEvent, localNow time.Time) {
	if client == nil || len(events) == 0 {
		return
	}
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "date", Arguments: []string{"+%s"}, Timeout: 5 * time.Second, MaximumOutput: 128})
	if err != nil {
		return
	}
	remoteEpoch, err := strconv.ParseInt(strings.TrimSpace(result.Stdout), 10, 64)
	if err != nil || remoteEpoch <= 0 {
		return
	}
	skew := localNow.Sub(time.Unix(remoteEpoch, 0).UTC())
	if absDuration(skew) <= 2*time.Second || absDuration(skew) > 24*time.Hour {
		return
	}
	for index := range events {
		events[index].ObservedAt = events[index].ObservedAt.Add(skew).UTC()
	}
}

func (m *PlayerActivityManager) stopRemoteActivityDaemon(ctx context.Context, identity model.RemoteProcessIdentity) error {
	server, err := m.store.MinecraftServers().Get(ctx, identity.ServerID, false)
	if err != nil {
		return err
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	sshSettings := model.SSHSettings{}
	if m.settings != nil {
		sshSettings = m.settings.Snapshot().SSH
	}
	client, err := m.clients.Connect(ctx, session, sshSettings)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	dataDirectory, daemonPath, _, _ := playerActivityRemotePaths(*server)
	_, err = client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", playerActivityCollectorStopScript(), "mineops", path.Join(dataDirectory, "player-activity.pid"), daemonPath, path.Join(dataDirectory, "player-activity.config")}, Timeout: 15 * time.Second, MaximumOutput: 4096})
	return err
}

var _ serverLifecycleObserver = (*PlayerActivityManager)(nil)
