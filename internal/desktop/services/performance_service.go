package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// PerformanceOverviewResult contains one complete Performance Center snapshot.
type PerformanceOverviewResult struct {
	Overview *service.PerformanceOverview `json:"overview,omitempty"`
	Error    *apperror.DTO                `json:"error,omitempty"`
}

// SparkCapabilityServiceResult contains capability evidence or a stable error.
type SparkCapabilityServiceResult struct {
	Capability *model.SparkCapability `json:"capability,omitempty"`
	Error      *apperror.DTO          `json:"error,omitempty"`
}

// SparkInstallPlanServiceResult contains an exact approved mutation plan.
type SparkInstallPlanServiceResult struct {
	Plan  *service.SparkInstallPlan `json:"plan,omitempty"`
	Error *apperror.DTO             `json:"error,omitempty"`
}

// SparkInstallServiceResult contains the completed install/upgrade result.
type SparkInstallServiceResult struct {
	Result *service.SparkInstallResult `json:"result,omitempty"`
	Error  *apperror.DTO               `json:"error,omitempty"`
}

// SparkSnapshotServiceResult contains one TPS/MSPT snapshot.
type SparkSnapshotServiceResult struct {
	Snapshot *model.SparkSnapshot `json:"snapshot,omitempty"`
	Error    *apperror.DTO        `json:"error,omitempty"`
}

// SparkReportServiceResult contains one durable report and its Operation ID.
type SparkReportServiceResult struct {
	Report *model.SparkReport `json:"report,omitempty"`
	Error  *apperror.DTO      `json:"error,omitempty"`
}

// PerformanceService exposes Spark detection, install, collection, reports, and correlated metrics.
type PerformanceService struct {
	manager *service.PerformanceManager
	logger  *applog.Logger
}

// NewPerformanceService creates the Wails Performance Center facade.
func NewPerformanceService(manager *service.PerformanceManager, logger *applog.Logger) *PerformanceService {
	return &PerformanceService{manager: manager, logger: logger}
}

// Overview returns bounded Spark, report, Agent, and related metric state.
func (s *PerformanceService) Overview(ctx context.Context, serverID string) (result PerformanceOverviewResult) {
	defer s.recoverOverview(ctx, "PerformanceService.Overview", &result)
	overview, err := s.manager.Overview(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return PerformanceOverviewResult{Error: &dto}
	}
	return PerformanceOverviewResult{Overview: &overview}
}

// Probe detects Spark installation, compatibility, permissions, and collection method.
func (s *PerformanceService) Probe(ctx context.Context, serverID string) (result SparkCapabilityServiceResult) {
	defer s.recoverCapability(ctx, "PerformanceService.Probe", &result)
	capability, err := s.manager.Probe(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkCapabilityServiceResult{Error: &dto}
	}
	return SparkCapabilityServiceResult{Capability: &capability}
}

// PlanInstall computes the approved source, checksum, target, backup, and restart impact.
func (s *PerformanceService) PlanInstall(ctx context.Context, serverID string) (result SparkInstallPlanServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "PerformanceService.PlanInstall", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	plan, err := s.manager.PlanInstall(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkInstallPlanServiceResult{Error: &dto}
	}
	return SparkInstallPlanServiceResult{Plan: &plan}
}

// Install executes one explicitly confirmed Spark install or upgrade.
func (s *PerformanceService) Install(ctx context.Context, serverID, planDigest string, confirmed bool) (result SparkInstallServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "PerformanceService.Install", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	installed, err := s.manager.Install(ctx, model.ID(serverID), planDigest, confirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkInstallServiceResult{Error: &dto}
	}
	return SparkInstallServiceResult{Result: &installed}
}

// Rollback restores the latest recorded Spark backup after explicit confirmation.
func (s *PerformanceService) Rollback(ctx context.Context, serverID string, confirmed bool) (result SparkCapabilityServiceResult) {
	defer s.recoverCapability(ctx, "PerformanceService.Rollback", &result)
	capability, err := s.manager.Rollback(ctx, model.ID(serverID), confirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkCapabilityServiceResult{Error: &dto}
	}
	return SparkCapabilityServiceResult{Capability: &capability}
}

// CollectSnapshot collects and persists one locked TPS/MSPT sample.
func (s *PerformanceService) CollectSnapshot(ctx context.Context, serverID string) (result SparkSnapshotServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "PerformanceService.CollectSnapshot", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	snapshot, err := s.manager.CollectSnapshot(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkSnapshotServiceResult{Error: &dto}
	}
	return SparkSnapshotServiceResult{Snapshot: &snapshot}
}

// StartHealthReport starts one privacy-confirmed health report Operation.
func (s *PerformanceService) StartHealthReport(ctx context.Context, serverID string, privacyAcknowledged bool) (result SparkReportServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "PerformanceService.StartHealthReport", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	report, err := s.manager.StartHealthReport(ctx, model.ID(serverID), privacyAcknowledged)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkReportServiceResult{Error: &dto}
	}
	return SparkReportServiceResult{Report: &report}
}

// StartProfiler starts one explicit-duration privacy-confirmed profiler Operation.
func (s *PerformanceService) StartProfiler(ctx context.Context, serverID string, durationSeconds int, privacyAcknowledged bool) (result SparkReportServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "PerformanceService.StartProfiler", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	report, err := s.manager.StartProfiler(ctx, model.ID(serverID), durationSeconds, privacyAcknowledged)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkReportServiceResult{Error: &dto}
	}
	return SparkReportServiceResult{Report: &report}
}

// DeleteReport 删除一条已结束的 Spark 报告记录。
func (s *PerformanceService) DeleteReport(ctx context.Context, reportID string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "PerformanceService.DeleteReport", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.DeleteReport(ctx, model.ID(reportID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *PerformanceService) recoverOverview(ctx context.Context, boundary string, result *PerformanceOverviewResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *PerformanceService) recoverCapability(ctx context.Context, boundary string, result *SparkCapabilityServiceResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
