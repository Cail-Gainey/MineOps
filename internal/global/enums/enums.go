// Package enums 存放 MineOps 各层共用、带校验的强类型字符串枚举。
package enums

// OperationState 标识一个长任务的持久化生命周期。
type OperationState string

// OperationType 标识一个持久化任务所执行的用户意图。
type OperationType string

// OperationTargetType 标识任务所锁定的资源类别。
type OperationTargetType string

// SSHAuthType 标识 SSH Session 使用的凭据机制。
type SSHAuthType string

// SSHHostKeyPolicy 标识 SSH Session 的主机密钥校验策略。
type SSHHostKeyPolicy string

// MinecraftServerType 标识一个受支持的服务端发行版或代理端家族。
type MinecraftServerType string

// FirewallPolicy 标识 MineOps 如何处理已配置的 Minecraft TCP 端口。
type FirewallPolicy string

// InstallationState 标识一个安装任务的持久化生命周期。
type InstallationState string

// InstallationStepState 标识一个带检查点的安装步骤的生命周期。
type InstallationStepState string

// ProxyMode 标识出站目录与构件 HTTP 流量的路由方式。
type ProxyMode string

// RemoteProcessState 标识探测得到的进程证据,与持久化的 Server 生命周期状态相互独立。
type RemoteProcessState string

// FirewallBackend 标识探测到的远端 Linux 防火墙实现。
type FirewallBackend string

// MetricUnit 标识一个指标的物理单位或语义单位。
type MetricUnit string

// MetricValueType 标识一个指标值应如何渲染与校验。
type MetricValueType string

// MetricAggregation 标识降采样时首选的聚合算法。
type MetricAggregation string

// MetricGranularity 标识原始、分钟或小时级的存储与查询粒度。
type MetricGranularity string

// SparkReportKind 标识一种受控的 Minecraft spark 报告流程。
type SparkReportKind string

// SparkReportState 标识一份 Minecraft spark 报告的持久化生命周期。
type SparkReportState string

// AlertComparison 标识告警规则使用的有界数值比较方式。
type AlertComparison string

// AlertEventState 标识一次阈值告警处于活跃还是已恢复。
type AlertEventState string

// PlayerActivityEventType 标识一条采集到的玩家在线状态事件。
type PlayerActivityEventType string

// PlayerSessionState 标识玩家连接仍在进行还是已结算。
type PlayerSessionState string

// PlayerSessionCloseReason 标识玩家会话结算的原因。
type PlayerSessionCloseReason string

// PlayerIdentityKind 标识支撑某台 Server 内玩家身份的证据来源。
type PlayerIdentityKind string

// PlayerActivityAccuracy 标识已持久化玩家活动数据的可信程度。
type PlayerActivityAccuracy string

const (
	OperationPending   OperationState = "pending"
	OperationRunning   OperationState = "running"
	OperationSucceeded OperationState = "succeeded"
	OperationFailed    OperationState = "failed"
	OperationCancelled OperationState = "cancelled"

	OperationInstall  OperationType = "install"
	OperationDownload OperationType = "download"
	OperationUpload   OperationType = "upload"
	OperationExtract  OperationType = "extract"
	OperationStart    OperationType = "start"
	OperationStop     OperationType = "stop"
	OperationRestart  OperationType = "restart"
	OperationBackup   OperationType = "backup"
	OperationRestore  OperationType = "restore"
	OperationUpdate   OperationType = "update"
	OperationDelete   OperationType = "delete"
	OperationProfile  OperationType = "profile"

	OperationTargetServer     OperationTargetType = "server"
	OperationTargetSSHSession OperationTargetType = "ssh_session"
	OperationTargetFile       OperationTargetType = "file"
	OperationTargetJava       OperationTargetType = "java_runtime"
	OperationTargetAgent      OperationTargetType = "agent"
	OperationTargetSettings   OperationTargetType = "settings"
	OperationTargetSpark      OperationTargetType = "spark"

	SSHAuthPassword   SSHAuthType = "password"
	SSHAuthPrivateKey SSHAuthType = "private_key"
	SSHAuthAgent      SSHAuthType = "agent"

	SSHHostKeyStrict       SSHHostKeyPolicy = "strict"
	SSHHostKeyTrustOnFirst SSHHostKeyPolicy = "trust_on_first_use"

	ServerVanilla   MinecraftServerType = "vanilla"
	ServerPaper     MinecraftServerType = "paper"
	ServerPurpur    MinecraftServerType = "purpur"
	ServerSpigot    MinecraftServerType = "spigot"
	ServerFabric    MinecraftServerType = "fabric"
	ServerForge     MinecraftServerType = "forge"
	ServerNeoForge  MinecraftServerType = "neoforge"
	ServerQuilt     MinecraftServerType = "quilt"
	ServerFolia     MinecraftServerType = "folia"
	ServerVelocity  MinecraftServerType = "velocity"
	ServerWaterfall MinecraftServerType = "waterfall"
	ServerBungee    MinecraftServerType = "bungeecord"

	FirewallDisabled  FirewallPolicy = "disabled"
	FirewallPrompt    FirewallPolicy = "prompt"
	FirewallAutomatic FirewallPolicy = "automatic"

	InstallationWaiting   InstallationState = "waiting"
	InstallationRunning   InstallationState = "running"
	InstallationSucceeded InstallationState = "succeeded"
	InstallationFailed    InstallationState = "failed"
	InstallationCancelled InstallationState = "cancelled"

	InstallationStepWaiting   InstallationStepState = "waiting"
	InstallationStepRunning   InstallationStepState = "running"
	InstallationStepSuccess   InstallationStepState = "success"
	InstallationStepFailed    InstallationStepState = "failed"
	InstallationStepSkipped   InstallationStepState = "skipped"
	InstallationStepCancelled InstallationStepState = "cancelled"

	ProxyNone   ProxyMode = "none"
	ProxySystem ProxyMode = "system"
	ProxyHTTP   ProxyMode = "http"
	ProxyHTTPS  ProxyMode = "https"
	ProxySOCKS5 ProxyMode = "socks5"

	RemoteProcessUnknown    RemoteProcessState = "unknown"
	RemoteProcessRunning    RemoteProcessState = "running"
	RemoteProcessExited     RemoteProcessState = "exited"
	RemoteProcessMismatched RemoteProcessState = "mismatched"

	FirewallBackendNone      FirewallBackend = "none"
	FirewallBackendUFW       FirewallBackend = "ufw"
	FirewallBackendFirewalld FirewallBackend = "firewalld"

	MetricUnitPercent      MetricUnit = "percent"
	MetricUnitBytes        MetricUnit = "bytes"
	MetricUnitBytesPerSec  MetricUnit = "bytes_per_second"
	MetricUnitCount        MetricUnit = "count"
	MetricUnitMilliseconds MetricUnit = "milliseconds"
	MetricUnitTicksPerSec  MetricUnit = "ticks_per_second"
	MetricUnitRatio        MetricUnit = "ratio"
	MetricUnitSeconds      MetricUnit = "seconds"

	MetricValueGauge   MetricValueType = "gauge"
	MetricValueCounter MetricValueType = "counter"

	MetricAggregationAverage MetricAggregation = "average"
	MetricAggregationSum     MetricAggregation = "sum"
	MetricAggregationLatest  MetricAggregation = "latest"
	MetricAggregationMaximum MetricAggregation = "maximum"

	MetricGranularityRaw    MetricGranularity = "raw"
	MetricGranularityMinute MetricGranularity = "minute"
	MetricGranularityHour   MetricGranularity = "hour"

	SparkReportHealth   SparkReportKind = "health"
	SparkReportProfiler SparkReportKind = "profiler"

	SparkReportPending   SparkReportState = "pending"
	SparkReportRunning   SparkReportState = "running"
	SparkReportCompleted SparkReportState = "completed"
	SparkReportFailed    SparkReportState = "failed"
	SparkReportCancelled SparkReportState = "cancelled"

	AlertGreaterThan        AlertComparison = "greater_than"
	AlertGreaterThanOrEqual AlertComparison = "greater_than_or_equal"
	AlertLessThan           AlertComparison = "less_than"
	AlertLessThanOrEqual    AlertComparison = "less_than_or_equal"

	AlertEventActive    AlertEventState = "active"
	AlertEventRecovered AlertEventState = "recovered"

	PlayerActivityJoin  PlayerActivityEventType = "join"
	PlayerActivityLeave PlayerActivityEventType = "leave"

	PlayerSessionOpen        PlayerSessionState = "open"
	PlayerSessionClosed      PlayerSessionState = "closed"
	PlayerSessionInterrupted PlayerSessionState = "interrupted"

	PlayerCloseLeave           PlayerSessionCloseReason = "leave"
	PlayerCloseDuplicateJoin   PlayerSessionCloseReason = "duplicate_join"
	PlayerCloseServerStopped   PlayerSessionCloseReason = "server_stopped"
	PlayerCloseUnexpectedExit  PlayerSessionCloseReason = "unexpected_exit"
	PlayerCloseProcessReplaced PlayerSessionCloseReason = "process_replaced"
	PlayerCloseIdentityMerged  PlayerSessionCloseReason = "identity_merged"

	PlayerIdentityUUID     PlayerIdentityKind = "uuid"
	PlayerIdentityNameOnly PlayerIdentityKind = "name_only"

	PlayerAccuracyExact          PlayerActivityAccuracy = "exact"
	PlayerAccuracyServerBoundary PlayerActivityAccuracy = "server_boundary"
	PlayerAccuracyEstimated      PlayerActivityAccuracy = "estimated"
	PlayerAccuracyReconstructed  PlayerActivityAccuracy = "reconstructed"
	PlayerAccuracyIncomplete     PlayerActivityAccuracy = "incomplete"
)

// LifecycleState 标识经过校验的 Minecraft 服务端生命周期。
type LifecycleState string

const (
	LifecycleCreating   LifecycleState = "creating"
	LifecycleInstalling LifecycleState = "installing"
	LifecycleReady      LifecycleState = "ready"
	LifecycleStarting   LifecycleState = "starting"
	LifecycleRunning    LifecycleState = "running"
	LifecycleStopping   LifecycleState = "stopping"
	LifecycleStopped    LifecycleState = "stopped"
	LifecycleUpdating   LifecycleState = "updating"
	LifecycleBackingUp  LifecycleState = "backing_up"
	LifecycleDeleted    LifecycleState = "deleted"
	LifecycleFailed     LifecycleState = "failed"
)

// MetricType 标识一条持久化指标样本的命名空间。
type MetricType string

const (
	MetricHostCPU             MetricType = "host.cpu"
	MetricHostMemory          MetricType = "host.memory"
	MetricMinecraftTPS        MetricType = "minecraft.tps"
	MetricMinecraftMSPT       MetricType = "minecraft.mspt"
	MetricHostLoad1           MetricType = "host.load.1m"
	MetricHostMemoryUsed      MetricType = "host.memory.used"
	MetricHostMemoryTotal     MetricType = "host.memory.total"
	MetricHostSwapUsed        MetricType = "host.swap.used"
	MetricHostDiskUsed        MetricType = "host.disk.used"
	MetricHostDiskTotal       MetricType = "host.disk.total"
	MetricHostDiskRead        MetricType = "host.disk.read_bytes_per_second"
	MetricHostDiskWrite       MetricType = "host.disk.write_bytes_per_second"
	MetricHostNetworkReceive  MetricType = "host.network.receive_bytes_per_second"
	MetricHostNetworkTransmit MetricType = "host.network.transmit_bytes_per_second"
	MetricProcessCPU          MetricType = "process.cpu"
	MetricProcessRSS          MetricType = "process.rss"
	MetricProcessThreads      MetricType = "process.threads"
	MetricProcessFDs          MetricType = "process.file_descriptors"
	MetricProcessUptime       MetricType = "process.uptime"
	MetricProcessRunning      MetricType = "process.running"
	MetricMinecraftPlayers    MetricType = "minecraft.players"
)

// SparkStatus 标识 Minecraft spark 的可用性,与 Agent 状态相互独立。
type SparkStatus string

// SettingsCategory 标识一个可独立重置的设置分区。
type SettingsCategory string

const (
	SparkUnknown     SparkStatus = "unknown"
	SparkUnavailable SparkStatus = "unavailable"
	SparkUnsupported SparkStatus = "unsupported"
	SparkAvailable   SparkStatus = "available"
	SparkCollecting  SparkStatus = "collecting"
	SparkFailed      SparkStatus = "failed"

	SettingsGeneral    SettingsCategory = "general"
	SettingsTheme      SettingsCategory = "theme"
	SettingsPaths      SettingsCategory = "paths"
	SettingsMirrors    SettingsCategory = "mirrors"
	SettingsLogging    SettingsCategory = "logging"
	SettingsMonitoring SettingsCategory = "monitoring"
	SettingsFirewall   SettingsCategory = "firewall"
	SettingsLayout     SettingsCategory = "layout"
	SettingsSSH        SettingsCategory = "ssh"
	SettingsTerminal   SettingsCategory = "terminal"
	SettingsDownloads  SettingsCategory = "downloads"
	SettingsStorage    SettingsCategory = "storage"
)

// String 返回序列化后的Operation 状态。
func (v OperationState) String() string { return string(v) }

// Valid 返回该Operation 状态是否为已注册值。
func (v OperationState) Valid() bool {
	return contains(v, OperationPending, OperationRunning, OperationSucceeded, OperationFailed, OperationCancelled)
}

// String 返回序列化后的Operation 类型。
func (v OperationType) String() string { return string(v) }

// Valid 返回该Operation 类型是否为已注册值。
func (v OperationType) Valid() bool {
	return contains(v, OperationInstall, OperationDownload, OperationUpload, OperationExtract, OperationStart, OperationStop, OperationRestart, OperationBackup, OperationRestore, OperationUpdate, OperationDelete, OperationProfile)
}

// String 返回序列化后的Operation 目标类型。
func (v OperationTargetType) String() string { return string(v) }

// Valid 返回该Operation 目标类型是否为已注册值。
func (v OperationTargetType) Valid() bool {
	return contains(v, OperationTargetServer, OperationTargetSSHSession, OperationTargetFile, OperationTargetJava, OperationTargetAgent, OperationTargetSettings, OperationTargetSpark)
}

// String 返回序列化后的SSH 认证方式。
func (v SSHAuthType) String() string { return string(v) }

// Valid 返回该SSH 认证方式是否为已注册值。
func (v SSHAuthType) Valid() bool {
	return contains(v, SSHAuthPassword, SSHAuthPrivateKey, SSHAuthAgent)
}

// String 返回序列化后的SSH 主机密钥策略。
func (v SSHHostKeyPolicy) String() string { return string(v) }

// Valid 返回该SSH 主机密钥策略是否为已注册值。
func (v SSHHostKeyPolicy) Valid() bool {
	return contains(v, SSHHostKeyStrict, SSHHostKeyTrustOnFirst)
}

// String 返回序列化后的Minecraft 服务端类型。
func (v MinecraftServerType) String() string { return string(v) }

// Valid 返回该Minecraft 服务端类型是否为已注册值。
func (v MinecraftServerType) Valid() bool {
	return contains(v, ServerVanilla, ServerPaper, ServerPurpur, ServerSpigot, ServerFabric, ServerForge, ServerNeoForge, ServerQuilt, ServerFolia, ServerVelocity, ServerWaterfall, ServerBungee)
}

// String 返回序列化后的防火墙策略。
func (v FirewallPolicy) String() string { return string(v) }

// Valid 返回该防火墙策略是否为已注册值。
func (v FirewallPolicy) Valid() bool {
	return contains(v, FirewallDisabled, FirewallPrompt, FirewallAutomatic)
}

// String 返回序列化后的安装任务状态。
func (v InstallationState) String() string { return string(v) }

// Valid 返回该安装任务状态是否为已注册值。
func (v InstallationState) Valid() bool {
	return contains(v, InstallationWaiting, InstallationRunning, InstallationSucceeded, InstallationFailed, InstallationCancelled)
}

// String 返回序列化后的安装步骤状态。
func (v InstallationStepState) String() string { return string(v) }

// Valid 返回该安装步骤状态是否为已注册值。
func (v InstallationStepState) Valid() bool {
	return contains(v, InstallationStepWaiting, InstallationStepRunning, InstallationStepSuccess, InstallationStepFailed, InstallationStepSkipped, InstallationStepCancelled)
}

// String 返回序列化后的代理模式。
func (v ProxyMode) String() string { return string(v) }

// Valid 返回该代理模式是否为已注册值。
func (v ProxyMode) Valid() bool {
	return contains(v, ProxyNone, ProxySystem, ProxyHTTP, ProxyHTTPS, ProxySOCKS5)
}

// String 返回序列化后的远端进程状态。
func (v RemoteProcessState) String() string { return string(v) }

// Valid 返回该远端进程状态是否为已注册值。
func (v RemoteProcessState) Valid() bool {
	return contains(v, RemoteProcessUnknown, RemoteProcessRunning, RemoteProcessExited, RemoteProcessMismatched)
}

// String 返回序列化后的防火墙后端。
func (v FirewallBackend) String() string { return string(v) }

// Valid 返回该防火墙后端是否为已注册值。
func (v FirewallBackend) Valid() bool {
	return contains(v, FirewallBackendNone, FirewallBackendUFW, FirewallBackendFirewalld)
}

// String 返回序列化后的指标单位。
func (v MetricUnit) String() string { return string(v) }

// Valid 返回该指标单位是否为已注册值。
func (v MetricUnit) Valid() bool {
	return contains(v, MetricUnitPercent, MetricUnitBytes, MetricUnitBytesPerSec, MetricUnitCount, MetricUnitMilliseconds, MetricUnitTicksPerSec, MetricUnitRatio, MetricUnitSeconds)
}

// String 返回序列化后的指标值类型。
func (v MetricValueType) String() string { return string(v) }

// Valid 返回该指标值类型是否为已注册值。
func (v MetricValueType) Valid() bool { return contains(v, MetricValueGauge, MetricValueCounter) }

// String 返回序列化后的指标聚合方式。
func (v MetricAggregation) String() string { return string(v) }

// Valid 返回该指标聚合方式是否为已注册值。
func (v MetricAggregation) Valid() bool {
	return contains(v, MetricAggregationAverage, MetricAggregationSum, MetricAggregationLatest, MetricAggregationMaximum)
}

// String 返回序列化后的指标粒度。
func (v MetricGranularity) String() string { return string(v) }

// Valid 返回该指标粒度是否为已注册值。
func (v MetricGranularity) Valid() bool {
	return contains(v, MetricGranularityRaw, MetricGranularityMinute, MetricGranularityHour)
}

// String 返回序列化后的Minecraft spark 报告类型。
func (v SparkReportKind) String() string { return string(v) }

// Valid 返回该Minecraft spark 报告类型是否为已注册值。
func (v SparkReportKind) Valid() bool { return contains(v, SparkReportHealth, SparkReportProfiler) }

// String 返回序列化后的Minecraft spark 报告状态。
func (v SparkReportState) String() string { return string(v) }

// Valid 返回该Minecraft spark 报告状态是否为已注册值。
func (v SparkReportState) Valid() bool {
	return contains(v, SparkReportPending, SparkReportRunning, SparkReportCompleted, SparkReportFailed, SparkReportCancelled)
}

// String 返回序列化后的告警比较方式。
func (v AlertComparison) String() string { return string(v) }

// Valid 返回该告警比较方式是否为已注册值。
func (v AlertComparison) Valid() bool {
	return contains(v, AlertGreaterThan, AlertGreaterThanOrEqual, AlertLessThan, AlertLessThanOrEqual)
}

// Match 执行已注册的数值比较。
func (v AlertComparison) Match(value, threshold float64) bool {
	switch v {
	case AlertGreaterThan:
		return value > threshold
	case AlertGreaterThanOrEqual:
		return value >= threshold
	case AlertLessThan:
		return value < threshold
	case AlertLessThanOrEqual:
		return value <= threshold
	default:
		return false
	}
}

// String 返回序列化后的告警事件状态。
func (v AlertEventState) String() string { return string(v) }

// Valid 返回该告警事件状态是否为已注册值。
func (v AlertEventState) Valid() bool { return contains(v, AlertEventActive, AlertEventRecovered) }

// String 返回序列化后的玩家活动事件类型。
func (v PlayerActivityEventType) String() string { return string(v) }

// Valid 返回该玩家活动事件类型是否为已注册值。
func (v PlayerActivityEventType) Valid() bool {
	return contains(v, PlayerActivityJoin, PlayerActivityLeave)
}

// String 返回序列化后的玩家会话状态。
func (v PlayerSessionState) String() string { return string(v) }

// Valid 返回该玩家会话状态是否为已注册值。
func (v PlayerSessionState) Valid() bool {
	return contains(v, PlayerSessionOpen, PlayerSessionClosed, PlayerSessionInterrupted)
}

// String 返回序列化后的玩家会话结束原因。
func (v PlayerSessionCloseReason) String() string { return string(v) }

// Valid 返回该玩家会话结束原因是否为已注册值。
func (v PlayerSessionCloseReason) Valid() bool {
	return contains(v, PlayerCloseLeave, PlayerCloseDuplicateJoin, PlayerCloseServerStopped, PlayerCloseUnexpectedExit, PlayerCloseProcessReplaced, PlayerCloseIdentityMerged)
}

// String 返回序列化后的玩家身份类别。
func (v PlayerIdentityKind) String() string { return string(v) }

// Valid 返回该玩家身份类别是否为已注册值。
func (v PlayerIdentityKind) Valid() bool {
	return contains(v, PlayerIdentityUUID, PlayerIdentityNameOnly)
}

// String 返回序列化后的玩家活动数据精确度。
func (v PlayerActivityAccuracy) String() string { return string(v) }

// Valid 返回该玩家活动数据精确度是否为已注册值。
func (v PlayerActivityAccuracy) Valid() bool {
	return contains(v, PlayerAccuracyExact, PlayerAccuracyServerBoundary, PlayerAccuracyEstimated, PlayerAccuracyReconstructed, PlayerAccuracyIncomplete)
}

// String 返回序列化后的生命周期状态。
func (v LifecycleState) String() string { return string(v) }

// Valid 返回该生命周期状态是否为已注册值。
func (v LifecycleState) Valid() bool {
	return contains(v, LifecycleCreating, LifecycleInstalling, LifecycleReady, LifecycleStarting, LifecycleRunning, LifecycleStopping, LifecycleStopped, LifecycleUpdating, LifecycleBackingUp, LifecycleDeleted, LifecycleFailed)
}

// String 返回序列化后的指标名称。
func (v MetricType) String() string { return string(v) }

// Valid 返回该指标名称是否为已注册值。
func (v MetricType) Valid() bool {
	return contains(v, MetricTypes()...)
}

// MetricTypes 返回不可变的已注册 Metric 命名空间列表。
func MetricTypes() []MetricType {
	return []MetricType{
		MetricHostCPU, MetricHostMemory, MetricMinecraftTPS, MetricMinecraftMSPT,
		MetricHostLoad1, MetricHostMemoryUsed, MetricHostMemoryTotal, MetricHostSwapUsed,
		MetricHostDiskUsed, MetricHostDiskTotal, MetricHostDiskRead, MetricHostDiskWrite,
		MetricHostNetworkReceive, MetricHostNetworkTransmit, MetricProcessCPU, MetricProcessRSS,
		MetricProcessThreads, MetricProcessFDs, MetricProcessUptime, MetricProcessRunning,
		MetricMinecraftPlayers,
	}
}

// String 返回序列化后的Minecraft spark 状态。
func (v SparkStatus) String() string { return string(v) }

// Valid 返回该Minecraft spark 状态是否为已注册值。
func (v SparkStatus) Valid() bool {
	return contains(v, SparkUnknown, SparkUnavailable, SparkUnsupported, SparkAvailable, SparkCollecting, SparkFailed)
}

// String 返回序列化后的设置分类。
func (v SettingsCategory) String() string { return string(v) }

// Valid 返回该设置分类是否为已注册值。
func (v SettingsCategory) Valid() bool {
	return contains(v, SettingsGeneral, SettingsTheme, SettingsPaths, SettingsMirrors, SettingsLogging, SettingsMonitoring, SettingsFirewall, SettingsLayout, SettingsSSH, SettingsTerminal, SettingsDownloads, SettingsStorage)
}

func contains[T comparable](value T, candidates ...T) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}
