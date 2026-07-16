package service

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"path"
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
	// metricCollectorConcurrency 是同时采集的远程主机上限。
	metricCollectorConcurrency = 8
	// metricCollectorMaximumOutput 是单次采集允许的最大脚本输出。
	metricCollectorMaximumOutput = 64 * 1024
	// metricCollectorErrorLimit 是采集错误摘要的最大长度。
	metricCollectorErrorLimit = 1024
	// remoteCollectorSpoolBytes 是每台远端 Server 保留的离线采集上限。
	remoteCollectorSpoolBytes = 8 * 1024 * 1024
	// remoteCollectorMaximumOutput 是单次领取远端离线批次允许的最大输出。
	remoteCollectorMaximumOutput = remoteCollectorSpoolBytes + 64*1024
)

type remoteMetricClaim struct {
	path   string
	output string
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

// MetricCollector 通过已绑定的 SSH Session 周期执行内嵌采集脚本并把指标写入 Metric 存储。
type MetricCollector struct {
	clock    model.Clock
	store    repository.Store
	settings *appsettings.Manager
	clients  *SSHClientFactory
	metrics  *MetricManager
	logger   *applog.Logger

	mu       sync.Mutex
	statuses map[model.ID]CollectorStatus
	inflight map[model.ID]bool
	paused   map[model.ID]bool
	slots    chan struct{}
}

// NewMetricCollector 创建 SSH 拉取采集服务。
func NewMetricCollector(clock model.Clock, store repository.Store, settings *appsettings.Manager, clients *SSHClientFactory, metrics *MetricManager, logger *applog.Logger) (*MetricCollector, error) {
	if clock == nil || store == nil || settings == nil || clients == nil || metrics == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Metric Collector 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &MetricCollector{
		clock: clock, store: store, settings: settings, clients: clients, metrics: metrics, logger: logger,
		statuses: make(map[model.ID]CollectorStatus), inflight: make(map[model.ID]bool), paused: make(map[model.ID]bool),
		slots: make(chan struct{}, metricCollectorConcurrency),
	}, nil
}

// Run 按 Monitoring.IntervalSeconds 周期采集全部绑定 SSH 的服务器,直到 ctx 取消。
func (c *MetricCollector) Run(ctx context.Context) error {
	for {
		interval := time.Duration(c.settings.Snapshot().Monitoring.IntervalSeconds) * time.Second
		if interval < time.Second {
			interval = 5 * time.Second
		}
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

// CollectNow 立即对一台服务器执行一次采集并同步返回结果。
func (c *MetricCollector) CollectNow(ctx context.Context, serverID model.ID) error {
	if c.IsPaused(serverID) {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 的监控采集已暂停")
	}
	server, err := c.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	if !server.SSHSessionID.Valid() {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 未绑定 SSH Session,无法采集")
	}
	if !c.beginCollect(server.ID) {
		return apperror.New(apperror.CodeValidationConflict, "该 Server 正在采集中").WithRetryable(true)
	}
	defer c.endCollect(server.ID)
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.slots }()
	interval := time.Duration(c.settings.Snapshot().Monitoring.IntervalSeconds) * time.Second
	return c.collectServer(ctx, *server, interval)
}

// Status 返回一台服务器最近一次采集状态的副本。
func (c *MetricCollector) Status(serverID model.ID) CollectorStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	status, found := c.statuses[serverID]
	if !found {
		status = CollectorStatus{ServerID: serverID}
	}
	status.Collecting = c.inflight[serverID]
	status.Paused = c.paused[serverID]
	return status
}

// Pause stops future automatic collection passes for one Server.
func (c *MetricCollector) Pause(serverID model.ID) error {
	if !serverID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	c.mu.Lock()
	if c.paused == nil {
		c.paused = make(map[model.ID]bool)
	}
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

func (c *MetricCollector) collectAll(ctx context.Context, interval time.Duration) {
	servers, err := c.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{Limit: 500})
	if err != nil {
		c.logger.Error(ctx, "列举 Server 以采集 Metric 失败", err, applog.Fields{"component": "metric-collector"})
		return
	}
	var wait sync.WaitGroup
	for _, server := range servers {
		if !server.SSHSessionID.Valid() || strings.TrimSpace(server.RemotePath) == "" {
			continue
		}
		if c.IsPaused(server.ID) {
			continue
		}
		if !c.beginCollect(server.ID) {
			continue
		}
		select {
		case c.slots <- struct{}{}:
		case <-ctx.Done():
			c.endCollect(server.ID)
			return
		}
		wait.Add(1)
		go func(server model.MinecraftServer) {
			defer wait.Done()
			defer func() { <-c.slots }()
			defer c.endCollect(server.ID)
			if err := c.collectServer(ctx, server, interval); err != nil && ctx.Err() == nil {
				c.logger.Warn(ctx, "SSH 拉取采集失败", applog.Fields{
					"component": "metric-collector", "server_id": server.ID.String(), "error": err.Error(),
				})
			}
		}(server)
	}
	wait.Wait()
}

func (c *MetricCollector) beginCollect(serverID model.ID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[serverID] {
		return false
	}
	c.inflight[serverID] = true
	return true
}

func (c *MetricCollector) endCollect(serverID model.ID) {
	c.mu.Lock()
	delete(c.inflight, serverID)
	c.mu.Unlock()
}

// collectServer 同步远端守护进程缓冲；不可部署时退回当前 SSH 会话即时采集。
func (c *MetricCollector) collectServer(ctx context.Context, server model.MinecraftServer, interval time.Duration) error {
	session, err := c.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		c.recordFailure(server.ID, err.Error())
		return err
	}
	timeout := interval
	if timeout < 10*time.Second {
		timeout = 10 * time.Second
	}
	if timeout > 45*time.Second {
		timeout = 45 * time.Second
	}
	sshSettings := c.settings.Snapshot().SSH
	client, err := c.clients.Connect(ctx, session, sshSettings)
	if err != nil {
		c.recordFailure(server.ID, err.Error())
		return err
	}
	defer func() { _ = client.Close() }()
	// tmux pane PID 通常是包装 Shell；同时传递进程组,由脚本通过全局可读的 /proc stat/comm 定位实际 Java 子进程。
	managedPID := ""
	managedProcessGroupID := ""
	if identity, identityErr := c.store.ProcessIdentities().GetByServer(ctx, server.ID); identityErr == nil && identity.PID > 0 && identity.State == enums.RemoteProcessRunning {
		managedPID = strconv.Itoa(identity.PID)
		if identity.ProcessGroupID > 0 {
			managedProcessGroupID = strconv.Itoa(identity.ProcessGroupID)
		}
	}
	jarName := path.Base(server.LaunchProfile.JarPath)
	claim, bufferedErr := c.collectRemoteBufferedMetrics(ctx, client, server, jarName, managedPID, managedProcessGroupID, interval, timeout)
	collectionErrors := make([]string, 0, 4)
	if bufferedErr != nil {
		collectionErrors = append(collectionErrors, "offline_buffer: "+apperror.ToDTO(bufferedErr).Message)
	}
	output := claim.output
	if strings.TrimSpace(output) == "" {
		result, immediateErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh",
			Arguments:  []string{"-s", "--", server.RemotePath, jarName, managedPID, managedProcessGroupID},
			Input:      collectorScript,
			Timeout:    timeout, MaximumOutput: metricCollectorMaximumOutput,
		})
		if immediateErr != nil {
			failure := apperror.Wrap(apperror.CodeMetricCollectionFailed, "执行远程采集脚本失败", errors.Join(bufferedErr, immediateErr))
			c.recordFailure(server.ID, apperror.ToDTO(failure).Message+": "+immediateErr.Error())
			return failure
		}
		output = result.Stdout
	}
	now := c.clock.Now().UTC()
	samples, parseErrors := c.parseSamples(server, now, output)
	collectionErrors = append(collectionErrors, parseErrors...)
	if len(samples) == 0 {
		message := "采集脚本没有返回任何指标"
		if len(collectionErrors) > 0 {
			message = strings.Join(collectionErrors, "; ")
		}
		c.recordFailure(server.ID, message)
		return apperror.New(apperror.CodeMetricCollectionFailed, "采集脚本没有返回任何指标").WithDetails(map[string]any{"errors": collectionErrors})
	}
	for start := 0; start < len(samples); start += maximumMetricIngestBatch {
		end := min(start+maximumMetricIngestBatch, len(samples))
		if _, err := c.metrics.Ingest(ctx, samples[start:end]); err != nil {
			c.recordFailure(server.ID, err.Error())
			return err
		}
	}
	if claim.path != "" {
		if err := c.acknowledgeRemoteMetricClaim(ctx, client, claim.path, timeout); err != nil {
			collectionErrors = append(collectionErrors, "offline_ack: "+apperror.ToDTO(err).Message)
		}
	}
	collectedAt := now
	for _, sample := range samples {
		if sample.Timestamp.After(collectedAt) {
			collectedAt = sample.Timestamp
		}
	}
	c.recordSuccess(server.ID, collectedAt, collectionErrors)
	return nil
}

func (c *MetricCollector) collectRemoteBufferedMetrics(ctx context.Context, client *SSHClient, server model.MinecraftServer, jarName, managedPID, managedProcessGroupID string, interval, timeout time.Duration) (remoteMetricClaim, error) {
	dataDirectory := path.Join(server.RemotePath, ".mineops-monitoring")
	collectorPath := path.Join(dataDirectory, "collector.sh")
	daemonPath := path.Join(dataDirectory, "collector-daemon.sh")
	if err := uploadRemoteCollectorFile(ctx, client, dataDirectory, collectorPath, collectorScript, timeout); err != nil {
		return remoteMetricClaim{}, err
	}
	if err := uploadRemoteCollectorFile(ctx, client, dataDirectory, daemonPath, remoteCollectorDaemonScript, timeout); err != nil {
		return remoteMetricClaim{}, err
	}
	intervalSeconds := int(interval / time.Second)
	if intervalSeconds < 5 {
		intervalSeconds = 5
	}
	configDigest := remoteCollectorConfigDigest(server, jarName, managedPID, managedProcessGroupID, intervalSeconds)
	startScript := `set -eu
data_dir=$1
collector_path=$2
daemon_path=$3
server_dir=$4
jar_name=$5
managed_pid=$6
managed_pgid=$7
interval=$8
maximum_spool_bytes=$9
config_digest=${10}
pid_path="$data_dir/collector.pid"
config_path="$data_dir/collector.config"
umask 077
mkdir -p "$data_dir"
running=0
old_pid=""
if [ -f "$pid_path" ]; then old_pid=$(cat "$pid_path" 2>/dev/null || true); fi
if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$old_pid/cmdline" 2>/dev/null || true)
  current_config=$(cat "$config_path" 2>/dev/null || true)
  case "$command_line" in *"$daemon_path"*) [ "$current_config" = "$config_digest" ] && running=1 ;; esac
fi
if [ "$running" -eq 1 ]; then exit 0; fi
if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$old_pid/cmdline" 2>/dev/null || true)
  case "$command_line" in *"$daemon_path"*) kill "$old_pid" 2>/dev/null || true ;; esac
fi
config_tmp="$config_path.tmp.$$"
printf '%s\n' "$config_digest" > "$config_tmp"
mv -f "$config_tmp" "$config_path"
if command -v nohup >/dev/null 2>&1; then
  nohup sh "$daemon_path" "$data_dir" "$collector_path" "$server_dir" "$jar_name" "$managed_pid" "$managed_pgid" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
else
  sh "$daemon_path" "$data_dir" "$collector_path" "$server_dir" "$jar_name" "$managed_pid" "$managed_pgid" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
fi
printf '%s\n' "$!" > "$pid_path"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", startScript, "mineops", dataDirectory, collectorPath, daemonPath, server.RemotePath, jarName, managedPID, managedProcessGroupID, strconv.Itoa(intervalSeconds), strconv.Itoa(remoteCollectorSpoolBytes), configDigest},
		Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return remoteMetricClaim{}, apperror.Wrap(apperror.CodeMetricCollectionFailed, "启动远端监控守护进程失败", err)
	}
	claimPath := path.Join(dataDirectory, "metrics.claim")
	spoolPath := path.Join(dataDirectory, "metrics.spool")
	claimScript := `set -eu
claim_path=$1
spool_path=$2
attempt=0
while [ ! -s "$claim_path" ] && [ ! -s "$spool_path" ] && [ "$attempt" -lt 3 ]; do
  sleep 1
  attempt=$((attempt + 1))
done
if [ ! -s "$claim_path" ] && [ -s "$spool_path" ]; then mv "$spool_path" "$claim_path"; fi
if [ -s "$claim_path" ]; then cat "$claim_path"; fi`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", claimScript, "mineops", claimPath, spoolPath},
		Timeout: timeout, MaximumOutput: remoteCollectorMaximumOutput,
	})
	if err != nil {
		return remoteMetricClaim{}, apperror.Wrap(apperror.CodeMetricCollectionFailed, "领取远端监控缓冲失败", err)
	}
	if result.Truncated {
		return remoteMetricClaim{}, apperror.New(apperror.CodeMetricCollectionFailed, "远端监控缓冲超过安全读取上限")
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return remoteMetricClaim{}, nil
	}
	return remoteMetricClaim{path: claimPath, output: result.Stdout}, nil
}

func uploadRemoteCollectorFile(ctx context.Context, client *SSHClient, directory, target string, content []byte, timeout time.Duration) error {
	uploadScript := `set -eu
target=$1
directory=$2
umask 077
mkdir -p "$directory"
temporary="$target.tmp.$$"
trap 'rm -f "$temporary"' 0 1 2 15
cat > "$temporary"
chmod 700 "$temporary"
mv -f "$temporary" "$target"
trap - 0 1 2 15`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", uploadScript, "mineops", target, directory}, Input: content,
		Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "部署远端监控脚本失败", err)
	}
	return nil
}

func remoteCollectorConfigDigest(server model.MinecraftServer, jarName, managedPID, managedProcessGroupID string, intervalSeconds int) string {
	hash := sha256.New()
	for _, value := range [][]byte{
		collectorScript, remoteCollectorDaemonScript, []byte(server.ID.String()), []byte(server.RemotePath), []byte(jarName),
		[]byte(managedPID), []byte(managedProcessGroupID), []byte(strconv.Itoa(intervalSeconds)),
	} {
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (c *MetricCollector) acknowledgeRemoteMetricClaim(ctx context.Context, client *SSHClient, claimPath string, timeout time.Duration) error {
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "rm", Arguments: []string{"-f", claimPath}, Timeout: timeout, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "确认远端监控缓冲失败", err)
	}
	return nil
}

func (c *MetricCollector) stopRemoteCollector(ctx context.Context, server model.MinecraftServer) error {
	session, err := c.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	client, err := c.clients.Connect(ctx, session, c.settings.Snapshot().SSH)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	dataDirectory := path.Join(server.RemotePath, ".mineops-monitoring")
	pidPath := path.Join(dataDirectory, "collector.pid")
	daemonPath := path.Join(dataDirectory, "collector-daemon.sh")
	configPath := path.Join(dataDirectory, "collector.config")
	stopScript := `set -eu
pid_path=$1
daemon_path=$2
config_path=$3
pid=""
if [ -f "$pid_path" ]; then pid=$(cat "$pid_path" 2>/dev/null || true); fi
if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)
  case "$command_line" in *"$daemon_path"*) kill "$pid" 2>/dev/null || true ;; esac
fi
rm -f "$pid_path" "$config_path"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", stopScript, "mineops", pidPath, daemonPath, configPath},
		Timeout: 10 * time.Second, MaximumOutput: metricCollectorMaximumOutput,
	}); err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "停止远端监控守护进程失败", err)
	}
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
