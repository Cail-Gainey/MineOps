package service

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// MonitoringIssue distinguishes 采集失败、指标过期、Spark 不可用与 Spark 不兼容。
type MonitoringIssue struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"timestamp"`
}

// MonitoringOverview 汇总采集器状态、最新指标、Spark 状态、告警与采集问题。
type MonitoringOverview struct {
	ServerID     model.ID               `json:"serverID"`
	Collector    *CollectorStatus       `json:"collector,omitempty"`
	Latest       []model.MetricSample   `json:"latest"`
	Spark        *model.SparkCapability `json:"spark,omitempty"`
	ActiveAlerts []model.AlertEvent     `json:"activeAlerts"`
	Issues       []MonitoringIssue      `json:"issues"`
	CollectedAt  *time.Time             `json:"collectedAt,omitempty"`
}

// MonitoringManager provides the unified stage 12 monitoring query boundary.
type MonitoringManager struct {
	clock     model.Clock
	store     repository.Store
	settings  *appsettings.Manager
	metrics   *MetricManager
	collector *MetricCollector
	spark     *SparkManager
}

// NewMonitoringManager creates the unified Collector, Metric, Spark, and Alert query service.
func NewMonitoringManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, metrics *MetricManager, collector *MetricCollector, spark *SparkManager) (*MonitoringManager, error) {
	if clock == nil || store == nil || settings == nil || metrics == nil || collector == nil || spark == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Monitoring Service 依赖不能为空")
	}
	return &MonitoringManager{clock: clock, store: store, settings: settings, metrics: metrics, collector: collector, spark: spark}, nil
}

// Overview returns unified current state while preserving distinct failure categories.
func (m *MonitoringManager) Overview(ctx context.Context, serverID model.ID) (MonitoringOverview, error) {
	if _, err := m.store.MinecraftServers().Get(ctx, serverID, false); err != nil {
		return MonitoringOverview{}, err
	}
	overview := MonitoringOverview{ServerID: serverID}
	var err error
	overview.Latest, err = m.metrics.Latest(ctx, serverID)
	if err != nil {
		return MonitoringOverview{}, err
	}
	for _, sample := range overview.Latest {
		if overview.CollectedAt == nil || sample.Timestamp.After(*overview.CollectedAt) {
			timestamp := sample.Timestamp
			overview.CollectedAt = &timestamp
		}
	}
	if capability, capabilityErr := m.store.Spark().GetCapability(ctx, serverID); capabilityErr == nil {
		overview.Spark = capability
	} else if apperror.ToDTO(capabilityErr).Code != apperror.CodeIONotFound.String() {
		return MonitoringOverview{}, capabilityErr
	}
	overview.ActiveAlerts, err = m.store.Alerts().ListEvents(ctx, repository.AlertEventQuery{ServerID: serverID, State: enums.AlertEventActive, Limit: 100})
	if err != nil {
		return MonitoringOverview{}, err
	}
	collectorStatus := m.collector.Status(serverID)
	overview.Collector = &collectorStatus
	now := m.clock.Now().UTC()
	if collectorStatus.Paused {
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: "collector.paused", Message: "该 Server 的自动监控采集已暂停", Severity: "warning", Timestamp: now})
	} else if collectorStatus.LastError != "" {
		timestamp := now
		if collectorStatus.LastErrorAt != nil {
			timestamp = *collectorStatus.LastErrorAt
		}
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: "collector.error", Message: collectorStatus.LastError, Severity: "warning", Timestamp: timestamp})
	}
	if !collectorStatus.Paused && overview.CollectedAt != nil && now.Sub(*overview.CollectedAt) > time.Duration(m.settings.Snapshot().Monitoring.OfflineAfterSeconds)*time.Second {
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: "metric.stale", Message: "最新 Metric 已超过离线判定窗口", Severity: "warning", Timestamp: *overview.CollectedAt})
	}
	if !collectorStatus.Paused && len(overview.Latest) == 0 && collectorStatus.CollectedAt == nil {
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: "collector.no_data", Message: "尚未采集到该 Server 的任何监控数据", Severity: "warning", Timestamp: now})
	}
	if overview.Spark == nil || overview.Spark.Status == enums.SparkUnavailable {
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: "spark.unavailable", Message: "Minecraft spark 未安装或当前不可用", Severity: "warning", Timestamp: now})
	} else if overview.Spark.Status == enums.SparkUnsupported {
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: "spark.unsupported", Message: "Minecraft spark 版本或平台不兼容", Severity: "error", Timestamp: overview.Spark.DetectedAt})
	} else if overview.Spark.Status == enums.SparkFailed {
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: overview.Spark.LastErrorCode, Message: overview.Spark.LastError, Severity: "error", Timestamp: overview.Spark.DetectedAt})
	} else if overview.Spark.LastError != "" {
		overview.Issues = append(overview.Issues, MonitoringIssue{Code: overview.Spark.LastErrorCode, Message: overview.Spark.LastError, Severity: "warning", Timestamp: overview.Spark.DetectedAt})
	}
	if overview.Spark != nil && overview.Spark.Status == enums.SparkAvailable && !collectorStatus.Paused {
		latestSpark, latestSparkErr := m.store.Spark().LatestSnapshot(ctx, serverID)
		if latestSparkErr == nil {
			maximumAge := time.Duration(m.settings.Snapshot().Monitoring.SparkIntervalSeconds*3) * time.Second
			if now.Sub(latestSpark.CollectedAt) > maximumAge {
				overview.Issues = append(overview.Issues, MonitoringIssue{Code: "spark.stale", Message: "Spark TPS/MSPT 数据已超过三个采集周期未更新", Severity: "warning", Timestamp: latestSpark.CollectedAt})
			}
		} else if apperror.ToDTO(latestSparkErr).Code == apperror.CodeIONotFound.String() {
			overview.Issues = append(overview.Issues, MonitoringIssue{Code: "spark.no_data", Message: "Spark 可用但尚未产生 TPS/MSPT Snapshot", Severity: "warning", Timestamp: overview.Spark.DetectedAt})
		} else {
			return MonitoringOverview{}, latestSparkErr
		}
	}
	return overview, nil
}

// CollectNow triggers one immediate pull collection pass for the selected Server.
func (m *MonitoringManager) CollectNow(ctx context.Context, serverID model.ID) error {
	if _, err := m.store.MinecraftServers().Get(ctx, serverID, false); err != nil {
		return err
	}
	return m.collector.CollectNow(ctx, serverID)
}

// Pause stops future automatic collection passes for one Server.
func (m *MonitoringManager) Pause(ctx context.Context, serverID model.ID) error {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	if err := m.collector.Pause(serverID); err != nil {
		return err
	}
	if err := m.collector.stopRemoteCollector(ctx, *server); err != nil {
		_ = m.collector.Resume(serverID)
		return err
	}
	if err := m.spark.stopRemoteCollector(ctx, *server); err != nil {
		_ = m.collector.Resume(serverID)
		_ = m.collector.ReconcileSession(context.WithoutCancel(ctx), server.SSHSessionID)
		return err
	}
	return nil
}

// Resume restores automatic collection for one Server.
func (m *MonitoringManager) Resume(ctx context.Context, serverID model.ID) error {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	if err := m.collector.Resume(serverID); err != nil {
		return err
	}
	if err := m.collector.ReconcileServer(ctx, serverID); err != nil {
		_ = m.collector.Pause(serverID)
		_ = m.collector.ReconcileSession(context.WithoutCancel(ctx), server.SSHSessionID)
		return err
	}
	if server.State == enums.LifecycleRunning {
		m.spark.ServerStarted(serverID)
	}
	return nil
}

// ClearHistory deletes every persisted Metric row and cached latest value for one Server.
func (m *MonitoringManager) ClearHistory(ctx context.Context, serverID model.ID) error {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	if err := m.spark.clearRemoteHistory(ctx, *server); err != nil {
		return err
	}
	if _, err := m.metrics.ClearServerHistory(ctx, serverID); err != nil {
		return err
	}
	if _, err := m.store.Spark().DeleteServerSnapshots(ctx, serverID, 10_000); err != nil {
		return err
	}
	m.collector.ClearStatus(serverID)
	if server.State == enums.LifecycleRunning && !m.collector.IsPaused(serverID) {
		m.spark.ServerStarted(serverID)
	}
	return nil
}

// Query delegates indexed history and explicit gap handling to Metric Manager.
func (m *MonitoringManager) Query(ctx context.Context, query model.MetricQuery) (model.MetricQueryResult, error) {
	return m.metrics.Query(ctx, query)
}
