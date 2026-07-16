package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/minecraftspark"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// SparkInstallPlan describes an exact approved artifact, source, target, backup, checksum, and restart impact.
type SparkInstallPlan struct {
	ServerID          model.ID                  `json:"serverID"`
	ServerName        string                    `json:"serverName"`
	ServerType        enums.MinecraftServerType `json:"serverType"`
	CurrentStatus     enums.SparkStatus         `json:"currentStatus"`
	CurrentVersion    string                    `json:"currentVersion,omitempty"`
	TargetVersion     string                    `json:"targetVersion"`
	Platform          string                    `json:"platform"`
	Provider          string                    `json:"provider"`
	ReleaseID         string                    `json:"releaseID,omitempty"`
	Source            string                    `json:"source"`
	Build             int                       `json:"build"`
	URL               string                    `json:"url"`
	ChecksumAlgorithm string                    `json:"checksumAlgorithm"`
	Checksum          string                    `json:"checksum"`
	SHA256            string                    `json:"sha256,omitempty"`
	ArtifactSize      int64                     `json:"artifactSize"`
	TargetPath        string                    `json:"targetPath"`
	BackupPath        string                    `json:"backupPath"`
	Dependencies      []SparkInstallDependency  `json:"dependencies"`
	RestartRequired   bool                      `json:"restartRequired"`
	Impact            string                    `json:"impact"`
	GeneratedAt       time.Time                 `json:"generatedAt"`
	PlanDigest        string                    `json:"planDigest"`
}

// SparkInstallDependency describes one exact dependency installed with a Spark mod.
type SparkInstallDependency struct {
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	Source            string   `json:"source"`
	URL               string   `json:"url"`
	FallbackURLs      []string `json:"fallbackURLs"`
	ChecksumAlgorithm string   `json:"checksumAlgorithm"`
	Checksum          string   `json:"checksum"`
	SHA256            string   `json:"sha256,omitempty"`
	ArtifactSize      int64    `json:"artifactSize"`
	TargetPath        string   `json:"targetPath"`
	BackupPath        string   `json:"backupPath"`
	MatchPattern      string   `json:"matchPattern"`
}

// SparkInstallResult contains the installed capability and exact plan used for the mutation.
type SparkInstallResult struct {
	Capability model.SparkCapability `json:"capability"`
	Plan       SparkInstallPlan      `json:"plan"`
}

type sparkInstalledArtifact struct {
	Name         string `json:"name"`
	TargetPath   string `json:"targetPath"`
	BackupPath   string `json:"backupPath,omitempty"`
	ExistingPath string `json:"existingPath,omitempty"`
}

type sparkInstallManifest struct {
	Artifacts []sparkInstalledArtifact `json:"artifacts"`
}

//go:embed remote_spark_collector_daemon.sh
var remoteSparkCollectorDaemonScript []byte

type remoteSparkClaim struct {
	path   string
	output string
}

type remoteSparkBatch struct {
	collectedAt time.Time
	raw         string
}

// SparkManager owns capability detection, approved installation, versioned commands, snapshots, and reports.
type SparkManager struct {
	clock      model.Clock
	store      repository.Store
	settings   *appsettings.Manager
	clients    *SSHClientFactory
	processes  *RemoteProcessController
	metrics    *MetricManager
	collector  *MetricCollector
	operations *OperationRunner
	downloads  *DownloadManager
	artifacts  *minecraftspark.Catalog
	logger     *applog.Logger

	locksMu sync.Mutex
	locks   map[model.ID]*sync.Mutex
	plansMu sync.Mutex
	plans   map[string]SparkInstallPlan
	started chan model.ID
	stopped chan model.ID
}

const (
	sparkCapabilityReprobeInterval       = 5 * time.Minute
	sparkFailedCapabilityReprobeInterval = 30 * time.Second
	remoteSparkSpoolBytes                = 4 * 1024 * 1024
	remoteSparkMaximumOutput             = remoteSparkSpoolBytes + 64*1024
)

var (
	sparkConsoleVersionPattern  = regexp.MustCompile(`(?i)\bspark\s+v?([0-9]+(?:\.[0-9]+){2})\b`)
	sparkArtifactVersionPattern = regexp.MustCompile(`(?i)\bspark[-_]v?([0-9]+(?:\.[0-9]+){2})(?:[-_.]|$)`)
)

// NewSparkManager creates the Minecraft spark application service with no Apache Spark dependency.
func NewSparkManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, clients *SSHClientFactory, processes *RemoteProcessController, metrics *MetricManager, collector *MetricCollector, operations *OperationRunner, downloads *DownloadManager, logger *applog.Logger) (*SparkManager, error) {
	if clock == nil || store == nil || settings == nil || clients == nil || processes == nil || metrics == nil || collector == nil || operations == nil || downloads == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Spark Service 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &SparkManager{
		clock: clock, store: store, settings: settings, clients: clients, processes: processes, metrics: metrics, collector: collector,
		operations: operations, downloads: downloads, artifacts: minecraftspark.NewCatalog(downloads.client), logger: logger,
		locks: make(map[model.ID]*sync.Mutex), plans: make(map[string]SparkInstallPlan), started: make(chan model.ID, 32), stopped: make(chan model.ID, 32),
	}, nil
}

// ServerStarted schedules immediate Spark collector deployment and backlog synchronisation.
func (m *SparkManager) ServerStarted(serverID model.ID) {
	if !serverID.Valid() {
		return
	}
	select {
	case m.started <- serverID:
	default:
	}
}

// ServerStopped schedules immediate shutdown of the remote Spark collector.
func (m *SparkManager) ServerStopped(serverID model.ID) {
	if !serverID.Valid() {
		return
	}
	select {
	case m.stopped <- serverID:
	default:
	}
}

func (m *SparkManager) lockServer(serverID model.ID) func() {
	m.locksMu.Lock()
	lock := m.locks[serverID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[serverID] = lock
	}
	m.locksMu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func calculateSparkPlanDigest(plan SparkInstallPlan) (string, error) {
	plan.PlanDigest = ""
	payload, err := json.Marshal(plan)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, "计算 Spark 安装计划摘要失败", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func sparkArtifactChecksum(sha256Value, sha512Value string) (string, string, error) {
	sha512Value = strings.ToLower(strings.TrimSpace(sha512Value))
	if sha512Value != "" {
		if len(sha512Value) != sha512.Size*2 {
			return "", "", apperror.New(apperror.CodeValidationInvalidArgument, "Spark Artifact SHA-512 长度无效")
		}
		if _, err := hex.DecodeString(sha512Value); err != nil {
			return "", "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "Spark Artifact SHA-512 无效", err)
		}
		return "sha512", sha512Value, nil
	}
	sha256Value = strings.ToLower(strings.TrimSpace(sha256Value))
	if len(sha256Value) != sha256.Size*2 {
		return "", "", apperror.New(apperror.CodeValidationInvalidArgument, "Spark Artifact SHA-256 长度无效")
	}
	if _, err := hex.DecodeString(sha256Value); err != nil {
		return "", "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "Spark Artifact SHA-256 无效", err)
	}
	return "sha256", sha256Value, nil
}

func (m *SparkManager) rememberPlan(plan SparkInstallPlan) {
	m.plansMu.Lock()
	defer m.plansMu.Unlock()
	for digest, existing := range m.plans {
		if existing.ServerID == plan.ServerID || m.clock.Now().UTC().Sub(existing.GeneratedAt) > 15*time.Minute {
			delete(m.plans, digest)
		}
	}
	m.plans[plan.PlanDigest] = plan
}

func (m *SparkManager) confirmedPlan(serverID model.ID, digest string) (SparkInstallPlan, error) {
	if len(digest) != sha256.Size*2 {
		return SparkInstallPlan{}, apperror.New(apperror.CodeValidationConflict, "Spark 安装计划摘要无效，请重新生成计划")
	}
	m.plansMu.Lock()
	plan, found := m.plans[digest]
	m.plansMu.Unlock()
	if !found || plan.ServerID != serverID || m.clock.Now().UTC().Sub(plan.GeneratedAt) > 15*time.Minute {
		return SparkInstallPlan{}, apperror.New(apperror.CodeValidationConflict, "Spark 安装计划已过期或不属于当前 Server，请重新生成计划")
	}
	return plan, nil
}

func (m *SparkManager) forgetPlan(digest string) {
	m.plansMu.Lock()
	delete(m.plans, digest)
	m.plansMu.Unlock()
}

// Run periodically collects locked Spark TPS/MSPT snapshots for running Servers with an available saved capability.
func (m *SparkManager) Run(ctx context.Context) error {
	settingsChanged := make(chan struct{}, 1)
	unsubscribe := m.settings.Subscribe(func(change appsettings.Change) {
		if change.Category == enums.SettingsMonitoring {
			select {
			case settingsChanged <- struct{}{}:
			default:
			}
		}
	})
	defer unsubscribe()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case serverID := <-m.started:
			go m.syncRunningServer(ctx, serverID)
		case serverID := <-m.stopped:
			go func(serverID model.ID) {
				server, err := m.store.MinecraftServers().Get(context.WithoutCancel(ctx), serverID, false)
				if err == nil {
					_ = m.stopRemoteCollector(context.WithoutCancel(ctx), *server)
				}
			}(serverID)
		case <-settingsChanged:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			interval := time.Duration(m.settings.Snapshot().Monitoring.SparkIntervalSeconds) * time.Second
			timer.Reset(interval)
		case <-timer.C:
			m.collectRunningServers(ctx)
			interval := time.Duration(m.settings.Snapshot().Monitoring.SparkIntervalSeconds) * time.Second
			timer.Reset(interval)
		}
	}
}

func (m *SparkManager) collectRunningServers(ctx context.Context) {
	servers, err := m.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{State: enums.LifecycleRunning, Limit: 500})
	if err != nil {
		m.logger.Error(context.WithoutCancel(ctx), "查询 Spark 周期采集 Server 失败", err, applog.Fields{"component": "spark-collector"})
		return
	}
	var wait sync.WaitGroup
	slots := make(chan struct{}, 4)
	for _, server := range servers {
		if m.collector.IsPaused(server.ID) {
			continue
		}
		server := server
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			m.syncRunningServer(ctx, server.ID)
		}()
	}
	wait.Wait()
}

func (m *SparkManager) syncRunningServer(ctx context.Context, serverID model.ID) {
	if m.collector.IsPaused(serverID) {
		return
	}
	unlock := m.lockServer(serverID)
	defer unlock()
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil || server.State != enums.LifecycleRunning {
		return
	}
	capability, capabilityErr := m.GetCapability(ctx, server.ID)
	if capabilityErr != nil {
		if apperror.ToDTO(capabilityErr).Code != apperror.CodeIONotFound.String() {
			m.logger.Warn(context.WithoutCancel(ctx), "读取 Spark 周期采集能力失败", applog.Fields{"server_id": server.ID.String(), "error": capabilityErr.Error()})
			return
		}
		probed, probeErr := m.probeLocked(ctx, server.ID)
		if probeErr != nil {
			m.logger.Warn(context.WithoutCancel(ctx), "首次探测运行中 Server 的 Spark 能力失败", applog.Fields{"server_id": server.ID.String(), "error": probeErr.Error()})
			return
		}
		capability = &probed
	}
	if shouldReprobeSparkCapability(capability, m.clock.Now().UTC()) {
		probed, probeErr := m.probeLocked(ctx, server.ID)
		if probeErr != nil {
			m.logger.Warn(context.WithoutCancel(ctx), "刷新 Spark Parser Capability 失败", applog.Fields{"server_id": server.ID.String(), "error": probeErr.Error()})
			return
		}
		capability = &probed
	}
	if capability.Status != enums.SparkAvailable || !minecraftspark.SupportsPluginVersion(capability.PluginVersion) || !capability.TPSSupported {
		return
	}
	if syncErr := m.syncRemoteSnapshotsLocked(ctx, *server, capability); syncErr != nil && ctx.Err() == nil {
		m.recordCapabilityFailure(context.WithoutCancel(ctx), capability, syncErr)
		m.logger.Warn(context.WithoutCancel(ctx), "Spark 远端周期采集同步失败", applog.Fields{"server_id": server.ID.String(), "error": syncErr.Error()})
	}
}

func shouldReprobeSparkCapability(capability *model.SparkCapability, now time.Time) bool {
	if capability == nil {
		return true
	}
	if capability.RestartRequired || capability.Status == enums.SparkUnknown {
		return true
	}
	if capability.Status == enums.SparkUnavailable && capability.InstallManifest != "" {
		return true
	}
	if capability.Status == enums.SparkFailed {
		return now.Sub(capability.DetectedAt) >= sparkFailedCapabilityReprobeInterval
	}
	if capability.Status == enums.SparkUnavailable {
		return now.Sub(capability.DetectedAt) >= sparkCapabilityReprobeInterval
	}
	return capability.ParserVersion != minecraftspark.ParserVersion || capability.SourceSchemaHash != minecraftspark.SourceSchemaHash
}

// Probe detects exact artifact, server family, version compatibility, running permission, and collection method.
func (m *SparkManager) Probe(ctx context.Context, serverID model.ID) (model.SparkCapability, error) {
	unlock := m.lockServer(serverID)
	defer unlock()
	return m.probeLocked(ctx, serverID)
}

func (m *SparkManager) probeLocked(ctx context.Context, serverID model.ID) (model.SparkCapability, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return model.SparkCapability{}, err
	}
	now := m.clock.Now().UTC()
	capability := model.SparkCapability{
		ServerID: server.ID, Status: enums.SparkUnknown, ServerType: server.Type,
		Platform: string(minecraftspark.PlatformForServerType(server.Type)), DetectedAt: now, SchemaVersion: model.SparkSchemaVersion,
	}
	if previous, previousErr := m.store.Spark().GetCapability(ctx, serverID); previousErr == nil {
		capability.ArtifactPath = previous.ArtifactPath
		capability.BackupPath = previous.BackupPath
		capability.InstallManifest = previous.InstallManifest
	}
	artifact, artifactErr := minecraftspark.ArtifactForServer(server.Type, server.Version)
	if artifactErr == nil {
		capability.Distribution = artifact.Platform
	} else if server.Type == enums.ServerPaper && minecraftspark.PaperUsesBundledSpark(server.Version) {
		capability.Distribution = "built-in"
	}
	versionOutput, commandErr := m.executeCommandLocked(ctx, *server, "version", 0)
	if commandErr == nil {
		capability.Installed = true
		capability.PermissionGranted = true
		capability.ReportSupported = true
		capability.PluginVersion = detectSparkVersion(versionOutput)
		if minecraftspark.SupportsPluginVersion(capability.PluginVersion) {
			capability.TPSSupported, capability.MSPTSupported = sparkMetricCapabilities(server.Type)
			capability.Status = enums.SparkAvailable
			capability.ParserVersion = minecraftspark.ParserVersion
			capability.CollectionMethod = string(minecraftspark.CollectionRCONText)
			capability.SourceSchemaHash = minecraftspark.SourceSchemaHash
		} else {
			capability.Status = enums.SparkUnsupported
			capability.LastErrorCode = apperror.CodeSparkUnsupported.String()
			capability.LastError = "检测到 Minecraft spark，但插件版本不在锁定解析矩阵中"
		}
	}
	if capability.Status == enums.SparkUnknown && artifactErr == nil {
		artifactPath, foundVersion, fileErr := m.detectArtifact(ctx, *server, artifact.TargetKind)
		if fileErr != nil {
			capability.LastErrorCode = apperror.ToDTO(fileErr).Code
			capability.LastError = apperror.ToDTO(fileErr).Message
		} else if artifactPath != "" {
			capability.Installed = true
			capability.ArtifactPath = artifactPath
			capability.PluginVersion = foundVersion
			if minecraftspark.SupportsPluginVersion(foundVersion) {
				capability.TPSSupported, capability.MSPTSupported = sparkMetricCapabilities(server.Type)
				capability.ParserVersion = minecraftspark.ParserVersion
				capability.CollectionMethod = string(minecraftspark.CollectionRCONText)
				capability.SourceSchemaHash = minecraftspark.SourceSchemaHash
				capability.Status = enums.SparkUnavailable
				capability.LastErrorCode = apperror.CodeSparkUnavailable.String()
				commandFailure := apperror.ToDTO(commandErr)
				commandFailureReason, _ := commandFailure.Details["reason"].(string)
				switch {
				case commandFailureReason == "command_unregistered":
					capability.RestartRequired = true
					capability.LastError = "检测到受支持的 Spark Artifact，但运行时命令尚未注册；安装或升级后必须重启 Minecraft Server。"
				case server.State == enums.LifecycleRunning || server.State == enums.LifecycleStarting:
					capability.LastError = commandFailure.Message
					if capability.LastError == "" {
						capability.LastError = "检测到受支持的 Spark Artifact，但运行时命令尚未通过探测"
					}
				default:
					capability.LastError = "检测到受支持的 Spark Artifact；启动 Minecraft Server 后重新探测即可采集"
				}
			} else {
				capability.Status = enums.SparkUnsupported
				capability.LastErrorCode = apperror.CodeSparkUnsupported.String()
				capability.LastError = "远程 Spark Artifact 版本不在锁定解析矩阵中"
			}
		} else {
			capability.Status = enums.SparkUnavailable
			capability.LastErrorCode = apperror.CodeSparkUnavailable.String()
			capability.LastError = sparkUnavailableProbeMessage(*server, commandErr)
		}
	}
	if artifactErr != nil && capability.Status == enums.SparkUnknown {
		capability.Status = enums.SparkUnsupported
		capability.LastErrorCode = apperror.CodeSparkUnsupported.String()
		capability.LastError = "当前 Server 类型没有批准的自动安装 Artifact"
	}
	if err := capability.Validate(); err != nil {
		return model.SparkCapability{}, err
	}
	if err := m.store.Spark().SaveCapability(ctx, &capability); err != nil {
		return model.SparkCapability{}, err
	}
	return capability, nil
}

// sparkUnavailableProbeMessage preserves the actual console probe failure without claiming every platform has built-in Spark.
func sparkUnavailableProbeMessage(server model.MinecraftServer, commandErr error) string {
	builtIn := server.Type == enums.ServerPaper || server.Type == enums.ServerPurpur || server.Type == enums.ServerFolia
	running := server.State == enums.LifecycleRunning || server.State == enums.LifecycleStarting
	if !running {
		if builtIn {
			return "未检测到独立的 Minecraft spark Artifact；服务器当前未运行，内置 Spark 需要启动后通过控制台探测"
		}
		return "未检测到 Minecraft spark Artifact；启动服务器或安装 Spark 后重新探测"
	}
	failure := apperror.ToDTO(commandErr)
	if reason, _ := failure.Details["reason"].(string); reason == "command_unregistered" {
		return "运行中的服务器未注册 Spark 命令，且未检测到独立的 Minecraft spark Artifact"
	}
	if failure.Message != "" {
		return failure.Message
	}
	if builtIn {
		return "未检测到独立的 Minecraft spark Artifact，且无法通过运行中的服务器控制台探测内置 Spark"
	}
	return "未检测到 Minecraft spark Artifact，且运行中的服务器无法执行 Spark 探测命令"
}

// PlanInstall computes the exact approved artifact, target path, backup location, and restart impact without mutating the Server.
func (m *SparkManager) PlanInstall(ctx context.Context, serverID model.ID) (SparkInstallPlan, error) {
	unlock := m.lockServer(serverID)
	defer unlock()
	plan, err := m.planInstallLocked(ctx, serverID, m.clock.Now().UTC())
	if err != nil {
		return SparkInstallPlan{}, err
	}
	m.rememberPlan(plan)
	return plan, nil
}

func (m *SparkManager) planInstallLocked(ctx context.Context, serverID model.ID, generatedAt time.Time) (SparkInstallPlan, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return SparkInstallPlan{}, err
	}
	artifact, err := m.artifacts.ResolveArtifact(ctx, server.Type, server.Version)
	if err != nil {
		if server.Type == enums.ServerPaper && minecraftspark.PaperUsesBundledSpark(server.Version) {
			return SparkInstallPlan{}, apperror.New(apperror.CodeSparkUnsupported, "Paper 1.21+ 已内置 Spark，MineOps 不自动安装外置 Jar")
		}
		return SparkInstallPlan{}, apperror.Wrap(apperror.CodeSparkUnsupported, "当前 Server 类型没有批准的 Spark Artifact", err)
	}
	current, probeErr := m.probeLocked(ctx, serverID)
	if probeErr != nil {
		current = model.SparkCapability{Status: enums.SparkUnknown}
	}
	targetDirectory := path.Join(server.RemotePath, artifact.TargetKind)
	if strings.TrimSpace(artifact.FileName) == "" {
		return SparkInstallPlan{}, apperror.New(apperror.CodeValidationInvalidArgument, "Spark Artifact 文件名不能为空")
	}
	targetPath := path.Join(targetDirectory, artifact.FileName)
	timestamp := generatedAt.Format("20060102T150405.000000000Z")
	backupPath := path.Join(server.RemotePath, ".mineops-spark-backups", "spark-"+timestamp+".jar")
	restartRequired := server.State == enums.LifecycleRunning || server.State == enums.LifecycleStarting
	artifactURL, err := m.downloads.ResolveArtifactURL(artifact.Provider, artifact.URL)
	if err != nil {
		return SparkInstallPlan{}, err
	}
	checksumAlgorithm, checksum, err := sparkArtifactChecksum(artifact.SHA256, artifact.SHA512)
	if err != nil {
		return SparkInstallPlan{}, err
	}
	dependencies := make([]SparkInstallDependency, 0, len(artifact.Dependencies))
	for _, dependency := range artifact.Dependencies {
		dependencyURL, resolveErr := m.downloads.ResolveArtifactURL("fabric-api", dependency.URL)
		if resolveErr != nil {
			return SparkInstallPlan{}, resolveErr
		}
		fallbackURLs := make([]string, 0, len(dependency.FallbackURLs))
		for _, fallback := range dependency.FallbackURLs {
			fallbackURL, fallbackErr := m.downloads.ResolveArtifactURL("fabric-api", fallback)
			if fallbackErr != nil {
				return SparkInstallPlan{}, fallbackErr
			}
			fallbackURLs = append(fallbackURLs, fallbackURL)
		}
		dependencyChecksumAlgorithm, dependencyChecksum, checksumErr := sparkArtifactChecksum(dependency.SHA256, dependency.SHA512)
		if checksumErr != nil {
			return SparkInstallPlan{}, checksumErr
		}
		dependencies = append(dependencies, SparkInstallDependency{
			Name: dependency.Name, Version: dependency.Version, Source: dependencyURL, URL: dependencyURL,
			ChecksumAlgorithm: dependencyChecksumAlgorithm, Checksum: dependencyChecksum, SHA256: dependency.SHA256, ArtifactSize: dependency.Size,
			FallbackURLs: fallbackURLs,
			TargetPath:   path.Join(server.RemotePath, dependency.TargetKind, dependency.FileName),
			BackupPath:   path.Join(server.RemotePath, ".mineops-spark-backups", strings.TrimSuffix(dependency.FileName, path.Ext(dependency.FileName))+"-"+timestamp+path.Ext(dependency.FileName)),
			MatchPattern: dependency.MatchPattern,
		})
	}
	impact := "将从已批准的 Minecraft spark 下载源获取与当前 Server 兼容的精确版本，校验官方 " + strings.ToUpper(checksumAlgorithm) + "，备份现有 Spark Artifact 后原子替换。"
	if len(dependencies) > 0 {
		impact += " 同时安装与当前 Minecraft 版本匹配的锁定依赖："
		for index, dependency := range dependencies {
			if index > 0 {
				impact += "、"
			}
			impact += dependency.Name + " " + dependency.Version
		}
		impact += "。"
	}
	if restartRequired {
		impact += " Server 当前正在运行，替换后必须重启才能加载新版本。"
	} else {
		impact += " 下次启动时加载新版本。"
	}
	plan := SparkInstallPlan{
		ServerID: server.ID, ServerName: server.Name, ServerType: server.Type, CurrentStatus: current.Status, CurrentVersion: current.PluginVersion,
		TargetVersion: artifact.Version, Platform: artifact.Platform, Provider: artifact.Provider, ReleaseID: artifact.ReleaseID, Source: artifactURL, Build: artifact.Build,
		URL: artifactURL, ChecksumAlgorithm: checksumAlgorithm, Checksum: checksum, SHA256: artifact.SHA256, ArtifactSize: artifact.Size, TargetPath: targetPath, BackupPath: backupPath,
		Dependencies: dependencies, RestartRequired: restartRequired, Impact: impact, GeneratedAt: generatedAt,
	}
	plan.PlanDigest, err = calculateSparkPlanDigest(plan)
	if err != nil {
		return SparkInstallPlan{}, err
	}
	return plan, nil
}

// Install downloads, verifies, backs up, and atomically replaces one approved Spark artifact after explicit confirmation.
func (m *SparkManager) Install(ctx context.Context, serverID model.ID, planDigest string, confirmed bool) (SparkInstallResult, error) {
	if !confirmed {
		return SparkInstallResult{}, apperror.New(apperror.CodeValidationConflict, "安装或升级 Spark 前必须确认批准来源、目标路径和重启影响")
	}
	unlock := m.lockServer(serverID)
	defer unlock()
	plan, err := m.confirmedPlan(serverID, planDigest)
	if err != nil {
		return SparkInstallResult{}, err
	}
	currentPlan, err := m.planInstallLocked(ctx, serverID, plan.GeneratedAt)
	if err != nil {
		return SparkInstallResult{}, err
	}
	if currentPlan.PlanDigest != plan.PlanDigest {
		return SparkInstallResult{}, apperror.New(apperror.CodeValidationConflict, "Spark 安装计划与当前 Server 或下载设置不一致，请重新生成并确认")
	}
	plan = currentPlan
	server, session, client, err := m.connect(ctx, serverID)
	if err != nil {
		return SparkInstallResult{}, err
	}
	defer func() { _ = client.Close() }()
	type plannedArtifact struct {
		name, checksumAlgorithm, checksum, targetPath, backupPath, matchPattern, temporaryPath string
		expectedSize                                                                           int64
		urls                                                                                   []string
	}
	artifacts := make([]plannedArtifact, 0, len(plan.Dependencies)+1)
	for index, dependency := range plan.Dependencies {
		urls := append([]string{dependency.URL}, dependency.FallbackURLs...)
		artifacts = append(artifacts, plannedArtifact{
			name: dependency.Name, urls: urls, checksumAlgorithm: dependency.ChecksumAlgorithm, checksum: dependency.Checksum, expectedSize: dependency.ArtifactSize, targetPath: dependency.TargetPath,
			backupPath: dependency.BackupPath, matchPattern: dependency.MatchPattern,
			temporaryPath: path.Join(server.RemotePath, fmt.Sprintf(".mineops-spark-download-%s-%d.tmp", plan.PlanDigest[:16], index)),
		})
	}
	artifacts = append(artifacts, plannedArtifact{
		name: "Spark", urls: []string{plan.URL}, checksumAlgorithm: plan.ChecksumAlgorithm, checksum: plan.Checksum, expectedSize: plan.ArtifactSize, targetPath: plan.TargetPath, backupPath: plan.BackupPath,
		matchPattern: "spark*.jar", temporaryPath: path.Join(server.RemotePath, fmt.Sprintf(".mineops-spark-download-%s-%d.tmp", plan.PlanDigest[:16], len(artifacts))),
	})
	temporaryPaths := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		temporaryPaths = append(temporaryPaths, artifact.temporaryPath)
	}
	defer func() {
		_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{Executable: "rm", Arguments: append([]string{"-f"}, temporaryPaths...), Timeout: 30 * time.Second, MaximumOutput: 4096})
	}()
	localDirectory, err := os.MkdirTemp("", "mineops-spark-*")
	if err != nil {
		return SparkInstallResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Spark 本地下载目录失败", err)
	}
	defer func() { _ = os.RemoveAll(localDirectory) }()
	downloader := httpclient.NewDownloader(m.downloads.client)
	for index, artifact := range artifacts {
		localPath := filepath.Join(localDirectory, fmt.Sprintf("artifact-%d.jar", index))
		downloadErrors := make([]string, 0, len(artifact.urls))
		downloaded := false
		for _, source := range artifact.urls {
			downloadRequest := httpclient.DownloadRequest{URL: source, Destination: localPath, ExpectedSize: artifact.expectedSize, MaximumSize: 64 * 1024 * 1024}
			if artifact.checksumAlgorithm == "sha512" {
				downloadRequest.ExpectedSHA512 = artifact.checksum
			} else {
				downloadRequest.ExpectedSHA256 = artifact.checksum
			}
			if _, downloadErr := downloader.Download(ctx, downloadRequest, nil); downloadErr != nil {
				downloadErrors = append(downloadErrors, source+": "+downloadErr.Error())
				continue
			}
			downloaded = true
			break
		}
		if !downloaded {
			return SparkInstallResult{}, apperror.New(apperror.CodeArtifactChecksumMismatch, artifact.name+" 所有批准下载源均失败").WithDetails(map[string]any{
				"sources": artifact.urls, "expectedChecksumAlgorithm": artifact.checksumAlgorithm, "expectedChecksum": artifact.checksum, "downloadErrors": downloadErrors,
			})
		}
		payload, readErr := os.ReadFile(localPath)
		if readErr != nil {
			return SparkInstallResult{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取已校验的 "+artifact.name+" Artifact 失败", readErr)
		}
		if uploadErr := uploadRemoteFile(ctx, client, artifact.temporaryPath, payload); uploadErr != nil {
			return SparkInstallResult{}, apperror.Wrap(apperror.CodeSFTPTransferFailed, "上传 "+artifact.name+" Artifact 失败", uploadErr)
		}
	}
	script := `set -eu
pattern=$1
target=$2
backup=$3
temporary=$4
expected=$5
algorithm=$6
mkdir -p -- "$(dirname "$target")" "$(dirname "$backup")"
case "$algorithm" in
  sha256) actual=$(sha256sum "$temporary" | awk '{print $1}') ;;
  sha512) actual=$(sha512sum "$temporary" | awk '{print $1}') ;;
  *) printf 'unsupported checksum algorithm: %s\n' "$algorithm" >&2; exit 1 ;;
esac
if [ "$actual" != "$expected" ]; then printf 'remote %s mismatch: expected %s, got %s\n' "$algorithm" "$expected" "$actual" >&2; exit 1; fi
matches=$(find "$(dirname "$target")" -maxdepth 1 -type f -iname "$pattern" -print 2>/dev/null || true)
count=$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')
if [ "$count" -gt 1 ]; then printf 'multiple matching artifacts for %s\n%s\n' "$pattern" "$matches" >&2; exit 1; fi
existing=$matches
if [ -n "$existing" ]; then mv -- "$existing" "$backup"; else backup=''; fi
if ! mv -- "$temporary" "$target"; then
  [ -z "$backup" ] || mv -- "$backup" "$existing"
  exit 1
fi
printf 'target=%s\nbackup=%s\nexisting=%s\n' "$target" "$backup" "$existing"`
	installed := make([]sparkInstalledArtifact, 0, len(artifacts))
	mainValues := map[string]string{}
	for _, artifact := range artifacts {
		result, commandErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh", Arguments: []string{"-c", script, "mineops", artifact.matchPattern, artifact.targetPath, artifact.backupPath, artifact.temporaryPath, artifact.checksum, artifact.checksumAlgorithm},
			Timeout: 5 * time.Minute, MaximumOutput: 32 * 1024,
		})
		if commandErr != nil {
			rollbackErrors := make([]string, 0)
			for index := len(installed) - 1; index >= 0; index-- {
				changed := installed[index]
				rollbackScript := `set -eu
target=$1
backup=$2
existing=$3
rm -f -- "$target"
if [ -n "$backup" ] && [ -f "$backup" ]; then mv -- "$backup" "$existing"; fi`
				rollbackResult, rollbackErr := client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{
					Executable: "sh", Arguments: []string{"-c", rollbackScript, "mineops", changed.TargetPath, changed.BackupPath, changed.ExistingPath},
					Timeout: 30 * time.Second, MaximumOutput: 4096,
				})
				if rollbackErr != nil {
					rollbackErrors = append(rollbackErrors, changed.Name+": "+strings.TrimSpace(rollbackResult.Stderr))
				}
			}
			return SparkInstallResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, artifact.name+" 远程校验、备份或替换失败", commandErr).WithDetails(map[string]any{
				"stderr": strings.TrimSpace(result.Stderr), "sources": artifact.urls, "expectedChecksumAlgorithm": artifact.checksumAlgorithm, "expectedChecksum": artifact.checksum,
				"rollbackErrors": rollbackErrors, "sshSessionID": session.ID.String(),
			})
		}
		values := parseKeyValueLines(result.Stdout)
		installed = append(installed, sparkInstalledArtifact{Name: artifact.name, TargetPath: values["target"], BackupPath: values["backup"], ExistingPath: values["existing"]})
		if artifact.name == "Spark" {
			mainValues = values
		}
	}
	manifestPayload, err := json.Marshal(sparkInstallManifest{Artifacts: installed})
	if err != nil {
		return SparkInstallResult{}, apperror.Wrap(apperror.CodeInternal, "序列化 Spark 安装回滚清单失败", err)
	}
	recordedBackupPath := mainValues["backup"]
	if recordedBackupPath == "" {
		for _, artifact := range installed {
			if artifact.BackupPath != "" {
				recordedBackupPath = artifact.BackupPath
				break
			}
		}
	}
	tpsSupported, msptSupported := sparkMetricCapabilities(server.Type)
	lastError := "Spark Artifact 已安装；启动 Minecraft Server 后重新探测即可采集。"
	if plan.RestartRequired {
		lastError = "Spark Artifact 已安装，必须重启 Minecraft Server 后才能探测和采集。"
	}
	capability := model.SparkCapability{
		ServerID: server.ID, Status: enums.SparkUnavailable, ServerType: server.Type,
		Platform: string(minecraftspark.PlatformForServerType(server.Type)), Distribution: plan.Platform,
		Installed: true, PluginVersion: plan.TargetVersion, ParserVersion: minecraftspark.ParserVersion,
		CollectionMethod: string(minecraftspark.CollectionRCONText), SourceSchemaHash: minecraftspark.SourceSchemaHash,
		TPSSupported: tpsSupported, MSPTSupported: msptSupported,
		ArtifactPath: mainValues["target"], BackupPath: recordedBackupPath, InstallManifest: string(manifestPayload), RestartRequired: plan.RestartRequired,
		DetectedAt: m.clock.Now().UTC(), LastErrorCode: apperror.CodeSparkUnavailable.String(), LastError: lastError, SchemaVersion: model.SparkSchemaVersion,
	}
	if err := m.store.Spark().SaveCapability(ctx, &capability); err != nil {
		return SparkInstallResult{}, err
	}
	m.forgetPlan(plan.PlanDigest)
	return SparkInstallResult{Capability: capability, Plan: plan}, nil
}

// Rollback restores the latest recorded Spark backup and preserves the replaced artifact for diagnosis.
func (m *SparkManager) Rollback(ctx context.Context, serverID model.ID, confirmed bool) (model.SparkCapability, error) {
	if !confirmed {
		return model.SparkCapability{}, apperror.New(apperror.CodeValidationConflict, "Spark 回滚前必须确认替换影响")
	}
	unlock := m.lockServer(serverID)
	defer unlock()
	capability, err := m.GetCapability(ctx, serverID)
	if err != nil {
		return model.SparkCapability{}, err
	}
	server, _, client, err := m.connect(ctx, serverID)
	if err != nil {
		return model.SparkCapability{}, err
	}
	defer func() { _ = client.Close() }()
	manifest := sparkInstallManifest{}
	if capability.InstallManifest != "" {
		if err := json.Unmarshal([]byte(capability.InstallManifest), &manifest); err != nil {
			return model.SparkCapability{}, apperror.Wrap(apperror.CodeValidationConflict, "Spark 回滚清单无效", err)
		}
	} else if capability.ArtifactPath != "" {
		manifest.Artifacts = []sparkInstalledArtifact{{Name: "Spark", TargetPath: capability.ArtifactPath, BackupPath: capability.BackupPath, ExistingPath: capability.ArtifactPath}}
	}
	if len(manifest.Artifacts) == 0 {
		return model.SparkCapability{}, apperror.New(apperror.CodeValidationConflict, "没有可用的 Spark 安装回滚清单")
	}
	script := `set -eu
target=$1
backup=$2
existing=$3
failed="${target}.failed.$(date +%s%N)"
[ ! -e "$target" ] || mv -- "$target" "$failed"
if [ -n "$backup" ]; then
  test -f "$backup"
  [ -n "$existing" ] || existing="$target"
  if ! mv -- "$backup" "$existing"; then [ ! -e "$failed" ] || mv -- "$failed" "$target"; exit 1; fi
fi`
	rollbackErrors := make([]string, 0)
	mainArtifactPath := ""
	for index := len(manifest.Artifacts) - 1; index >= 0; index-- {
		artifact := manifest.Artifacts[index]
		if !safeSparkPath(server.RemotePath, artifact.TargetPath) || artifact.BackupPath != "" && !safeSparkPath(server.RemotePath, artifact.BackupPath) || artifact.ExistingPath != "" && !safeSparkPath(server.RemotePath, artifact.ExistingPath) {
			return model.SparkCapability{}, apperror.New(apperror.CodeValidationConflict, "Spark 回滚路径不在 Server 受控目录中")
		}
		result, rollbackErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh", Arguments: []string{"-c", script, "mineops", artifact.TargetPath, artifact.BackupPath, artifact.ExistingPath},
			Timeout: 30 * time.Second, MaximumOutput: 16 * 1024,
		})
		if rollbackErr != nil {
			rollbackErrors = append(rollbackErrors, artifact.Name+": "+strings.TrimSpace(result.Stderr))
		}
		if artifact.Name == "Spark" && artifact.BackupPath != "" {
			mainArtifactPath = artifact.ExistingPath
			if mainArtifactPath == "" {
				mainArtifactPath = artifact.TargetPath
			}
		}
	}
	if len(rollbackErrors) > 0 {
		return model.SparkCapability{}, apperror.New(apperror.CodeIOWriteFailed, "Spark 安装清单回滚不完整").WithDetails(map[string]any{"rollbackErrors": rollbackErrors})
	}
	capability.Status = enums.SparkUnknown
	capability.Installed = mainArtifactPath != ""
	capability.PluginVersion = ""
	capability.ParserVersion = ""
	capability.CollectionMethod = ""
	capability.SourceSchemaHash = ""
	capability.TPSSupported = false
	capability.MSPTSupported = false
	capability.ReportSupported = false
	capability.PermissionGranted = false
	capability.ArtifactPath = mainArtifactPath
	capability.BackupPath = ""
	capability.InstallManifest = ""
	capability.RestartRequired = server.State == enums.LifecycleRunning || server.State == enums.LifecycleStarting
	capability.DetectedAt = m.clock.Now().UTC()
	capability.LastErrorCode = apperror.CodeSparkUnavailable.String()
	capability.LastError = "Spark 安装清单已回滚；启动或重启 Minecraft Server 后将重新探测实际版本。"
	if err := m.store.Spark().SaveCapability(ctx, capability); err != nil {
		return model.SparkCapability{}, err
	}
	return *capability, nil
}

func (m *SparkManager) syncRemoteSnapshotsLocked(ctx context.Context, server model.MinecraftServer, capability *model.SparkCapability) error {
	identity, err := m.store.ProcessIdentities().GetByServer(ctx, server.ID)
	if err != nil {
		return apperror.Wrap(apperror.CodeSparkUnavailable, "Spark 远端采集需要运行中的 tmux Console", err)
	}
	if identity.TmuxSession == "" {
		return apperror.New(apperror.CodeSparkUnavailable, "Spark 远端采集需要运行中的 tmux Console")
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	command, err := sparkAllowedCommand(server.Type, "tps", 0)
	if err != nil {
		return err
	}
	claim, err := m.collectRemoteBufferedSnapshots(ctx, client, server, identity.TmuxSession, command)
	if err != nil || strings.TrimSpace(claim.output) == "" {
		return err
	}
	batches, err := parseRemoteSparkBatches(claim.output)
	if err != nil {
		return err
	}
	var latestCollectedAt time.Time
	if latest, latestErr := m.store.Spark().LatestSnapshot(ctx, server.ID); latestErr == nil {
		latestCollectedAt = latest.CollectedAt
	}
	validBatches := 0
	var parseFailure error
	for _, batch := range batches {
		if !latestCollectedAt.IsZero() && !batch.collectedAt.After(latestCollectedAt) {
			continue
		}
		parsed, parseErr := minecraftspark.ParseTPSResponse(minecraftspark.AdapterKey{
			Platform: minecraftspark.PlatformForServerType(server.Type), Distribution: capability.Distribution,
			PluginVersion: capability.PluginVersion, CollectionMethod: minecraftspark.CollectionRCONText,
			ParserVersion: capability.ParserVersion, SourceSchemaHash: capability.SourceSchemaHash,
		}, batch.raw)
		if parseErr != nil {
			rawDigest := sha256.Sum256([]byte(batch.raw))
			parseFailure = apperror.Wrap(apperror.CodeSparkParseFailed, "远端 Spark TPS/MSPT 响应不符合锁定 Grammar", parseErr).WithDetails(map[string]any{
				"rawResponseSHA256": hex.EncodeToString(rawDigest[:]), "rawResponseBytes": len(batch.raw),
			})
			continue
		}
		validBatches++
		if _, err := m.persistSnapshot(ctx, server, capability, parsed, batch.collectedAt); err != nil {
			return err
		}
		latestCollectedAt = batch.collectedAt
	}
	if err := m.acknowledgeRemoteSparkClaim(ctx, client, claim.path); err != nil {
		return err
	}
	if validBatches == 0 && parseFailure != nil {
		return parseFailure
	}
	return nil
}

func (m *SparkManager) collectRemoteBufferedSnapshots(ctx context.Context, client *SSHClient, server model.MinecraftServer, tmuxSession, command string) (remoteSparkClaim, error) {
	dataDirectory := path.Join(server.RemotePath, ".mineops-monitoring")
	daemonPath := path.Join(dataDirectory, "spark-collector-daemon.sh")
	if err := uploadRemoteCollectorFile(ctx, client, dataDirectory, daemonPath, remoteSparkCollectorDaemonScript, 30*time.Second); err != nil {
		return remoteSparkClaim{}, err
	}
	intervalSeconds := m.settings.Snapshot().Monitoring.SparkIntervalSeconds
	if intervalSeconds < 5 {
		intervalSeconds = 5
	}
	configDigest := remoteSparkCollectorConfigDigest(server, tmuxSession, command, intervalSeconds)
	startScript := `set -eu
data_dir=$1
daemon_path=$2
tmux_session=$3
spark_command=$4
interval=$5
maximum_spool_bytes=$6
config_digest=$7
pid_path="$data_dir/spark.pid"
config_path="$data_dir/spark.config"
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
if [ "$running" -eq 0 ]; then
  if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
    command_line=$(tr '\000' ' ' < "/proc/$old_pid/cmdline" 2>/dev/null || true)
    case "$command_line" in *"$daemon_path"*) kill "$old_pid" 2>/dev/null || true ;; esac
  fi
  config_tmp="$config_path.tmp.$$"
  printf '%s\n' "$config_digest" > "$config_tmp"
  mv -f "$config_tmp" "$config_path"
  if command -v nohup >/dev/null 2>&1; then
    nohup sh "$daemon_path" "$data_dir" "$tmux_session" "$spark_command" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
  else
    sh "$daemon_path" "$data_dir" "$tmux_session" "$spark_command" "$interval" "$maximum_spool_bytes" >/dev/null 2>&1 </dev/null &
  fi
  printf '%s\n' "$!" > "$pid_path"
fi`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", startScript, "mineops", dataDirectory, daemonPath, tmuxSession, command, strconv.Itoa(intervalSeconds), strconv.Itoa(remoteSparkSpoolBytes), configDigest},
		Timeout: 30 * time.Second, MaximumOutput: 16 * 1024,
	}); err != nil {
		return remoteSparkClaim{}, apperror.Wrap(apperror.CodeSparkUnavailable, "启动远端 Spark 采集守护进程失败", err)
	}
	claimPath := path.Join(dataDirectory, "spark.claim")
	spoolPath := path.Join(dataDirectory, "spark.spool")
	claimScript := `set -eu
claim_path=$1
spool_path=$2
attempt=0
while [ ! -s "$claim_path" ] && [ ! -s "$spool_path" ] && [ "$attempt" -lt 4 ]; do
  sleep 1
  attempt=$((attempt + 1))
done
if [ ! -s "$claim_path" ] && [ -s "$spool_path" ]; then mv "$spool_path" "$claim_path"; fi
if [ -s "$claim_path" ]; then cat "$claim_path"; fi`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", claimScript, "mineops", claimPath, spoolPath},
		Timeout: 30 * time.Second, MaximumOutput: remoteSparkMaximumOutput,
	})
	if err != nil {
		return remoteSparkClaim{}, apperror.Wrap(apperror.CodeSparkUnavailable, "领取远端 Spark 采集缓冲失败", err)
	}
	if result.Truncated {
		return remoteSparkClaim{}, apperror.New(apperror.CodeSparkParseFailed, "远端 Spark 采集缓冲超过安全读取上限")
	}
	return remoteSparkClaim{path: claimPath, output: result.Stdout}, nil
}

func remoteSparkCollectorConfigDigest(server model.MinecraftServer, tmuxSession, command string, intervalSeconds int) string {
	hash := sha256.New()
	for _, value := range [][]byte{
		remoteSparkCollectorDaemonScript, []byte(server.ID.String()), []byte(server.RemotePath), []byte(tmuxSession), []byte(command), []byte(strconv.Itoa(intervalSeconds)),
	} {
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func parseRemoteSparkBatches(output string) ([]remoteSparkBatch, error) {
	scanner := bufio.NewScanner(bytes.NewBufferString(output))
	scanner.Buffer(make([]byte, 64*1024), remoteSparkMaximumOutput)
	batches := make([]remoteSparkBatch, 0)
	var timestamp time.Time
	var raw strings.Builder
	readingRaw := false
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "spark_batch_start="):
			epoch, err := strconv.ParseInt(strings.TrimPrefix(line, "spark_batch_start="), 10, 64)
			if err != nil {
				return nil, apperror.Wrap(apperror.CodeSparkParseFailed, "解析远端 Spark 采集时间失败", err)
			}
			timestamp = time.Unix(epoch, 0).UTC()
			raw.Reset()
		case line == "spark_raw_begin":
			readingRaw = true
		case line == "spark_raw_end":
			readingRaw = false
		case strings.HasPrefix(line, "spark_batch_end="):
			if timestamp.IsZero() || strings.TrimSpace(raw.String()) == "" {
				return nil, apperror.New(apperror.CodeSparkParseFailed, "远端 Spark 采集批次不完整")
			}
			batches = append(batches, remoteSparkBatch{collectedAt: timestamp, raw: strings.TrimSpace(raw.String())})
			timestamp = time.Time{}
			raw.Reset()
		case readingRaw:
			raw.WriteString(line)
			raw.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, apperror.Wrap(apperror.CodeSparkParseFailed, "读取远端 Spark 采集缓冲失败", err)
	}
	return batches, nil
}

func (m *SparkManager) acknowledgeRemoteSparkClaim(ctx context.Context, client *SSHClient, claimPath string) error {
	if claimPath == "" {
		return nil
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "rm", Arguments: []string{"-f", claimPath}, Timeout: 10 * time.Second, MaximumOutput: 4096}); err != nil {
		return apperror.Wrap(apperror.CodeSparkUnavailable, "确认远端 Spark 采集缓冲失败", err)
	}
	return nil
}

func (m *SparkManager) stopRemoteCollector(ctx context.Context, server model.MinecraftServer) error {
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	dataDirectory := path.Join(server.RemotePath, ".mineops-monitoring")
	pidPath := path.Join(dataDirectory, "spark.pid")
	daemonPath := path.Join(dataDirectory, "spark-collector-daemon.sh")
	configPath := path.Join(dataDirectory, "spark.config")
	stopScript := `set -eu
pid_path=$1
daemon_path=$2
config_path=$3
pid=""
if [ -f "$pid_path" ]; then pid=$(cat "$pid_path" 2>/dev/null || true); fi
if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)
  case "$command_line" in
    *"$daemon_path"*)
      kill "$pid" 2>/dev/null || true
      attempt=0
      while kill -0 "$pid" 2>/dev/null && [ "$attempt" -lt 3 ]; do
        sleep 1
        attempt=$((attempt + 1))
      done ;;
  esac
fi
rm -f "$pid_path" "$config_path"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", stopScript, "mineops", pidPath, daemonPath, configPath}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSparkUnavailable, "停止远端 Spark 采集守护进程失败", err)
	}
	return nil
}

func (m *SparkManager) clearRemoteHistory(ctx context.Context, server model.MinecraftServer) error {
	if err := m.stopRemoteCollector(ctx, server); err != nil {
		return err
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	dataDirectory := path.Join(server.RemotePath, ".mineops-monitoring")
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "rm", Arguments: []string{"-f", path.Join(dataDirectory, "spark.spool"), path.Join(dataDirectory, "spark.claim")}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSparkUnavailable, "清理远端 Spark 采集缓冲失败", err)
	}
	return nil
}

// CollectSnapshot executes only the locked TPS command, parses the exact baseline grammar, persists it, and ingests unified metrics.
func (m *SparkManager) CollectSnapshot(ctx context.Context, serverID model.ID) (model.SparkSnapshot, error) {
	unlock := m.lockServer(serverID)
	defer unlock()
	return m.collectSnapshotLocked(ctx, serverID)
}

func (m *SparkManager) collectSnapshotLocked(ctx context.Context, serverID model.ID) (model.SparkSnapshot, error) {
	if m.collector.IsPaused(serverID) {
		return model.SparkSnapshot{}, apperror.New(apperror.CodeValidationConflict, "该 Server 的监控采集已暂停")
	}
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return model.SparkSnapshot{}, err
	}
	capability, err := m.GetCapability(ctx, serverID)
	if err != nil || capability.Status == enums.SparkUnknown || capability.ParserVersion != minecraftspark.ParserVersion || capability.SourceSchemaHash != minecraftspark.SourceSchemaHash {
		probed, probeErr := m.probeLocked(ctx, serverID)
		if probeErr != nil {
			return model.SparkSnapshot{}, probeErr
		}
		capability = &probed
	}
	if capability.Status != enums.SparkAvailable {
		if capability.Status == enums.SparkUnsupported {
			return model.SparkSnapshot{}, apperror.New(apperror.CodeSparkUnsupported, "Spark 版本不在锁定解析矩阵中")
		}
		message := capability.LastError
		if message == "" {
			message = "Spark 当前不可用；请启动或重启 Minecraft Server 后重新探测"
		}
		return model.SparkSnapshot{}, apperror.New(apperror.CodeSparkUnavailable, message)
	}
	if !minecraftspark.SupportsPluginVersion(capability.PluginVersion) || !capability.TPSSupported {
		return model.SparkSnapshot{}, apperror.New(apperror.CodeSparkUnsupported, "Spark 版本或当前 Server 类型不在锁定解析矩阵中")
	}
	var parsed minecraftspark.Report
	var raw string
	for attempt := 0; attempt < 2; attempt++ {
		raw, err = m.executeCommandLocked(ctx, *server, "tps", 0)
		if err != nil {
			m.recordCapabilityFailure(context.WithoutCancel(ctx), capability, err)
			return model.SparkSnapshot{}, err
		}
		parsed, err = minecraftspark.ParseTPSResponse(minecraftspark.AdapterKey{
			Platform: minecraftspark.PlatformForServerType(server.Type), Distribution: capability.Distribution,
			PluginVersion: capability.PluginVersion, CollectionMethod: minecraftspark.CollectionRCONText,
			ParserVersion: capability.ParserVersion, SourceSchemaHash: capability.SourceSchemaHash,
		}, raw)
		if err == nil {
			break
		}
		if attempt == 0 {
			probed, probeErr := m.probeLocked(ctx, serverID)
			if probeErr == nil && probed.Status == enums.SparkAvailable && probed.TPSSupported {
				capability = &probed
				continue
			}
		}
		rawDigest := sha256.Sum256([]byte(raw))
		failure := apperror.Wrap(apperror.CodeSparkParseFailed, "Spark TPS/MSPT 响应不符合锁定 Grammar", err).WithDetails(map[string]any{
			"rawResponseSHA256": hex.EncodeToString(rawDigest[:]), "rawResponseBytes": len(raw),
		})
		m.recordCapabilityFailure(context.WithoutCancel(ctx), capability, failure)
		return model.SparkSnapshot{}, failure
	}
	return m.persistSnapshot(ctx, *server, capability, parsed, m.clock.Now().UTC())
}

func (m *SparkManager) persistSnapshot(ctx context.Context, server model.MinecraftServer, capability *model.SparkCapability, parsed minecraftspark.Report, collectedAt time.Time) (model.SparkSnapshot, error) {
	id, err := model.NewID(collectedAt)
	if err != nil {
		return model.SparkSnapshot{}, err
	}
	snapshot := model.SparkSnapshot{
		ID: id, ServerID: server.ID, SourceID: id, ServerType: server.Type, Platform: capability.Platform,
		PluginVersion: capability.PluginVersion, ParserVersion: capability.ParserVersion, CollectionMethod: capability.CollectionMethod, SourceSchemaHash: capability.SourceSchemaHash,
		TPS5Seconds: parsed.Snapshot.TPS.Last5Seconds, TPS5SecondsCapped: parsed.Snapshot.TPS.Last5SecondsCapped,
		TPS10Seconds: parsed.Snapshot.TPS.Last10Seconds, TPS10SecondsCapped: parsed.Snapshot.TPS.Last10SecondsCapped,
		TPS1Minute: parsed.Snapshot.TPS.Last1Minute, TPS1MinuteCapped: parsed.Snapshot.TPS.Last1MinuteCapped,
		TPS5Minutes: parsed.Snapshot.TPS.Last5Minutes, TPS5MinutesCapped: parsed.Snapshot.TPS.Last5MinutesCapped,
		TPS15Minutes: parsed.Snapshot.TPS.Last15Minutes, TPS15MinutesCapped: parsed.Snapshot.TPS.Last15MinutesCapped,
		CollectedAt: collectedAt.UTC(), SchemaVersion: model.SparkSchemaVersion,
	}
	if parsed.Snapshot.MSPT != nil {
		snapshot.MSPTAvailable = true
		snapshot.MSPTMinimum = parsed.Snapshot.MSPT.Minimum
		snapshot.MSPTMedian = parsed.Snapshot.MSPT.Median
		snapshot.MSPTP95 = parsed.Snapshot.MSPT.P95
		snapshot.MSPTMaximum = parsed.Snapshot.MSPT.Maximum
	}
	if err := snapshot.Validate(); err != nil {
		return model.SparkSnapshot{}, err
	}
	samples := sparkMetricSamples(snapshot, capability.PluginVersion)
	if _, err := m.metrics.validateSamples(ctx, samples); err != nil {
		return model.SparkSnapshot{}, err
	}
	if err := m.metrics.ensureCapacity(ctx); err != nil {
		return model.SparkSnapshot{}, err
	}
	insert := func() error {
		return m.store.Transaction(ctx, func(registry repository.Registry) error {
			if err := registry.Spark().CreateSnapshot(ctx, &snapshot); err != nil {
				return err
			}
			return registry.Metrics().InsertSamples(ctx, samples)
		})
	}
	if err := insert(); err != nil {
		if !isStorageFullError(err) {
			return model.SparkSnapshot{}, err
		}
		if _, cleanupErr := m.metrics.cleanupRetention(ctx, 100); cleanupErr != nil {
			return model.SparkSnapshot{}, errors.Join(err, cleanupErr)
		}
		if _, cleanupErr := m.metrics.cleanupPressure(ctx); cleanupErr != nil {
			return model.SparkSnapshot{}, errors.Join(err, cleanupErr)
		}
		if retryErr := insert(); retryErr != nil {
			return model.SparkSnapshot{}, apperror.Wrap(apperror.CodeMetricCapacityExceeded, "Spark Snapshot 写入失败，清理后数据库或磁盘容量仍不足", retryErr).WithRetryable(true)
		}
	}
	m.metrics.publishAccepted(samples)
	capability.Status = enums.SparkAvailable
	capability.Installed = true
	capability.PermissionGranted = true
	capability.ReportSupported = true
	capability.RestartRequired = false
	capability.LastErrorCode = ""
	capability.LastError = ""
	capability.DetectedAt = collectedAt.UTC()
	if err := m.store.Spark().SaveCapability(context.WithoutCancel(ctx), capability); err != nil {
		return model.SparkSnapshot{}, err
	}
	return snapshot, nil
}

func sparkMetricSamples(snapshot model.SparkSnapshot, pluginVersion string) []model.MetricSample {
	samples := []model.MetricSample{{
		ServerID: snapshot.ServerID, SourceID: snapshot.ServerID, Metric: enums.MetricMinecraftTPS, Unit: enums.MetricUnitTicksPerSec,
		Timestamp: snapshot.CollectedAt, Value: snapshot.TPS5Seconds,
		Tags: map[string]string{"window": "5s", "pluginVersion": pluginVersion, "capped": strconv.FormatBool(snapshot.TPS5SecondsCapped)}, SchemaVersion: model.MetricSchemaVersion,
	}}
	if snapshot.MSPTAvailable {
		samples = append(samples, model.MetricSample{
			ServerID: snapshot.ServerID, SourceID: snapshot.ServerID, Metric: enums.MetricMinecraftMSPT, Unit: enums.MetricUnitMilliseconds,
			Timestamp: snapshot.CollectedAt, Value: snapshot.MSPTP95, Tags: map[string]string{"quantile": "p95", "pluginVersion": pluginVersion}, SchemaVersion: model.MetricSchemaVersion,
		})
	}
	return samples
}

// StartHealthReport starts one durable privacy-confirmed Spark health upload operation.
func (m *SparkManager) StartHealthReport(ctx context.Context, serverID model.ID, privacyAcknowledged bool) (model.SparkReport, error) {
	if !privacyAcknowledged && m.settings.Snapshot().Monitoring.ReportPrivacyConfirmation {
		return model.SparkReport{}, apperror.New(apperror.CodeValidationConflict, "Spark 外部报告可能包含服务器信息，必须先确认隐私风险")
	}
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return model.SparkReport{}, err
	}
	pluginVersion := minecraftspark.BaselinePluginVersion
	if capability, capabilityErr := m.store.Spark().GetCapability(ctx, serverID); capabilityErr == nil && minecraftspark.SupportsPluginVersion(capability.PluginVersion) {
		pluginVersion = capability.PluginVersion
	}
	report, err := m.newReport(serverID, enums.SparkReportHealth, 0, privacyAcknowledged, pluginVersion)
	if err != nil {
		return model.SparkReport{}, err
	}
	if err := m.store.Spark().CreateReport(ctx, &report); err != nil {
		return model.SparkReport{}, err
	}
	_, err = m.operations.Start(ctx, OperationRequest{
		Type: enums.OperationProfile, TargetType: enums.OperationTargetSpark, TargetID: serverID,
		Prepare: func(id model.ID) error {
			report.OperationID = &id
			report.UpdatedAt = m.clock.Now().UTC()
			return m.store.Spark().UpdateReport(ctx, &report)
		},
		Handler: func(operationContext context.Context, reporter OperationReporter) (executionErr error) {
			unlock := m.lockServer(server.ID)
			defer unlock()
			activeReport, loadErr := m.store.Spark().GetReport(operationContext, report.ID)
			if loadErr != nil {
				return loadErr
			}
			started := m.clock.Now().UTC()
			activeReport.State, activeReport.StartedAt, activeReport.UpdatedAt = enums.SparkReportRunning, &started, started
			_ = m.store.Spark().UpdateReport(context.WithoutCancel(operationContext), activeReport)
			defer func() {
				if executionErr != nil {
					m.failReport(context.WithoutCancel(operationContext), activeReport, executionErr)
				}
			}()
			if err := reporter.SetProgress("spark_health", 0.2, "正在生成并上传 Spark Health Report"); err != nil {
				return err
			}
			raw, err := m.executeCommandLocked(operationContext, *server, "health", 0)
			if err != nil {
				return err
			}
			reportURL, err := officialSparkReportURL(raw)
			if err != nil {
				return err
			}
			finished := m.clock.Now().UTC()
			activeReport.State, activeReport.ReportURL, activeReport.FinishedAt, activeReport.UpdatedAt = enums.SparkReportCompleted, reportURL, &finished, finished
			if err := m.store.Spark().UpdateReport(context.WithoutCancel(operationContext), activeReport); err != nil {
				return err
			}
			return reporter.SetProgress("spark_health", 1, "Spark Health Report 已生成")
		},
	})
	if err != nil {
		m.failReport(context.WithoutCancel(ctx), &report, err)
		return model.SparkReport{}, err
	}
	return report, nil
}

// StartProfiler starts a cancellable explicit-duration profiler operation and records the official viewer reference.
func (m *SparkManager) StartProfiler(ctx context.Context, serverID model.ID, durationSeconds int, privacyAcknowledged bool) (model.SparkReport, error) {
	if durationSeconds < 5 || durationSeconds > 600 {
		return model.SparkReport{}, apperror.New(apperror.CodeValidationInvalidArgument, "Spark Profiler 时长必须在 5 到 600 秒之间")
	}
	if !privacyAcknowledged && m.settings.Snapshot().Monitoring.ReportPrivacyConfirmation {
		return model.SparkReport{}, apperror.New(apperror.CodeValidationConflict, "Spark Profiler 报告可能包含服务器信息，必须先确认隐私风险")
	}
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return model.SparkReport{}, err
	}
	pluginVersion := minecraftspark.BaselinePluginVersion
	if capability, capabilityErr := m.store.Spark().GetCapability(ctx, serverID); capabilityErr == nil && minecraftspark.SupportsPluginVersion(capability.PluginVersion) {
		pluginVersion = capability.PluginVersion
	}
	report, err := m.newReport(serverID, enums.SparkReportProfiler, durationSeconds, privacyAcknowledged, pluginVersion)
	if err != nil {
		return model.SparkReport{}, err
	}
	if err := m.store.Spark().CreateReport(ctx, &report); err != nil {
		return model.SparkReport{}, err
	}
	_, err = m.operations.Start(ctx, OperationRequest{
		Type: enums.OperationProfile, TargetType: enums.OperationTargetSpark, TargetID: serverID,
		Prepare: func(id model.ID) error {
			report.OperationID = &id
			report.UpdatedAt = m.clock.Now().UTC()
			return m.store.Spark().UpdateReport(ctx, &report)
		},
		Handler: func(operationContext context.Context, reporter OperationReporter) (executionErr error) {
			unlock := m.lockServer(server.ID)
			defer unlock()
			activeReport, loadErr := m.store.Spark().GetReport(operationContext, report.ID)
			if loadErr != nil {
				return loadErr
			}
			started := m.clock.Now().UTC()
			activeReport.State, activeReport.StartedAt, activeReport.UpdatedAt = enums.SparkReportRunning, &started, started
			_ = m.store.Spark().UpdateReport(context.WithoutCancel(operationContext), activeReport)
			defer func() {
				if executionErr != nil {
					_, _ = m.executeCommandLocked(context.WithoutCancel(operationContext), *server, "profiler_cancel", 0)
					m.failReport(context.WithoutCancel(operationContext), activeReport, executionErr)
				}
			}()
			if _, err := m.executeCommandLocked(operationContext, *server, "profiler_start", durationSeconds); err != nil {
				return err
			}
			for second := 0; second < durationSeconds; second++ {
				if err := reporter.SetProgress("spark_profiler", float64(second)/float64(durationSeconds+1), fmt.Sprintf("Spark Profiler 采集中：%d/%d 秒", second, durationSeconds)); err != nil {
					return err
				}
				timer := time.NewTimer(time.Second)
				select {
				case <-operationContext.Done():
					if !timer.Stop() {
						<-timer.C
					}
					return operationContext.Err()
				case <-timer.C:
				}
			}
			if err := reporter.SetProgress("spark_profiler", 0.95, "正在停止 Profiler 并生成报告"); err != nil {
				return err
			}
			raw, err := m.executeCommandLocked(operationContext, *server, "profiler_stop", 0)
			if err != nil {
				return err
			}
			reportURL, err := officialSparkReportURL(raw)
			if err != nil {
				return err
			}
			finished := m.clock.Now().UTC()
			activeReport.State, activeReport.ReportURL, activeReport.FinishedAt, activeReport.UpdatedAt = enums.SparkReportCompleted, reportURL, &finished, finished
			if err := m.store.Spark().UpdateReport(context.WithoutCancel(operationContext), activeReport); err != nil {
				return err
			}
			return reporter.SetProgress("spark_profiler", 1, "Spark Profiler Report 已生成")
		},
	})
	if err != nil {
		m.failReport(context.WithoutCancel(ctx), &report, err)
		return model.SparkReport{}, err
	}
	return report, nil
}

// GetCapability returns persisted Spark capability evidence for one Server.
func (m *SparkManager) GetCapability(ctx context.Context, serverID model.ID) (*model.SparkCapability, error) {
	capability, err := m.store.Spark().GetCapability(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if capability.Status != enums.SparkAvailable || (capability.PermissionGranted && !capability.RestartRequired) {
		return capability, nil
	}
	capability.Status = enums.SparkUnavailable
	capability.PermissionGranted = false
	capability.ReportSupported = false
	capability.LastErrorCode = apperror.CodeSparkUnavailable.String()
	if capability.RestartRequired {
		capability.LastError = "Spark Artifact 已安装，必须重启 Minecraft Server 后才能探测和采集。"
	} else {
		capability.LastError = "检测到受支持的 Spark Artifact，但运行时命令尚未通过探测；启动 Server 后请重新探测。"
	}
	if err := m.store.Spark().SaveCapability(ctx, capability); err != nil {
		return nil, err
	}
	return capability, nil
}

// LatestSnapshot returns the latest persisted Spark health observation.
func (m *SparkManager) LatestSnapshot(ctx context.Context, serverID model.ID) (*model.SparkSnapshot, error) {
	return m.store.Spark().LatestSnapshot(ctx, serverID)
}

// ListSnapshots returns bounded Spark snapshot history.
func (m *SparkManager) ListSnapshots(ctx context.Context, query repository.SparkSnapshotQuery) ([]model.SparkSnapshot, error) {
	return m.store.Spark().ListSnapshots(ctx, query)
}

// ListReports returns bounded health and profiler report history.
func (m *SparkManager) ListReports(ctx context.Context, query repository.SparkReportQuery) ([]model.SparkReport, error) {
	return m.store.Spark().ListReports(ctx, query)
}

// DeleteReport 删除一条已结束的 Spark 报告记录，运行中的报告必须先取消或等待结束。
func (m *SparkManager) DeleteReport(ctx context.Context, reportID model.ID) error {
	report, err := m.store.Spark().GetReport(ctx, reportID)
	if err != nil {
		return err
	}
	if report.State == enums.SparkReportPending || report.State == enums.SparkReportRunning {
		return apperror.New(apperror.CodeValidationConflict, "运行中的 Spark Report 无法删除")
	}
	return m.store.Spark().DeleteReport(ctx, reportID)
}

// RecoverInterruptedReports reconciles Spark Report state after Operation recovery on application startup.
func (m *SparkManager) RecoverInterruptedReports(ctx context.Context) error {
	reports, err := m.store.Spark().ListReports(ctx, repository.SparkReportQuery{Limit: 1000})
	if err != nil {
		return err
	}
	for index := range reports {
		report := &reports[index]
		if report.State != enums.SparkReportPending && report.State != enums.SparkReportRunning {
			continue
		}
		report.State = enums.SparkReportFailed
		report.ErrorCode = apperror.CodeProcessExitFailed.String()
		report.ErrorMessage = "应用异常退出，Spark Report 已中断"
		if report.OperationID != nil {
			if operation, operationErr := m.store.Operations().Get(ctx, *report.OperationID); operationErr == nil {
				switch operation.State {
				case enums.OperationCancelled:
					report.State = enums.SparkReportCancelled
					report.ErrorCode = apperror.CodeProcessCancelled.String()
				case enums.OperationFailed:
					report.ErrorCode = operation.ErrorCode
				}
				if operation.Message != "" {
					report.ErrorMessage = operation.Message
				}
			}
		}
		now := m.clock.Now().UTC()
		report.FinishedAt = &now
		report.UpdatedAt = now
		if err := m.store.Spark().UpdateReport(ctx, report); err != nil {
			return err
		}
	}
	return nil
}

func (m *SparkManager) executeCommandLocked(ctx context.Context, server model.MinecraftServer, query string, durationSeconds int) (string, error) {
	if err := m.stopRemoteCollector(ctx, server); err != nil {
		return "", err
	}
	identity, err := m.store.ProcessIdentities().GetByServer(ctx, server.ID)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeSparkUnavailable, "Spark Query 需要 MineOps 管理的运行中 Server Console", err)
	}
	probe, err := m.processes.Probe(ctx, *identity)
	if err != nil || probe.Identity.State != enums.RemoteProcessRunning {
		return "", apperror.New(apperror.CodeSparkUnavailable, "Spark Query 需要运行中的 Minecraft Server")
	}
	command, err := sparkAllowedCommand(server.Type, query, durationSeconds)
	if err != nil {
		return "", err
	}
	_, client, err := m.processes.connect(ctx, identity.SSHSessionID)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()
	if identity.TmuxSession != "" {
		return m.executeTmuxCommand(ctx, client, *identity, command, query)
	}
	sizeResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "wc", Arguments: []string{"-c", identity.ConsoleLog}, Timeout: 5 * time.Second, MaximumOutput: 4096})
	if err != nil {
		return "", apperror.Wrap(apperror.CodeSparkUnavailable, "读取 Spark Console 起始位置失败", err)
	}
	sizeFields := strings.Fields(sizeResult.Stdout)
	if len(sizeFields) == 0 {
		return "", apperror.New(apperror.CodeSparkParseFailed, "Spark Console 起始位置响应为空")
	}
	offset, err := strconv.ParseInt(sizeFields[0], 10, 64)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeSparkParseFailed, "解析 Spark Console 起始位置失败", err)
	}
	if !path.IsAbs(identity.ConsoleFIFO) {
		return "", apperror.New(apperror.CodeSparkUnavailable, "旧版 Spark Console 缺少受控 FIFO 路径")
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `printf '%s\n' "$1" > "$2"`, "mineops-spark", command, identity.ConsoleFIFO},
		Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return "", apperror.Wrap(apperror.CodeSparkUnavailable, "写入受控 Spark Command 失败", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	lastSize := 0
	lastChange := time.Now()
	for time.Now().Before(deadline) {
		result, readErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "tail", Arguments: []string{"-c", "+" + strconv.FormatInt(offset+1, 10), identity.ConsoleLog},
			Timeout: 5 * time.Second, MaximumOutput: 128 * 1024,
		})
		if readErr == nil {
			if result.Truncated {
				return "", apperror.New(apperror.CodeSparkParseFailed, "Spark Console 响应超过安全读取上限").WithRetryable(true)
			}
			if len(result.Stdout) != lastSize {
				lastSize = len(result.Stdout)
				lastChange = time.Now()
			}
			if result.Stdout != "" && time.Since(lastChange) >= 750*time.Millisecond {
				if responseErr := validateSparkCommandResponse(result.Stdout); responseErr != nil {
					return "", responseErr
				}
				if sparkCommandResponseComplete(query, result.Stdout) {
					return result.Stdout, nil
				}
			}
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", apperror.New(apperror.CodeSparkUnavailable, "Spark Command 响应超时").WithRetryable(true)
}

func (m *SparkManager) executeTmuxCommand(ctx context.Context, client *SSHClient, identity model.RemoteProcessIdentity, command, query string) (string, error) {
	baseline, err := m.processes.captureTmuxPane(ctx, client, identity.TmuxSession)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeSparkUnavailable, "读取 Spark Console 起始内容失败", err)
	}
	if err := m.processes.sendTmuxLine(ctx, client, identity.TmuxSession, command); err != nil {
		return "", apperror.Wrap(apperror.CodeSparkUnavailable, "写入受控 Spark Command 失败", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	lastResponse := ""
	lastChange := time.Now()
	for time.Now().Before(deadline) {
		captured, captureErr := m.processes.captureTmuxPane(ctx, client, identity.TmuxSession)
		if captureErr == nil {
			response, overlapFound := tmuxOutputAfterBaseline(baseline, captured)
			if overlapFound {
				response = strings.TrimSpace(response)
				if response != lastResponse {
					lastResponse = response
					lastChange = time.Now()
				}
				if response != "" && time.Since(lastChange) >= 750*time.Millisecond {
					if responseErr := validateSparkCommandResponse(response); responseErr != nil {
						return "", responseErr
					}
					if sparkCommandResponseComplete(query, response) {
						return response, nil
					}
				}
			}
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", apperror.New(apperror.CodeSparkUnavailable, "Spark Command 响应超时").WithRetryable(true)
}

func tmuxOutputAfterBaseline(baseline, captured string) (string, bool) {
	baseline = strings.TrimRight(baseline, "\r\n")
	captured = strings.TrimRight(captured, "\r\n")
	if baseline == "" {
		return captured, true
	}
	if captured == "" {
		return "", false
	}
	baselineLines := strings.Split(baseline, "\n")
	capturedLines := strings.Split(captured, "\n")
	bestOverlap := 0
	for baselineStart, line := range baselineLines {
		if line != capturedLines[0] {
			continue
		}
		overlap := 0
		for baselineStart+overlap < len(baselineLines) && overlap < len(capturedLines) && baselineLines[baselineStart+overlap] == capturedLines[overlap] {
			overlap++
		}
		if baselineStart+overlap < len(baselineLines)-1 || overlap <= bestOverlap {
			continue
		}
		bestOverlap = overlap
	}
	if bestOverlap == 0 {
		return "", false
	}
	return strings.Join(capturedLines[bestOverlap:], "\n"), true
}

func validateSparkCommandResponse(raw string) error {
	normalized := strings.ToLower(raw)
	if !strings.Contains(normalized, "unknown or incomplete command") && !strings.Contains(normalized, "unknown command") {
		return nil
	}
	rawDigest := sha256.Sum256([]byte(raw))
	return apperror.New(apperror.CodeSparkUnavailable, "Spark Command 尚未注册；安装或升级后请重启 Minecraft Server。").WithDetails(map[string]any{
		"reason": "command_unregistered", "rawResponseSHA256": hex.EncodeToString(rawDigest[:]), "rawResponseBytes": len(raw),
	})
}

func sparkCommandResponseComplete(query, raw string) bool {
	switch query {
	case "version":
		return strings.Contains(strings.ToLower(raw), "spark") && detectSparkVersion(raw) != "unknown"
	case "tps", "mspt":
		return strings.Contains(raw, "TPS from last 5s, 10s, 1m, 5m, 15m:")
	case "health", "profiler_stop":
		_, err := officialSparkReportURL(raw)
		return err == nil
	case "profiler_start", "profiler_cancel":
		return strings.TrimSpace(raw) != ""
	default:
		return strings.TrimSpace(raw) != ""
	}
}

func (m *SparkManager) connect(ctx context.Context, serverID model.ID) (*model.MinecraftServer, *model.SSHSession, *SSHClient, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, nil, nil, err
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return nil, nil, nil, err
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return nil, nil, nil, err
	}
	return server, session, client, nil
}

func (m *SparkManager) detectArtifact(ctx context.Context, server model.MinecraftServer, targetKind string) (string, string, error) {
	_, _, client, err := m.connect(ctx, server.ID)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = client.Close() }()
	targetDirectory := path.Join(server.RemotePath, targetKind)
	result, commandErr := client.RunCommand(ctx, RemoteCommand{
		Executable: "find", Arguments: []string{targetDirectory, "-maxdepth", "1", "-type", "f", "-iname", "spark*.jar", "-print", "-quit"},
		Timeout: 10 * time.Second, MaximumOutput: 4096,
	})
	if commandErr != nil && strings.TrimSpace(result.Stdout) == "" {
		return "", "", nil
	}
	artifactPath := strings.TrimSpace(result.Stdout)
	if artifactPath == "" {
		return "", "", nil
	}
	return artifactPath, detectSparkVersion(path.Base(artifactPath)), nil
}

func (m *SparkManager) newReport(serverID model.ID, kind enums.SparkReportKind, durationSeconds int, privacyAcknowledged bool, pluginVersion string) (model.SparkReport, error) {
	now := m.clock.Now().UTC()
	id, err := model.NewID(now)
	if err != nil {
		return model.SparkReport{}, err
	}
	return model.SparkReport{
		ID: id, ServerID: serverID, Kind: kind, State: enums.SparkReportPending, DurationSeconds: durationSeconds,
		PrivacyAcknowledged: privacyAcknowledged, PluginVersion: pluginVersion,
		ParserVersion: minecraftspark.ParserVersion, CreatedAt: now, UpdatedAt: now, SchemaVersion: model.SparkSchemaVersion,
	}, nil
}

func (m *SparkManager) failReport(ctx context.Context, report *model.SparkReport, failure error) {
	now := m.clock.Now().UTC()
	dto := apperror.ToDTO(failure)
	report.State = enums.SparkReportFailed
	if errors.Is(failure, context.Canceled) {
		report.State = enums.SparkReportCancelled
	}
	report.ErrorCode, report.ErrorMessage = dto.Code, dto.Message
	report.FinishedAt, report.UpdatedAt = &now, now
	if err := m.store.Spark().UpdateReport(ctx, report); err != nil {
		m.logger.Error(ctx, "更新 Spark Report 失败状态失败", err, applog.Fields{"report_id": report.ID.String()})
	}
}

func (m *SparkManager) recordCapabilityFailure(ctx context.Context, capability *model.SparkCapability, failure error) {
	applySparkCollectionFailure(capability, apperror.ToDTO(failure), m.clock.Now().UTC())
	if err := m.store.Spark().SaveCapability(ctx, capability); err != nil {
		m.logger.Error(ctx, "持久化 Spark 失败状态失败", err, applog.Fields{"server_id": capability.ServerID.String()})
	}
}

// applySparkCollectionFailure 依据失败性质更新采集能力状态:
// 瞬时可重试失败(命令超时、临时无响应)不降级已验证的 Available 能力,否则周期采集的偶发失败会让性能页
// 反复回到"需要重新探测";仅记录最近错误并保留 Status/PermissionGranted 供下次采集重试。
// 确定性失败(命令未注册需重启、解析 Grammar 不符=版本不兼容)才降级为 Unavailable/Failed。
func applySparkCollectionFailure(capability *model.SparkCapability, dto apperror.DTO, now time.Time) {
	capability.LastErrorCode, capability.LastError = dto.Code, dto.Message
	capability.DetectedAt = now
	if dto.Retryable && capability.Status == enums.SparkAvailable {
		return
	}
	capability.Status = enums.SparkFailed
	if dto.Code == apperror.CodeSparkUnavailable.String() {
		capability.Status = enums.SparkUnavailable
		capability.PermissionGranted = false
		capability.ReportSupported = false
		if reason, _ := dto.Details["reason"].(string); reason == "command_unregistered" {
			capability.RestartRequired = true
		}
	}
}

func sparkAllowedCommand(serverType enums.MinecraftServerType, query string, durationSeconds int) (string, error) {
	prefix := "spark"
	switch serverType {
	case enums.ServerBungee, enums.ServerWaterfall:
		prefix = "sparkb"
	case enums.ServerVelocity:
		prefix = "sparkv"
	}
	switch query {
	case "tps", "mspt":
		return prefix + " tps", nil
	case "health":
		return prefix + " health --upload", nil
	case "version":
		return prefix, nil
	case "profiler_start":
		if durationSeconds < 5 || durationSeconds > 600 {
			return "", apperror.New(apperror.CodeValidationInvalidArgument, "Spark Profiler 时长无效")
		}
		return prefix + " profiler start --timeout " + strconv.Itoa(durationSeconds+30), nil
	case "profiler_stop":
		return prefix + " profiler stop", nil
	case "profiler_cancel":
		return prefix + " profiler cancel", nil
	default:
		return "", apperror.New(apperror.CodeSparkUnsupported, "Spark Command 不在版本化允许列表中")
	}
}

func sparkMetricCapabilities(serverType enums.MinecraftServerType) (bool, bool) {
	switch serverType {
	case enums.ServerPaper, enums.ServerPurpur, enums.ServerFabric, enums.ServerForge, enums.ServerNeoForge:
		return true, true
	case enums.ServerSpigot:
		return true, false
	default:
		return false, false
	}
}

func detectSparkVersion(raw string) string {
	for _, pattern := range []*regexp.Regexp{sparkConsoleVersionPattern, sparkArtifactVersionPattern} {
		match := pattern.FindStringSubmatch(raw)
		if len(match) == 2 {
			return match[1]
		}
	}
	return "unknown"
}

func officialSparkReportURL(raw string) (string, error) {
	marker := "https://spark.lucko.me/"
	for offset := 0; ; {
		index := strings.Index(raw[offset:], marker)
		if index < 0 {
			break
		}
		start := offset + index
		end := start
		for end < len(raw) && raw[end] > ' ' && !strings.ContainsRune("<>\"'()[]{}", rune(raw[end])) {
			end++
		}
		candidate := strings.TrimRight(raw[start:end], ".,;:!§")
		parsed, err := url.Parse(candidate)
		if err == nil && isOfficialSparkViewerURL(parsed) {
			return parsed.String(), nil
		}
		offset = start + len(marker)
	}
	return "", apperror.New(apperror.CodeSparkParseFailed, "Spark 响应中没有官方 Viewer URL")
}

func isOfficialSparkViewerURL(parsed *url.URL) bool {
	if parsed == nil || parsed.Scheme != "https" || parsed.Hostname() != "spark.lucko.me" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 1 || segments[0] == "" {
		return false
	}
	switch strings.ToLower(segments[0]) {
	case "docs", "download", "source", "github", "discord", "status", "privacy", "terms":
		return false
	default:
		return true
	}
}

func parseKeyValueLines(raw string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found {
			values[key] = value
		}
	}
	return values
}

func safeSparkPath(root, candidate string) bool {
	root = path.Clean(root)
	candidate = path.Clean(candidate)
	return path.IsAbs(root) && path.IsAbs(candidate) && candidate != root && strings.HasPrefix(candidate, root+"/")
}

// uploadRemoteFile 以 0600 权限将内容写入远程受控路径。
func uploadRemoteFile(ctx context.Context, client *SSHClient, destination string, payload []byte) error {
	_, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `umask 077; cat > "$1"`, "mineops-upload", destination},
		Input: payload, Timeout: 2 * time.Minute, MaximumOutput: 4096,
	})
	return err
}
