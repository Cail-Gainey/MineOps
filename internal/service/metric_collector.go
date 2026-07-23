package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

//go:embed collector.sh
var collectorScript []byte

//go:embed remote_collector_daemon.sh
var remoteCollectorDaemonScript []byte

const (
	// metricCollectorConcurrency 是同时协调的远程 SSH Session 上限。
	metricCollectorConcurrency = 8
	// metricCollectorMaximumOutput 是单条控制命令允许的最大输出。
	metricCollectorMaximumOutput = 64 * 1024
	// metricCollectorErrorLimit 是采集错误摘要的最大长度。
	metricCollectorErrorLimit = 1024
	// remoteCollectorSpoolBytes 是每个远端 Server 保留的离线采集上限。
	remoteCollectorSpoolBytes = 8 * 1024 * 1024
	// remoteCollectorMaximumOutput 是单个 Server Claim 允许的最大读取输出。
	remoteCollectorMaximumOutput = remoteCollectorSpoolBytes + 64*1024
	// remoteCollectorManifestSeparator 是不参与 Shell 解释的清单字段分隔符。
	remoteCollectorManifestSeparator = byte(0x1f)
)

type remoteMetricClaim struct {
	path   string
	output string
}

type remoteMetricTarget struct {
	server                model.MinecraftServer
	jarName               string
	managedPID            string
	managedProcessGroupID string
}

type sessionCollectionRun struct {
	done       chan struct{}
	serverIDs  []model.ID
	results    map[model.ID]error
	sessionErr error
}

// CollectorStatus 描述一台服务器最近一次 SSH 拉取采集的结果状态。
type CollectorStatus struct {
	ServerID    model.ID   `json:"serverID"`
	CollectedAt *time.Time `json:"collectedAt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	LastErrorAt *time.Time `json:"lastErrorAt,omitempty"`
	Collecting  bool       `json:"collecting"`
	Paused      bool       `json:"paused"`
}

// MetricCollector 以 SSH Session 为边界协调远端共享采集器并写入 Metric 存储。
type MetricCollector struct {
	clock    model.Clock
	store    repository.Store
	settings *appsettings.Manager
	clients  *SSHClientFactory
	metrics  *MetricManager
	logger   *applog.Logger

	mu               sync.Mutex
	statuses         map[model.ID]CollectorStatus
	inflight         map[model.ID]*sessionCollectionRun
	collecting       map[model.ID]bool
	paused           map[model.ID]bool
	legacyMigrated   map[model.ID]bool
	legacyDeferred   map[model.ID]bool
	sessionActive    map[model.ID]bool
	startupSweepDone bool
	slots            chan struct{}
}

// NewMetricCollector 创建 SSH Session 级基础指标采集服务。
func NewMetricCollector(clock model.Clock, store repository.Store, settings *appsettings.Manager, clients *SSHClientFactory, metrics *MetricManager, logger *applog.Logger) (*MetricCollector, error) {
	if clock == nil || store == nil || settings == nil || clients == nil || metrics == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Metric Collector 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &MetricCollector{
		clock: clock, store: store, settings: settings, clients: clients, metrics: metrics, logger: logger,
		statuses: make(map[model.ID]CollectorStatus), inflight: make(map[model.ID]*sessionCollectionRun),
		collecting: make(map[model.ID]bool), paused: make(map[model.ID]bool), legacyMigrated: make(map[model.ID]bool),
		legacyDeferred: make(map[model.ID]bool), sessionActive: make(map[model.ID]bool), slots: make(chan struct{}, metricCollectorConcurrency),
	}, nil
}

// Run 按 Monitoring.IntervalSeconds 周期协调全部 SSH Session，直到 ctx 取消。
func (c *MetricCollector) Run(ctx context.Context) error {
	for {
		interval := c.collectionInterval()
		c.collectAll(ctx, interval)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// CollectNow 立即协调所选 Server 所属 SSH Session，并同步返回该 Server 的结果。
func (c *MetricCollector) CollectNow(ctx context.Context, serverID model.ID) error {
	return c.collectServerNow(ctx, serverID, true)
}

// ReconcileServer 立即把一个 Server 对账到共享目标清单，但不要求同步等待首个采样批次。
func (c *MetricCollector) ReconcileServer(ctx context.Context, serverID model.ID) error {
	return c.collectServerNow(ctx, serverID, false)
}

func (c *MetricCollector) collectServerNow(ctx context.Context, serverID model.ID, requireSample bool) error {
	if !serverID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	if c.IsPaused(serverID) {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 的监控采集已暂停")
	}
	server, err := c.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	if !server.SSHSessionID.Valid() {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 未绑定 SSH Session，无法采集")
	}

	for attempt := 0; attempt < 2; attempt++ {
		servers, listErr := c.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{
			SSHSessionID: server.SSHSessionID, IncludeDeleted: true, Limit: 500,
		})
		if listErr != nil {
			return listErr
		}
		targets, eligibility := c.prepareSessionTargets(ctx, servers)
		if eligibilityErr, found := eligibility[serverID]; found {
			return eligibilityErr
		}
		selected := false
		serverIDs := make([]model.ID, 0, len(targets))
		for _, target := range targets {
			serverIDs = append(serverIDs, target.server.ID)
			selected = selected || target.server.ID == serverID
		}
		if !selected {
			return apperror.New(apperror.CodeValidationConflict, "该 Server 当前不具备监控采集资格")
		}

		run, owner := c.beginSessionCollection(server.SSHSessionID, serverIDs)
		if owner {
			select {
			case c.slots <- struct{}{}:
				results, sessionErr := c.collectSession(ctx, server.SSHSessionID, servers, targets, c.collectionInterval(), serverID, requireSample)
				<-c.slots
				c.endSessionCollection(server.SSHSessionID, run, results, sessionErr)
			case <-ctx.Done():
				results := map[model.ID]error{serverID: ctx.Err()}
				c.endSessionCollection(server.SSHSessionID, run, results, ctx.Err())
			}
		} else if waitErr := waitForSessionCollection(ctx, run); waitErr != nil {
			return waitErr
		}
		if run.sessionErr != nil {
			return run.sessionErr
		}
		if result, found := run.results[serverID]; found {
			return result
		}
	}
	return apperror.New(apperror.CodeValidationConflict, "所选 Server 未进入本次 SSH Session 采集周期").WithRetryable(true)
}

// ReconcileSession 立即对账一个 SSH Session 的目标清单与共享守护进程。
func (c *MetricCollector) ReconcileSession(ctx context.Context, sshSessionID model.ID) error {
	if !sshSessionID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
	}
	for attempt := 0; attempt < 3; attempt++ {
		servers, err := c.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{
			SSHSessionID: sshSessionID, IncludeDeleted: true, Limit: 500,
		})
		if err != nil {
			return err
		}
		targets, _ := c.prepareSessionTargets(ctx, servers)
		serverIDs := make([]model.ID, 0, len(targets))
		for _, target := range targets {
			serverIDs = append(serverIDs, target.server.ID)
		}
		run, owner := c.beginSessionCollection(sshSessionID, serverIDs)
		if !owner {
			if err := waitForSessionCollection(ctx, run); err != nil {
				return err
			}
			continue
		}
		select {
		case c.slots <- struct{}{}:
			results, sessionErr := c.collectSession(ctx, sshSessionID, servers, targets, c.collectionInterval(), "", false)
			<-c.slots
			c.endSessionCollection(sshSessionID, run, results, sessionErr)
			return sessionErr
		case <-ctx.Done():
			c.endSessionCollection(sshSessionID, run, nil, ctx.Err())
			return ctx.Err()
		}
	}
	return apperror.New(apperror.CodeValidationConflict, "SSH Session 正在持续对账，请稍后重试").WithRetryable(true)
}

// Status 返回一台服务器最近一次采集状态的副本。
func (c *MetricCollector) Status(serverID model.ID) CollectorStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	status, found := c.statuses[serverID]
	if !found {
		status = CollectorStatus{ServerID: serverID}
	}
	status.Collecting = c.collecting[serverID]
	status.Paused = c.paused[serverID]
	return status
}

// Pause stops future automatic collection passes for one Server.
func (c *MetricCollector) Pause(serverID model.ID) error {
	if !serverID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	c.mu.Lock()
	c.paused[serverID] = true
	c.mu.Unlock()
	return nil
}

// Resume restores automatic collection for one Server.
func (c *MetricCollector) Resume(serverID model.ID) error {
	if !serverID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	c.mu.Lock()
	delete(c.paused, serverID)
	c.mu.Unlock()
	return nil
}

// IsPaused reports whether automatic collection is paused for one Server.
func (c *MetricCollector) IsPaused(serverID model.ID) bool {
	c.mu.Lock()
	paused := c.paused[serverID]
	c.mu.Unlock()
	return paused
}

// ClearStatus removes the last collection result for one Server.
func (c *MetricCollector) ClearStatus(serverID model.ID) {
	c.mu.Lock()
	delete(c.statuses, serverID)
	c.mu.Unlock()
}

func (c *MetricCollector) collectionInterval() time.Duration {
	interval := time.Duration(c.settings.Snapshot().Monitoring.IntervalSeconds) * time.Second
	if interval < 5*time.Second {
		return 5 * time.Second
	}
	return interval
}

func (c *MetricCollector) collectAll(ctx context.Context, interval time.Duration) {
	servers, err := c.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{IncludeDeleted: true, Limit: 500})
	if err != nil {
		c.logger.Error(ctx, "列举 Server 以采集 Metric 失败", err, applog.Fields{"component": "metric-collector"})
		return
	}
	groups := make(map[model.ID][]model.MinecraftServer)
	for _, server := range servers {
		if server.SSHSessionID.Valid() {
			groups[server.SSHSessionID] = append(groups[server.SSHSessionID], server)
		}
	}

	startupSweepSessions := make(map[model.ID]bool)
	startupSweep := c.startupSweepRequired()
	if startupSweep {
		sessions, listErr := c.store.SSHSessions().List(ctx, repository.SSHSessionQuery{Limit: 500})
		if listErr != nil {
			c.logger.Warn(ctx, "列举 SSH Session 以清理遗留采集器失败", applog.Fields{
				"component": "metric-collector", "error": listErr.Error(),
			})
		} else {
			for _, session := range sessions {
				groups[session.ID] = groups[session.ID]
				startupSweepSessions[session.ID] = true
			}
			c.markStartupSweepDone()
		}
	}
	for _, sessionID := range c.activeSessionIDs() {
		groups[sessionID] = groups[sessionID]
	}

	var wait sync.WaitGroup
	for sessionID, sessionServers := range groups {
		targets, eligibility := c.prepareSessionTargets(ctx, sessionServers)
		for serverID, eligibilityErr := range eligibility {
			if apperror.ToDTO(eligibilityErr).Code != apperror.CodeValidationConflict.String() {
				c.recordFailure(serverID, apperror.ToDTO(eligibilityErr).Message)
			}
		}
		if len(targets) == 0 && !c.isSessionActive(sessionID) && !startupSweepSessions[sessionID] {
			continue
		}
		serverIDs := make([]model.ID, 0, len(targets))
		for _, target := range targets {
			serverIDs = append(serverIDs, target.server.ID)
		}
		run, owner := c.beginSessionCollection(sessionID, serverIDs)
		if !owner {
			continue
		}
		select {
		case c.slots <- struct{}{}:
		case <-ctx.Done():
			c.endSessionCollection(sessionID, run, nil, ctx.Err())
			return
		}
		wait.Add(1)
		go func(sshSessionID model.ID, servers []model.MinecraftServer, sessionTargets []remoteMetricTarget, collection *sessionCollectionRun) {
			defer wait.Done()
			defer func() { <-c.slots }()
			results, sessionErr := c.collectSession(ctx, sshSessionID, servers, sessionTargets, interval, "", false)
			c.endSessionCollection(sshSessionID, collection, results, sessionErr)
			if sessionErr != nil && ctx.Err() == nil {
				c.logger.Warn(ctx, "SSH Session 基础指标协调失败", applog.Fields{
					"component": "metric-collector", "ssh_session_id": sshSessionID.String(), "error": sessionErr.Error(),
				})
			}
			for serverID, resultErr := range results {
				if resultErr != nil && sessionErr == nil && ctx.Err() == nil {
					c.logger.Warn(ctx, "Server 基础指标采集失败", applog.Fields{
						"component": "metric-collector", "server_id": serverID.String(), "error": resultErr.Error(),
					})
				}
			}
		}(sessionID, append([]model.MinecraftServer(nil), sessionServers...), append([]remoteMetricTarget(nil), targets...), run)
	}
	wait.Wait()
}

func (c *MetricCollector) prepareSessionTargets(ctx context.Context, servers []model.MinecraftServer) ([]remoteMetricTarget, map[model.ID]error) {
	targets := make([]remoteMetricTarget, 0, len(servers))
	errorsByServer := make(map[model.ID]error)
	for _, server := range servers {
		eligible, eligibilityErr := c.monitoringEligible(ctx, server)
		if !eligible {
			if eligibilityErr != nil {
				errorsByServer[server.ID] = eligibilityErr
			}
			continue
		}
		target := remoteMetricTarget{server: server, jarName: path.Base(server.LaunchProfile.JarPath)}
		if identity, identityErr := c.store.ProcessIdentities().GetByServer(ctx, server.ID); identityErr == nil && identity.PID > 0 && identity.State == enums.RemoteProcessRunning {
			target.managedPID = strconv.Itoa(identity.PID)
			if identity.ProcessGroupID > 0 {
				target.managedProcessGroupID = strconv.Itoa(identity.ProcessGroupID)
			}
		}
		targets = append(targets, target)
	}
	sort.Slice(targets, func(left, right int) bool {
		return targets[left].server.ID.String() < targets[right].server.ID.String()
	})
	return targets, errorsByServer
}

func (c *MetricCollector) monitoringEligible(ctx context.Context, server model.MinecraftServer) (bool, error) {
	var latestInstallationState *enums.InstallationState
	if server.State == enums.LifecycleFailed {
		tasks, err := c.store.Installations().ListByServer(ctx, server.ID, 1, 0)
		if err != nil {
			return false, err
		}
		if len(tasks) > 0 {
			state := tasks[0].State
			latestInstallationState = &state
		}
	}
	if err := automaticMonitoringEligibility(server, c.IsPaused(server.ID), latestInstallationState); err != nil {
		return false, err
	}
	return true, nil
}

func automaticMonitoringEligibility(server model.MinecraftServer, paused bool, latestInstallationState *enums.InstallationState) error {
	if server.DeletedAt != nil || server.State == enums.LifecycleDeleted {
		return apperror.New(apperror.CodeValidationConflict, "已删除 Server 不参与自动监控")
	}
	if paused {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 的监控采集已暂停")
	}
	if !server.SSHSessionID.Valid() || strings.TrimSpace(server.RemotePath) == "" {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 缺少有效 SSH Session 或远程路径")
	}
	if server.State == enums.LifecycleCreating || server.State == enums.LifecycleInstalling {
		return apperror.New(apperror.CodeValidationConflict, "Server 安装完成前不启用自动监控")
	}
	if server.State == enums.LifecycleFailed && latestInstallationState != nil && *latestInstallationState != enums.InstallationSucceeded {
		return apperror.New(apperror.CodeValidationConflict, "最近一次安装任务未成功，自动监控保持隔离")
	}
	return nil
}

func (c *MetricCollector) beginSessionCollection(sshSessionID model.ID, serverIDs []model.ID) (*sessionCollectionRun, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if current := c.inflight[sshSessionID]; current != nil {
		return current, false
	}
	run := &sessionCollectionRun{done: make(chan struct{}), serverIDs: append([]model.ID(nil), serverIDs...)}
	c.inflight[sshSessionID] = run
	for _, serverID := range serverIDs {
		c.collecting[serverID] = true
	}
	return run, true
}

func (c *MetricCollector) endSessionCollection(sshSessionID model.ID, run *sessionCollectionRun, results map[model.ID]error, sessionErr error) {
	c.mu.Lock()
	if c.inflight[sshSessionID] == run {
		delete(c.inflight, sshSessionID)
	}
	for _, serverID := range run.serverIDs {
		delete(c.collecting, serverID)
	}
	run.results = results
	run.sessionErr = sessionErr
	close(run.done)
	c.mu.Unlock()
}

func waitForSessionCollection(ctx context.Context, run *sessionCollectionRun) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-run.done:
		return nil
	}
}

func (c *MetricCollector) collectSession(ctx context.Context, sshSessionID model.ID, servers []model.MinecraftServer, targets []remoteMetricTarget, interval time.Duration, requestedServerID model.ID, requireSample bool) (map[model.ID]error, error) {
	results := make(map[model.ID]error, len(targets))
	targetByID := make(map[model.ID]remoteMetricTarget, len(targets))
	for _, target := range targets {
		targetByID[target.server.ID] = target
	}
	propagateFailure := func(failure error) (map[model.ID]error, error) {
		message := apperror.ToDTO(failure).Message
		for _, target := range targets {
			results[target.server.ID] = failure
			c.recordFailure(target.server.ID, message)
		}
		return results, failure
	}

	session, err := c.store.SSHSessions().Get(ctx, sshSessionID)
	if err != nil {
		return propagateFailure(err)
	}
	client, err := c.clients.Connect(ctx, session, c.settings.Snapshot().SSH)
	if err != nil {
		return propagateFailure(err)
	}
	defer func() { _ = client.Close() }()
	home, err := resolveRemoteCollectorHome(ctx, client)
	if err != nil {
		return propagateFailure(err)
	}
	timeout := interval
	if timeout < 10*time.Second {
		timeout = 10 * time.Second
	}
	if timeout > 45*time.Second {
		timeout = 45 * time.Second
	}

	probeResults := probeRemoteMetricTargets(ctx, client, servers, timeout)
	activeTargets := make([]remoteMetricTarget, 0, len(targets))
	legacyCollected := make(map[model.ID]bool)
	for _, server := range servers {
		_, active := targetByID[server.ID]
		deleted := server.DeletedAt != nil || server.State == enums.LifecycleDeleted
		cleanupOnly := deleted || c.IsPaused(server.ID)
		if !active && !cleanupOnly {
			continue
		}
		probeErr := probeResults[server.ID]
		if probeErr != nil {
			if active {
				results[server.ID] = probeErr
				c.recordFailure(server.ID, apperror.ToDTO(probeErr).Message)
			}
			continue
		}
		if c.isLegacyMigrated(server.ID) {
			continue
		}
		if deleted && c.isLegacyDeferred(server.ID) {
			continue
		}
		claim, migrationErr := collectLegacyRemoteMetrics(ctx, client, server, timeout)
		if migrationErr == nil && deleted {
			if strings.TrimSpace(claim.output) == "" {
				migrationErr = acknowledgeLegacyRemoteMetrics(ctx, client, server, "", timeout)
			} else {
				c.markLegacyDeferred(server.ID)
				continue
			}
		}
		if migrationErr == nil && !deleted {
			if strings.TrimSpace(claim.output) != "" {
				migrationErr = c.ingestMetricOutput(ctx, server, claim.output)
				if migrationErr == nil {
					legacyCollected[server.ID] = true
					migrationErr = acknowledgeLegacyRemoteMetrics(ctx, client, server, claim.path, timeout)
				}
			} else {
				migrationErr = acknowledgeLegacyRemoteMetrics(ctx, client, server, "", timeout)
			}
		}
		if migrationErr != nil {
			if _, active := targetByID[server.ID]; active {
				results[server.ID] = migrationErr
				c.recordFailure(server.ID, apperror.ToDTO(migrationErr).Message)
			}
			continue
		}
		c.markLegacyMigrated(server.ID)
	}
	for _, target := range targets {
		if results[target.server.ID] != nil || probeResults[target.server.ID] != nil {
			continue
		}
		activeTargets = append(activeTargets, target)
	}

	dataDirectory := remoteMetricCollectorDirectory(home, sshSessionID)
	if len(activeTargets) == 0 {
		if err := stopSharedRemoteCollector(ctx, client, dataDirectory, timeout); err != nil {
			return results, err
		}
		c.setSessionActive(sshSessionID, false)
		return results, nil
	}
	if err := ensureRemoteMetricCollectorDirectory(ctx, client, home, sshSessionID, timeout); err != nil {
		return propagateFailure(err)
	}
	manifest, err := buildRemoteCollectorManifest(activeTargets)
	if err != nil {
		return propagateFailure(err)
	}
	collectorPath := path.Join(dataDirectory, "collector.sh")
	daemonPath := path.Join(dataDirectory, "collector-daemon.sh")
	manifestPath := path.Join(dataDirectory, "targets.manifest")
	if err := uploadRemoteCollectorFile(ctx, client, dataDirectory, collectorPath, collectorScript, timeout); err != nil {
		return propagateFailure(err)
	}
	if err := uploadRemoteCollectorFile(ctx, client, dataDirectory, daemonPath, remoteCollectorDaemonScript, timeout); err != nil {
		return propagateFailure(err)
	}
	if err := uploadRemoteCollectorManifest(ctx, client, dataDirectory, manifestPath, manifest, timeout); err != nil {
		return propagateFailure(err)
	}
	if err := ensureSharedRemoteCollector(ctx, client, sshSessionID, dataDirectory, collectorPath, daemonPath, manifestPath, interval, timeout); err != nil {
		return propagateFailure(err)
	}
	c.setSessionActive(sshSessionID, true)

	for index, target := range activeTargets {
		waitAttempts := 0
		if requireSample && target.server.ID == requestedServerID || !requestedServerID.Valid() && index == 0 {
			waitAttempts = 3
		}
		claim, claimErr := collectSharedRemoteMetricClaim(ctx, client, dataDirectory, target.server.ID, waitAttempts, timeout)
		if claimErr != nil {
			results[target.server.ID] = claimErr
			c.recordFailure(target.server.ID, apperror.ToDTO(claimErr).Message)
			continue
		}
		if strings.TrimSpace(claim.output) == "" {
			if legacyCollected[target.server.ID] {
				results[target.server.ID] = nil
				continue
			}
			if !requireSample || target.server.ID != requestedServerID {
				results[target.server.ID] = nil
				continue
			}
			claimErr = apperror.New(apperror.CodeMetricCollectionFailed, "共享远端采集器尚未生成该 Server 的首个批次").WithRetryable(true)
			results[target.server.ID] = claimErr
			c.recordFailure(target.server.ID, apperror.ToDTO(claimErr).Message)
			continue
		}
		if claimErr = c.ingestMetricOutput(ctx, target.server, claim.output); claimErr == nil {
			claimErr = acknowledgeSharedRemoteMetricClaim(ctx, client, claim.path, timeout)
		}
		if claimErr != nil {
			c.recordFailure(target.server.ID, apperror.ToDTO(claimErr).Message)
		}
		results[target.server.ID] = claimErr
	}
	return results, nil
}

func resolveRemoteCollectorHome(ctx context.Context, client *SSHClient) (string, error) {
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "printenv", Arguments: []string{"HOME"}, Timeout: 5 * time.Second, MaximumOutput: 4096,
	})
	if err != nil || strings.TrimSpace(result.Stdout) == "" {
		return "", apperror.Wrap(apperror.CodeSFTPPathRejected, "读取远程 Home 目录失败", err)
	}
	home, err := model.NormalizeRemotePath(strings.TrimSpace(result.Stdout), "/", "/")
	if err != nil {
		return "", err
	}
	if home == "/" {
		return "", apperror.New(apperror.CodeSFTPPathRejected, "拒绝在远程根目录部署共享采集器")
	}
	return home, nil
}

func probeRemoteMetricTargets(ctx context.Context, client *SSHClient, servers []model.MinecraftServer, timeout time.Duration) map[model.ID]error {
	results := make(map[model.ID]error, len(servers))
	ordered := append([]model.MinecraftServer(nil), servers...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].ID.String() < ordered[right].ID.String() })
	const batchSize = 32
	probeScript := `set -eu
while [ "$#" -ge 2 ]; do
  server_id=$1
  directory=$2
  shift 2
  if [ -L "$directory" ]; then
    status=symlink
  elif [ ! -e "$directory" ]; then
    status=missing
  elif [ ! -d "$directory" ]; then
    status=not_directory
  else
    status=ok
  fi
  printf '%s\t%s\n' "$server_id" "$status"
done`
	for start := 0; start < len(ordered); start += batchSize {
		end := min(start+batchSize, len(ordered))
		arguments := []string{"-c", probeScript, "mineops-metric-probe"}
		for _, server := range ordered[start:end] {
			arguments = append(arguments, server.ID.String(), server.RemotePath)
		}
		result, err := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh", Arguments: arguments, Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
		})
		if err != nil {
			for _, server := range ordered[start:end] {
				results[server.ID] = apperror.Wrap(apperror.CodeSFTPPathRejected, "探测远程 Server 目录失败", err)
			}
			continue
		}
		seen := make(map[model.ID]bool, end-start)
		for _, line := range strings.Split(result.Stdout, "\n") {
			fields := strings.Split(strings.TrimSpace(line), "\t")
			if len(fields) != 2 {
				continue
			}
			serverID := model.ID(fields[0])
			seen[serverID] = true
			switch fields[1] {
			case "ok":
				results[serverID] = nil
			case "missing":
				results[serverID] = apperror.New(apperror.CodeSFTPPathRejected, "远程 Server 目录不存在，监控不会自动创建该目录")
			case "symlink":
				results[serverID] = apperror.New(apperror.CodeSFTPPathRejected, "远程 Server 路径是符号链接，拒绝部署监控")
			default:
				results[serverID] = apperror.New(apperror.CodeSFTPPathRejected, "远程 Server 路径不是普通目录")
			}
		}
		for _, server := range ordered[start:end] {
			if !seen[server.ID] {
				results[server.ID] = apperror.New(apperror.CodeIOReadFailed, "远程 Server 目录探测响应缺失")
			}
		}
	}
	return results
}

func collectLegacyRemoteMetrics(ctx context.Context, client *SSHClient, server model.MinecraftServer, timeout time.Duration) (remoteMetricClaim, error) {
	dataDirectory := path.Join(server.RemotePath, ".mineops-monitoring")
	pidPath := path.Join(dataDirectory, "collector.pid")
	daemonPath := path.Join(dataDirectory, "collector-daemon.sh")
	claimPath := path.Join(dataDirectory, "metrics.claim")
	spoolPath := path.Join(dataDirectory, "metrics.spool")
	migrationScript := `set -eu
server_dir=$1
data_dir=$2
pid_path=$3
daemon_path=$4
claim_path=$5
spool_path=$6
[ ! -L "$server_dir" ] && [ -d "$server_dir" ] || exit 12
[ ! -L "$data_dir" ] || exit 13
[ -d "$data_dir" ] || exit 0
pid=""
if [ -f "$pid_path" ]; then pid=$(cat "$pid_path" 2>/dev/null || true); fi
case "$pid" in
  ''|*[!0-9]*) pid="" ;;
  *) [ "$pid" -gt 1 ] 2>/dev/null || pid="" ;;
esac
if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)
  case "$command_line" in
    *"$daemon_path"*)
      kill "$pid" 2>/dev/null || true
      attempt=0
      while kill -0 "$pid" 2>/dev/null && [ "$attempt" -lt 5 ]; do sleep 1; attempt=$((attempt + 1)); done
      kill -0 "$pid" 2>/dev/null && exit 14
      ;;
  esac
fi
rm -f "$pid_path"
if [ ! -s "$claim_path" ] && [ -s "$spool_path" ]; then mv "$spool_path" "$claim_path"; fi
if [ -s "$claim_path" ]; then cat "$claim_path"; fi`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", migrationScript, "mineops-legacy-metric", server.RemotePath, dataDirectory, pidPath, daemonPath, claimPath, spoolPath},
		Timeout: timeout, MaximumOutput: remoteCollectorMaximumOutput,
	})
	if err != nil {
		return remoteMetricClaim{}, apperror.Wrap(apperror.CodeMetricCollectionFailed, "迁移旧版 Server 级监控采集器失败", err)
	}
	if result.Truncated {
		return remoteMetricClaim{}, apperror.New(apperror.CodeMetricCollectionFailed, "旧版远端监控缓冲超过安全读取上限")
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return remoteMetricClaim{}, nil
	}
	return remoteMetricClaim{path: claimPath, output: result.Stdout}, nil
}

func acknowledgeLegacyRemoteMetrics(ctx context.Context, client *SSHClient, server model.MinecraftServer, claimPath string, timeout time.Duration) error {
	dataDirectory := path.Join(server.RemotePath, ".mineops-monitoring")
	files := []string{
		path.Join(dataDirectory, "collector.pid"), path.Join(dataDirectory, "collector.config"),
		path.Join(dataDirectory, "collector.sh"), path.Join(dataDirectory, "collector-daemon.sh"),
		path.Join(dataDirectory, "metrics.spool"),
	}
	if claimPath != "" {
		files = append(files, claimPath)
	}
	arguments := append([]string{"-f", "--"}, files...)
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "rm", Arguments: arguments, Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "确认并清理旧版基础指标缓冲失败", err)
	}
	return nil
}

func remoteMetricCollectorDirectory(home string, sshSessionID model.ID) string {
	return path.Join(home, "MineOps", "Agents", "metric-collector", sshSessionID.String())
}

func ensureRemoteMetricCollectorDirectory(ctx context.Context, client *SSHClient, home string, sshSessionID model.ID, timeout time.Duration) error {
	mineOpsRoot := path.Join(home, "MineOps")
	agentsRoot := path.Join(mineOpsRoot, "Agents")
	collectorRoot := path.Join(agentsRoot, "metric-collector")
	dataDirectory := path.Join(collectorRoot, sshSessionID.String())
	spoolDirectory := path.Join(dataDirectory, "spool")
	claimDirectory := path.Join(dataDirectory, "claim")
	lockDirectory := path.Join(dataDirectory, "locks")
	ensureScript := `set -eu
for directory in "$@"; do
  if [ -L "$directory" ]; then exit 20; fi
  if [ -e "$directory" ] && [ ! -d "$directory" ]; then exit 21; fi
  if [ ! -e "$directory" ]; then
    if ! mkdir "$directory" 2>/dev/null; then
      [ -d "$directory" ] && [ ! -L "$directory" ] || exit 22
    fi
  fi
done
chmod 700 "$3" "$4" "$5" "$6" "$7"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", ensureScript, "mineops-metric-directory", mineOpsRoot, agentsRoot, collectorRoot, dataDirectory, spoolDirectory, claimDirectory, lockDirectory},
		Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "共享监控目录不存在、不是普通目录或包含符号链接", err)
	}
	return nil
}

func buildRemoteCollectorManifest(targets []remoteMetricTarget) ([]byte, error) {
	ordered := append([]remoteMetricTarget(nil), targets...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].server.ID.String() < ordered[right].server.ID.String()
	})
	var manifest bytes.Buffer
	manifest.WriteString("manifest_version=1\n")
	separator := string([]byte{remoteCollectorManifestSeparator})
	for _, target := range ordered {
		fields := []string{
			target.server.ID.String(), target.server.RemotePath, target.jarName,
			target.managedPID, target.managedProcessGroupID,
		}
		if !target.server.ID.Valid() || !path.IsAbs(target.server.RemotePath) {
			return nil, apperror.New(apperror.CodeValidationInvalidArgument, "远端监控目标 ID 或路径无效")
		}
		for _, field := range fields {
			if strings.ContainsAny(field, "\x00\r\n"+separator) {
				return nil, apperror.New(apperror.CodeValidationInvalidArgument, "远端监控目标包含清单控制字符")
			}
		}
		for _, numeric := range []string{target.managedPID, target.managedProcessGroupID} {
			if numeric != "" {
				parsed, err := strconv.Atoi(numeric)
				if err != nil || parsed < 1 {
					return nil, apperror.New(apperror.CodeValidationInvalidArgument, "远端监控目标 PID 或进程组无效")
				}
			}
		}
		manifest.WriteString(strings.Join(fields, separator))
		manifest.WriteByte('\n')
	}
	return manifest.Bytes(), nil
}

func uploadRemoteCollectorFile(ctx context.Context, client *SSHClient, directory, target string, content []byte, timeout time.Duration) error {
	uploadScript := `set -eu
target=$1
directory=$2
umask 077
[ -d "$directory" ] && [ ! -L "$directory" ]
chmod 700 "$directory"
temporary="$target.tmp.$$"
trap 'rm -f "$temporary"' 0 1 2 15
cat > "$temporary"
chmod 700 "$temporary"
mv -f "$temporary" "$target"
trap - 0 1 2 15`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", uploadScript, "mineops-metric-upload", target, directory}, Input: content,
		Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "部署远端共享监控脚本失败", err)
	}
	return nil
}

func uploadRemoteCollectorManifest(ctx context.Context, client *SSHClient, directory, target string, content []byte, timeout time.Duration) error {
	uploadScript := `set -eu
target=$1
directory=$2
umask 077
[ -d "$directory" ] && [ ! -L "$directory" ]
chmod 700 "$directory"
temporary="$target.tmp.$$"
trap 'rm -f "$temporary"' 0 1 2 15
cat > "$temporary"
chmod 600 "$temporary"
mv -f "$temporary" "$target"
trap - 0 1 2 15`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", uploadScript, "mineops-metric-manifest", target, directory}, Input: content,
		Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "写入远端共享监控目标清单失败", err)
	}
	return nil
}

func ensureSharedRemoteCollector(ctx context.Context, client *SSHClient, sshSessionID model.ID, dataDirectory, collectorPath, daemonPath, manifestPath string, interval, timeout time.Duration) error {
	intervalSeconds := int(interval / time.Second)
	if intervalSeconds < 5 {
		intervalSeconds = 5
	}
	configDigest := remoteCollectorConfigDigest(sshSessionID, intervalSeconds)
	startScript := `set -eu
data_dir=$1
collector_path=$2
daemon_path=$3
manifest_path=$4
interval=$5
maximum_spool_bytes=$6
config_digest=$7
pid_path="$data_dir/collector.pid"
config_path="$data_dir/collector.config"
umask 077
[ -d "$data_dir" ] && [ ! -L "$data_dir" ]
chmod 700 "$data_dir"
running=0
old_pid=""
if [ -f "$pid_path" ]; then old_pid=$(cat "$pid_path" 2>/dev/null || true); fi
case "$old_pid" in
  ''|*[!0-9]*) old_pid="" ;;
  *) [ "$old_pid" -gt 1 ] 2>/dev/null || old_pid="" ;;
esac
if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$old_pid/cmdline" 2>/dev/null || true)
  current_config=$(cat "$config_path" 2>/dev/null || true)
  case "$command_line" in *"$daemon_path"*) [ "$current_config" = "$config_digest" ] && running=1 ;; esac
fi
if [ "$running" -eq 1 ]; then exit 0; fi
if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$old_pid/cmdline" 2>/dev/null || true)
  case "$command_line" in
    *"$daemon_path"*)
      kill "$old_pid" 2>/dev/null || true
      attempt=0
      while kill -0 "$old_pid" 2>/dev/null && [ "$attempt" -lt 5 ]; do sleep 1; attempt=$((attempt + 1)); done
      kill -0 "$old_pid" 2>/dev/null && exit 15
      ;;
  esac
fi
config_tmp="$config_path.tmp.$$"
printf '%s\n' "$config_digest" > "$config_tmp"
chmod 600 "$config_tmp"
mv -f "$config_tmp" "$config_path"
if command -v nohup >/dev/null 2>&1; then
  nohup sh "$daemon_path" "$data_dir" "$collector_path" "$manifest_path" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
else
  sh "$daemon_path" "$data_dir" "$collector_path" "$manifest_path" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
fi
printf '%s\n' "$!" > "$pid_path"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", startScript, "mineops-metric-start", dataDirectory, collectorPath, daemonPath, manifestPath, strconv.Itoa(intervalSeconds), strconv.Itoa(remoteCollectorSpoolBytes), configDigest},
		Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "启动 SSH Session 共享监控守护进程失败", err)
	}
	return nil
}

func collectSharedRemoteMetricClaim(ctx context.Context, client *SSHClient, dataDirectory string, serverID model.ID, waitAttempts int, timeout time.Duration) (remoteMetricClaim, error) {
	if waitAttempts < 0 {
		waitAttempts = 0
	}
	if waitAttempts > 10 {
		waitAttempts = 10
	}
	claimPath := path.Join(dataDirectory, "claim", serverID.String()+".claim")
	claimScript := `set -eu
data_dir=$1
server_id=$2
maximum_wait_attempts=$3
spool_dir="$data_dir/spool"
claim_dir="$data_dir/claim"
lock_root="$data_dir/locks"
spool_path="$spool_dir/$server_id.spool"
claim_path="$claim_dir/$server_id.claim"
lock_path="$lock_root/$server_id.lock"
umask 077
[ -d "$data_dir" ] && [ ! -L "$data_dir" ]
[ -d "$claim_dir" ] && [ ! -L "$claim_dir" ]
[ -d "$lock_root" ] && [ ! -L "$lock_root" ]
attempt=0
while [ ! -s "$claim_path" ] && [ ! -s "$spool_path" ] && [ "$attempt" -lt "$maximum_wait_attempts" ]; do
  sleep 1
  attempt=$((attempt + 1))
done
attempt=0
while ! mkdir "$lock_path" 2>/dev/null; do
  owner_pid=$(cat "$lock_path/owner" 2>/dev/null || true)
  case "$owner_pid" in
    ''|*[!0-9]*) ;;
    *)
      if ! kill -0 "$owner_pid" 2>/dev/null; then
        rm -f "$lock_path/owner"
        rmdir "$lock_path" 2>/dev/null || true
        continue
      fi
      ;;
  esac
  attempt=$((attempt + 1))
  [ "$attempt" -lt 30 ] || exit 16
  sleep 1
done
printf '%s\n' "$$" > "$lock_path/owner"
trap 'rm -f "$lock_path/owner"; rmdir "$lock_path" 2>/dev/null || true' 0 1 2 15
if [ ! -s "$claim_path" ] && [ -s "$spool_path" ]; then mv "$spool_path" "$claim_path"; fi
rm -f "$lock_path/owner"
rmdir "$lock_path" 2>/dev/null || true
trap - 0 1 2 15
if [ -s "$claim_path" ]; then cat "$claim_path"; fi`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", claimScript, "mineops-metric-claim", dataDirectory, serverID.String(), strconv.Itoa(waitAttempts)},
		Timeout: timeout, MaximumOutput: remoteCollectorMaximumOutput,
	})
	if err != nil {
		return remoteMetricClaim{}, apperror.Wrap(apperror.CodeMetricCollectionFailed, "领取远端共享监控缓冲失败", err)
	}
	if result.Truncated {
		return remoteMetricClaim{}, apperror.New(apperror.CodeMetricCollectionFailed, "远端共享监控缓冲超过安全读取上限")
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return remoteMetricClaim{}, nil
	}
	return remoteMetricClaim{path: claimPath, output: result.Stdout}, nil
}

func acknowledgeSharedRemoteMetricClaim(ctx context.Context, client *SSHClient, claimPath string, timeout time.Duration) error {
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "rm", Arguments: []string{"-f", "--", claimPath}, Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "确认远端共享监控缓冲失败", err)
	}
	return nil
}

func stopSharedRemoteCollector(ctx context.Context, client *SSHClient, dataDirectory string, timeout time.Duration) error {
	pidPath := path.Join(dataDirectory, "collector.pid")
	daemonPath := path.Join(dataDirectory, "collector-daemon.sh")
	configPath := path.Join(dataDirectory, "collector.config")
	stopScript := `set -eu
data_dir=$1
pid_path=$2
daemon_path=$3
config_path=$4
[ -d "$data_dir" ] && [ ! -L "$data_dir" ] || exit 0
pid=""
if [ -f "$pid_path" ]; then pid=$(cat "$pid_path" 2>/dev/null || true); fi
case "$pid" in
  ''|*[!0-9]*) pid="" ;;
  *) [ "$pid" -gt 1 ] 2>/dev/null || pid="" ;;
esac
if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)
  case "$command_line" in
    *"$daemon_path"*)
      kill "$pid" 2>/dev/null || true
      attempt=0
      while kill -0 "$pid" 2>/dev/null && [ "$attempt" -lt 5 ]; do sleep 1; attempt=$((attempt + 1)); done
      kill -0 "$pid" 2>/dev/null && exit 15
      ;;
  esac
fi
rm -f "$pid_path" "$config_path"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", stopScript, "mineops-metric-stop", dataDirectory, pidPath, daemonPath, configPath},
		Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "停止 SSH Session 共享监控守护进程失败", err)
	}
	return nil
}

func remoteCollectorConfigDigest(sshSessionID model.ID, intervalSeconds int) string {
	hash := sha256.New()
	for _, value := range [][]byte{
		collectorScript, remoteCollectorDaemonScript, []byte(sshSessionID.String()),
		[]byte(strconv.Itoa(intervalSeconds)), []byte(strconv.Itoa(remoteCollectorSpoolBytes)),
	} {
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (c *MetricCollector) ingestMetricOutput(ctx context.Context, server model.MinecraftServer, output string) error {
	now := c.clock.Now().UTC()
	samples, parseErrors := c.parseSamples(server, now, output)
	if len(samples) == 0 {
		message := "采集脚本没有返回任何指标"
		if len(parseErrors) > 0 {
			message = strings.Join(parseErrors, "; ")
		}
		return apperror.New(apperror.CodeMetricCollectionFailed, message).WithDetails(map[string]any{"errors": parseErrors})
	}
	for start := 0; start < len(samples); start += maximumMetricIngestBatch {
		end := min(start+maximumMetricIngestBatch, len(samples))
		if _, err := c.metrics.Ingest(ctx, samples[start:end]); err != nil {
			return err
		}
	}
	collectedAt := samples[0].Timestamp
	for _, sample := range samples[1:] {
		if sample.Timestamp.After(collectedAt) {
			collectedAt = sample.Timestamp
		}
	}
	c.recordSuccess(server.ID, collectedAt, parseErrors)
	return nil
}

// parseSamples 把脚本的 key=value 输出转换为已注册单位的 MetricSample 列表与错误摘要。
func (c *MetricCollector) parseSamples(server model.MinecraftServer, now time.Time, output string) ([]model.MetricSample, []string) {
	pathTags := map[string]string{"path": server.RemotePath}
	interfaceTags := map[string]string{"interface": "all"}
	metricByKey := map[string]struct {
		metric enums.MetricType
		tags   map[string]string
	}{
		"cpu_percent":         {metric: enums.MetricHostCPU},
		"mem_percent":         {metric: enums.MetricHostMemory},
		"mem_used":            {metric: enums.MetricHostMemoryUsed},
		"mem_total":           {metric: enums.MetricHostMemoryTotal},
		"swap_used":           {metric: enums.MetricHostSwapUsed},
		"load1":               {metric: enums.MetricHostLoad1},
		"disk_used":           {metric: enums.MetricHostDiskUsed, tags: pathTags},
		"disk_total":          {metric: enums.MetricHostDiskTotal, tags: pathTags},
		"disk_read_bps":       {metric: enums.MetricHostDiskRead, tags: pathTags},
		"disk_write_bps":      {metric: enums.MetricHostDiskWrite, tags: pathTags},
		"net_rx_bps":          {metric: enums.MetricHostNetworkReceive, tags: interfaceTags},
		"net_tx_bps":          {metric: enums.MetricHostNetworkTransmit, tags: interfaceTags},
		"proc_running":        {metric: enums.MetricProcessRunning},
		"proc_cpu_percent":    {metric: enums.MetricProcessCPU},
		"proc_rss_bytes":      {metric: enums.MetricProcessRSS},
		"proc_threads":        {metric: enums.MetricProcessThreads},
		"proc_fds":            {metric: enums.MetricProcessFDs},
		"proc_uptime_seconds": {metric: enums.MetricProcessUptime},
	}
	samples := make([]model.MetricSample, 0, len(metricByKey))
	errorsFound := make([]string, 0, 4)
	sampleTime := now.UTC()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		key, value, found := strings.Cut(line, "=")
		if !found || key == "" {
			continue
		}
		if strings.HasPrefix(key, "err_") {
			errorsFound = append(errorsFound, strings.TrimPrefix(key, "err_")+": "+value)
			continue
		}
		if key == "ts" {
			seconds, timestampErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			candidate := time.Unix(seconds, 0).UTC()
			if timestampErr != nil || seconds <= 0 || candidate.After(now.UTC().Add(5*time.Minute)) {
				errorsFound = append(errorsFound, "ts: invalid remote timestamp")
				sampleTime = now.UTC()
			} else {
				sampleTime = candidate
			}
			continue
		}
		mapping, known := metricByKey[key]
		if !known {
			continue
		}
		parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if parseErr != nil {
			errorsFound = append(errorsFound, key+": invalid value")
			continue
		}
		definition, definitionErr := model.DefinitionForMetric(mapping.metric)
		if definitionErr != nil {
			errorsFound = append(errorsFound, key+": "+apperror.ToDTO(definitionErr).Message)
			continue
		}
		samples = append(samples, model.MetricSample{
			ServerID: server.ID, SourceID: server.ID, Metric: mapping.metric, Unit: definition.Unit,
			Timestamp: sampleTime, Value: parsed, Tags: mapping.tags, SchemaVersion: model.MetricSchemaVersion,
		})
	}
	if len(errorsFound) > 16 {
		errorsFound = errorsFound[:16]
	}
	return samples, errorsFound
}

func (c *MetricCollector) recordSuccess(serverID model.ID, collectedAt time.Time, collectionErrors []string) {
	message := strings.Join(collectionErrors, "; ")
	if len(message) > metricCollectorErrorLimit {
		message = message[:metricCollectorErrorLimit]
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	status := CollectorStatus{ServerID: serverID, CollectedAt: &collectedAt}
	if message != "" {
		status.LastError = message
		status.LastErrorAt = &collectedAt
	}
	c.statuses[serverID] = status
}

func (c *MetricCollector) recordFailure(serverID model.ID, message string) {
	if len(message) > metricCollectorErrorLimit {
		message = message[:metricCollectorErrorLimit]
	}
	now := c.clock.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	status := c.statuses[serverID]
	status.ServerID = serverID
	status.LastError = message
	status.LastErrorAt = &now
	c.statuses[serverID] = status
}

func (c *MetricCollector) startupSweepRequired() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.startupSweepDone
}

func (c *MetricCollector) markStartupSweepDone() {
	c.mu.Lock()
	c.startupSweepDone = true
	c.mu.Unlock()
}

func (c *MetricCollector) activeSessionIDs() []model.ID {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]model.ID, 0, len(c.sessionActive))
	for sessionID, active := range c.sessionActive {
		if active {
			result = append(result, sessionID)
		}
	}
	return result
}

func (c *MetricCollector) isSessionActive(sshSessionID model.ID) bool {
	c.mu.Lock()
	active := c.sessionActive[sshSessionID]
	c.mu.Unlock()
	return active
}

func (c *MetricCollector) setSessionActive(sshSessionID model.ID, active bool) {
	c.mu.Lock()
	if active {
		c.sessionActive[sshSessionID] = true
	} else {
		delete(c.sessionActive, sshSessionID)
	}
	c.mu.Unlock()
}

func (c *MetricCollector) isLegacyMigrated(serverID model.ID) bool {
	c.mu.Lock()
	migrated := c.legacyMigrated[serverID]
	c.mu.Unlock()
	return migrated
}

func (c *MetricCollector) markLegacyMigrated(serverID model.ID) {
	c.mu.Lock()
	c.legacyMigrated[serverID] = true
	delete(c.legacyDeferred, serverID)
	c.mu.Unlock()
}

func (c *MetricCollector) isLegacyDeferred(serverID model.ID) bool {
	c.mu.Lock()
	deferred := c.legacyDeferred[serverID]
	c.mu.Unlock()
	return deferred
}

func (c *MetricCollector) markLegacyDeferred(serverID model.ID) {
	c.mu.Lock()
	c.legacyDeferred[serverID] = true
	c.mu.Unlock()
}

// stopRemoteCollector 保留内部兼容入口；共享化后实际执行整个 SSH Session 对账。
func (c *MetricCollector) stopRemoteCollector(ctx context.Context, server model.MinecraftServer) error {
	return c.ReconcileSession(ctx, server.SSHSessionID)
}
