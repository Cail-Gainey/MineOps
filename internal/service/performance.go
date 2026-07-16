package service

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// PerformanceOverview combines Spark capability, snapshots, reports, and related host/process metrics.
type PerformanceOverview struct {
	ServerID       model.ID               `json:"serverID"`
	Capability     *model.SparkCapability `json:"capability,omitempty"`
	LatestSnapshot *model.SparkSnapshot   `json:"latestSnapshot,omitempty"`
	Snapshots      []model.SparkSnapshot  `json:"snapshots"`
	Reports        []model.SparkReport    `json:"reports"`
	RelatedMetrics []model.MetricSample   `json:"relatedMetrics"`
}

// PerformanceManager provides the Performance Center query and workflow boundary.
type PerformanceManager struct {
	store   repository.Store
	spark   *SparkManager
	metrics *MetricManager
}

// NewPerformanceManager creates the Spark and correlated metric query service.
func NewPerformanceManager(store repository.Store, spark *SparkManager, metrics *MetricManager) (*PerformanceManager, error) {
	if store == nil || spark == nil || metrics == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Performance Service 依赖不能为空")
	}
	return &PerformanceManager{store: store, spark: spark, metrics: metrics}, nil
}

// Overview returns bounded Performance Center state for one Server.
func (m *PerformanceManager) Overview(ctx context.Context, serverID model.ID) (PerformanceOverview, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return PerformanceOverview{}, err
	}
	overview := PerformanceOverview{ServerID: serverID}
	if capability, err := m.spark.GetCapability(ctx, serverID); err == nil {
		overview.Capability = capability
	} else {
		if apperror.ToDTO(err).Code != apperror.CodeIONotFound.String() {
			return PerformanceOverview{}, err
		}
		if server.State == enums.LifecycleRunning || server.State == enums.LifecycleStarting {
			if capability, probeErr := m.spark.Probe(ctx, serverID); probeErr == nil {
				overview.Capability = &capability
			}
		}
	}
	if snapshot, err := m.store.Spark().LatestSnapshot(ctx, serverID); err == nil {
		overview.LatestSnapshot = snapshot
	} else if apperror.ToDTO(err).Code != apperror.CodeIONotFound.String() {
		return PerformanceOverview{}, err
	}
	overview.Snapshots, err = m.store.Spark().ListSnapshots(ctx, repository.SparkSnapshotQuery{ServerID: serverID, Limit: 500})
	if err != nil {
		return PerformanceOverview{}, err
	}
	overview.Reports, err = m.store.Spark().ListReports(ctx, repository.SparkReportQuery{ServerID: serverID, Limit: 200})
	if err != nil {
		return PerformanceOverview{}, err
	}
	latest, err := m.metrics.Latest(ctx, serverID)
	if err != nil {
		return PerformanceOverview{}, err
	}
	for _, sample := range latest {
		switch sample.Metric {
		case enums.MetricHostCPU, enums.MetricHostMemory, enums.MetricProcessCPU, enums.MetricProcessRSS, enums.MetricProcessThreads, enums.MetricMinecraftTPS, enums.MetricMinecraftMSPT:
			overview.RelatedMetrics = append(overview.RelatedMetrics, sample)
		}
	}
	return overview, nil
}

// Probe delegates Spark capability detection for the selected Server.
func (m *PerformanceManager) Probe(ctx context.Context, serverID model.ID) (model.SparkCapability, error) {
	return m.spark.Probe(ctx, serverID)
}

// PlanInstall delegates exact official Spark installation planning.
func (m *PerformanceManager) PlanInstall(ctx context.Context, serverID model.ID) (SparkInstallPlan, error) {
	return m.spark.PlanInstall(ctx, serverID)
}

// Install delegates confirmed Spark install or upgrade execution.
func (m *PerformanceManager) Install(ctx context.Context, serverID model.ID, planDigest string, confirmed bool) (SparkInstallResult, error) {
	return m.spark.Install(ctx, serverID, planDigest, confirmed)
}

// Rollback delegates confirmed Spark artifact rollback.
func (m *PerformanceManager) Rollback(ctx context.Context, serverID model.ID, confirmed bool) (model.SparkCapability, error) {
	return m.spark.Rollback(ctx, serverID, confirmed)
}

// CollectSnapshot delegates one versioned TPS/MSPT collection pass.
func (m *PerformanceManager) CollectSnapshot(ctx context.Context, serverID model.ID) (model.SparkSnapshot, error) {
	return m.spark.CollectSnapshot(ctx, serverID)
}

// StartHealthReport delegates one privacy-confirmed health report operation.
func (m *PerformanceManager) StartHealthReport(ctx context.Context, serverID model.ID, privacyAcknowledged bool) (model.SparkReport, error) {
	return m.spark.StartHealthReport(ctx, serverID, privacyAcknowledged)
}

// StartProfiler delegates one explicit-duration cancellable profiler operation.
func (m *PerformanceManager) StartProfiler(ctx context.Context, serverID model.ID, durationSeconds int, privacyAcknowledged bool) (model.SparkReport, error) {
	return m.spark.StartProfiler(ctx, serverID, durationSeconds, privacyAcknowledged)
}

// DeleteReport 删除一条已结束的 Spark 报告记录。
func (m *PerformanceManager) DeleteReport(ctx context.Context, reportID model.ID) error {
	return m.spark.DeleteReport(ctx, reportID)
}
