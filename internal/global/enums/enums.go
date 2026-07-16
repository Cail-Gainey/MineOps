// Package enums contains validated strong string enums shared across MineOps layers.
package enums

// OperationState identifies the durable lifecycle of a long-running operation.
type OperationState string

// OperationType identifies the user intent executed by a durable operation.
type OperationType string

// OperationTargetType identifies the resource class locked by an operation.
type OperationTargetType string

// SSHAuthType identifies the credential mechanism used by an SSH session.
type SSHAuthType string

// SSHHostKeyPolicy identifies the host-key verification policy for an SSH session.
type SSHHostKeyPolicy string

// MinecraftServerType identifies one supported server distribution or proxy family.
type MinecraftServerType string

// FirewallPolicy identifies how MineOps handles the configured Minecraft TCP port.
type FirewallPolicy string

// InstallationState identifies the durable lifecycle of one installation task.
type InstallationState string

// InstallationStepState identifies one checkpointed installation step lifecycle.
type InstallationStepState string

// ProxyMode identifies how outbound catalog and artifact HTTP traffic is routed.
type ProxyMode string

// RemoteProcessState identifies probe evidence independently from persisted Server lifecycle state.
type RemoteProcessState string

// FirewallBackend identifies the detected remote Linux firewall implementation.
type FirewallBackend string

// MetricUnit identifies the physical or semantic unit of a metric.
type MetricUnit string

// MetricValueType identifies how a metric value should be rendered and validated.
type MetricValueType string

// MetricAggregation identifies the preferred rollup calculation.
type MetricAggregation string

// MetricGranularity identifies raw, minute, or hour storage/query resolution.
type MetricGranularity string

// SparkReportKind identifies one controlled Minecraft spark report workflow.
type SparkReportKind string

// SparkReportState identifies the durable lifecycle of one Minecraft spark report.
type SparkReportState string

// AlertComparison identifies the bounded numeric comparison used by an alert rule.
type AlertComparison string

// AlertEventState identifies whether a threshold incident is active or recovered.
type AlertEventState string

// PlayerActivityEventType identifies a collected player presence event.
type PlayerActivityEventType string

// PlayerSessionState identifies whether a player connection is still open or settled.
type PlayerSessionState string

// PlayerSessionCloseReason identifies why a player session was settled.
type PlayerSessionCloseReason string

// PlayerIdentityKind identifies the evidence backing a Server-scoped player identity.
type PlayerIdentityKind string

// PlayerActivityAccuracy identifies the confidence of persisted player activity data.
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

// LifecycleState identifies the verified Minecraft server lifecycle.
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

// MetricType identifies the namespace of a persisted metric sample.
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

// SparkStatus identifies Minecraft spark availability independently from Agent state.
type SparkStatus string

// SettingsCategory identifies one independently resettable settings section.
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

// String returns the serialized operation state.
func (v OperationState) String() string { return string(v) }

// Valid reports whether the operation state is registered.
func (v OperationState) Valid() bool {
	return contains(v, OperationPending, OperationRunning, OperationSucceeded, OperationFailed, OperationCancelled)
}

// String returns the serialized operation type.
func (v OperationType) String() string { return string(v) }

// Valid reports whether the operation type is registered.
func (v OperationType) Valid() bool {
	return contains(v, OperationInstall, OperationDownload, OperationUpload, OperationExtract, OperationStart, OperationStop, OperationRestart, OperationBackup, OperationRestore, OperationUpdate, OperationDelete, OperationProfile)
}

// String returns the serialized operation target type.
func (v OperationTargetType) String() string { return string(v) }

// Valid reports whether the operation target type is registered.
func (v OperationTargetType) Valid() bool {
	return contains(v, OperationTargetServer, OperationTargetSSHSession, OperationTargetFile, OperationTargetJava, OperationTargetAgent, OperationTargetSettings, OperationTargetSpark)
}

// String returns the serialized SSH authentication type.
func (v SSHAuthType) String() string { return string(v) }

// Valid reports whether the SSH authentication type is registered.
func (v SSHAuthType) Valid() bool {
	return contains(v, SSHAuthPassword, SSHAuthPrivateKey, SSHAuthAgent)
}

// String returns the serialized SSH host-key policy.
func (v SSHHostKeyPolicy) String() string { return string(v) }

// Valid reports whether the SSH host-key policy is registered.
func (v SSHHostKeyPolicy) Valid() bool {
	return contains(v, SSHHostKeyStrict, SSHHostKeyTrustOnFirst)
}

// String returns the serialized Minecraft server type.
func (v MinecraftServerType) String() string { return string(v) }

// Valid reports whether the Minecraft server type is registered.
func (v MinecraftServerType) Valid() bool {
	return contains(v, ServerVanilla, ServerPaper, ServerPurpur, ServerSpigot, ServerFabric, ServerForge, ServerNeoForge, ServerQuilt, ServerFolia, ServerVelocity, ServerWaterfall, ServerBungee)
}

// String returns the serialized firewall policy.
func (v FirewallPolicy) String() string { return string(v) }

// Valid reports whether the firewall policy is registered.
func (v FirewallPolicy) Valid() bool {
	return contains(v, FirewallDisabled, FirewallPrompt, FirewallAutomatic)
}

// String returns the serialized installation state.
func (v InstallationState) String() string { return string(v) }

// Valid reports whether the installation state is registered.
func (v InstallationState) Valid() bool {
	return contains(v, InstallationWaiting, InstallationRunning, InstallationSucceeded, InstallationFailed, InstallationCancelled)
}

// String returns the serialized installation step state.
func (v InstallationStepState) String() string { return string(v) }

// Valid reports whether the installation step state is registered.
func (v InstallationStepState) Valid() bool {
	return contains(v, InstallationStepWaiting, InstallationStepRunning, InstallationStepSuccess, InstallationStepFailed, InstallationStepSkipped, InstallationStepCancelled)
}

// String returns the serialized proxy mode.
func (v ProxyMode) String() string { return string(v) }

// Valid reports whether the proxy mode is registered.
func (v ProxyMode) Valid() bool {
	return contains(v, ProxyNone, ProxySystem, ProxyHTTP, ProxyHTTPS, ProxySOCKS5)
}

// String returns the serialized remote process state.
func (v RemoteProcessState) String() string { return string(v) }

// Valid reports whether the remote process state is registered.
func (v RemoteProcessState) Valid() bool {
	return contains(v, RemoteProcessUnknown, RemoteProcessRunning, RemoteProcessExited, RemoteProcessMismatched)
}

// String returns the serialized firewall backend.
func (v FirewallBackend) String() string { return string(v) }

// Valid reports whether the firewall backend is registered.
func (v FirewallBackend) Valid() bool {
	return contains(v, FirewallBackendNone, FirewallBackendUFW, FirewallBackendFirewalld)
}

// String returns the serialized metric unit.
func (v MetricUnit) String() string { return string(v) }

// Valid reports whether the metric unit is registered.
func (v MetricUnit) Valid() bool {
	return contains(v, MetricUnitPercent, MetricUnitBytes, MetricUnitBytesPerSec, MetricUnitCount, MetricUnitMilliseconds, MetricUnitTicksPerSec, MetricUnitRatio, MetricUnitSeconds)
}

// String returns the serialized metric value type.
func (v MetricValueType) String() string { return string(v) }

// Valid reports whether the metric value type is registered.
func (v MetricValueType) Valid() bool { return contains(v, MetricValueGauge, MetricValueCounter) }

// String returns the serialized metric aggregation.
func (v MetricAggregation) String() string { return string(v) }

// Valid reports whether the metric aggregation is registered.
func (v MetricAggregation) Valid() bool {
	return contains(v, MetricAggregationAverage, MetricAggregationSum, MetricAggregationLatest, MetricAggregationMaximum)
}

// String returns the serialized metric granularity.
func (v MetricGranularity) String() string { return string(v) }

// Valid reports whether the metric granularity is registered.
func (v MetricGranularity) Valid() bool {
	return contains(v, MetricGranularityRaw, MetricGranularityMinute, MetricGranularityHour)
}

// String returns the serialized Minecraft spark report kind.
func (v SparkReportKind) String() string { return string(v) }

// Valid reports whether the Minecraft spark report kind is registered.
func (v SparkReportKind) Valid() bool { return contains(v, SparkReportHealth, SparkReportProfiler) }

// String returns the serialized Minecraft spark report state.
func (v SparkReportState) String() string { return string(v) }

// Valid reports whether the Minecraft spark report state is registered.
func (v SparkReportState) Valid() bool {
	return contains(v, SparkReportPending, SparkReportRunning, SparkReportCompleted, SparkReportFailed, SparkReportCancelled)
}

// String returns the serialized alert comparison.
func (v AlertComparison) String() string { return string(v) }

// Valid reports whether the alert comparison is registered.
func (v AlertComparison) Valid() bool {
	return contains(v, AlertGreaterThan, AlertGreaterThanOrEqual, AlertLessThan, AlertLessThanOrEqual)
}

// Match evaluates the registered numeric comparison.
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

// String returns the serialized alert event state.
func (v AlertEventState) String() string { return string(v) }

// Valid reports whether the alert event state is registered.
func (v AlertEventState) Valid() bool { return contains(v, AlertEventActive, AlertEventRecovered) }

// String returns the serialized player activity event type.
func (v PlayerActivityEventType) String() string { return string(v) }

// Valid reports whether the player activity event type is registered.
func (v PlayerActivityEventType) Valid() bool {
	return contains(v, PlayerActivityJoin, PlayerActivityLeave)
}

// String returns the serialized player session state.
func (v PlayerSessionState) String() string { return string(v) }

// Valid reports whether the player session state is registered.
func (v PlayerSessionState) Valid() bool {
	return contains(v, PlayerSessionOpen, PlayerSessionClosed, PlayerSessionInterrupted)
}

// String returns the serialized player session close reason.
func (v PlayerSessionCloseReason) String() string { return string(v) }

// Valid reports whether the player session close reason is registered.
func (v PlayerSessionCloseReason) Valid() bool {
	return contains(v, PlayerCloseLeave, PlayerCloseDuplicateJoin, PlayerCloseServerStopped, PlayerCloseUnexpectedExit, PlayerCloseProcessReplaced, PlayerCloseIdentityMerged)
}

// String returns the serialized player identity kind.
func (v PlayerIdentityKind) String() string { return string(v) }

// Valid reports whether the player identity kind is registered.
func (v PlayerIdentityKind) Valid() bool {
	return contains(v, PlayerIdentityUUID, PlayerIdentityNameOnly)
}

// String returns the serialized player activity accuracy.
func (v PlayerActivityAccuracy) String() string { return string(v) }

// Valid reports whether the player activity accuracy is registered.
func (v PlayerActivityAccuracy) Valid() bool {
	return contains(v, PlayerAccuracyExact, PlayerAccuracyServerBoundary, PlayerAccuracyEstimated, PlayerAccuracyReconstructed, PlayerAccuracyIncomplete)
}

// String returns the serialized lifecycle state.
func (v LifecycleState) String() string { return string(v) }

// Valid reports whether the lifecycle state is registered.
func (v LifecycleState) Valid() bool {
	return contains(v, LifecycleCreating, LifecycleInstalling, LifecycleReady, LifecycleStarting, LifecycleRunning, LifecycleStopping, LifecycleStopped, LifecycleUpdating, LifecycleBackingUp, LifecycleDeleted, LifecycleFailed)
}

// String returns the serialized metric type.
func (v MetricType) String() string { return string(v) }

// Valid reports whether the metric type is registered.
func (v MetricType) Valid() bool {
	return contains(v, MetricTypes()...)
}

// MetricTypes returns the immutable registered Metric namespace list.
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

// String returns the serialized Minecraft spark status.
func (v SparkStatus) String() string { return string(v) }

// Valid reports whether the Minecraft spark status is registered.
func (v SparkStatus) Valid() bool {
	return contains(v, SparkUnknown, SparkUnavailable, SparkUnsupported, SparkAvailable, SparkCollecting, SparkFailed)
}

// String returns the serialized settings category.
func (v SettingsCategory) String() string { return string(v) }

// Valid reports whether the settings category is registered.
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
