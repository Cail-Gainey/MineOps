package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// PerformanceOverviewResult 承载一份完整的性能中心快照。
type PerformanceOverviewResult struct {
	Overview *service.PerformanceOverview `json:"overview,omitempty"`
	Error    *apperror.DTO                `json:"error,omitempty"`
}

// SparkCapabilityServiceResult 承载 Spark 能力证据或稳定错误。
type SparkCapabilityServiceResult struct {
	Capability *model.SparkCapability `json:"capability,omitempty"`
	Error      *apperror.DTO          `json:"error,omitempty"`
}

// SparkInstallPlanServiceResult 承载一份确切且已核准的变更计划。
type SparkInstallPlanServiceResult struct {
	Plan  *service.SparkInstallPlan `json:"plan,omitempty"`
	Error *apperror.DTO             `json:"error,omitempty"`
}

// SparkInstallServiceResult 承载安装或升级完成后的结果。
type SparkInstallServiceResult struct {
	Result *service.SparkInstallResult `json:"result,omitempty"`
	Error  *apperror.DTO               `json:"error,omitempty"`
}

// SparkSnapshotServiceResult 承载一条 TPS/MSPT Snapshot。
type SparkSnapshotServiceResult struct {
	Snapshot *model.SparkSnapshot `json:"snapshot,omitempty"`
	Error    *apperror.DTO        `json:"error,omitempty"`
}

// SparkReportServiceResult 承载一份持久化报告及其 Operation ID。
type SparkReportServiceResult struct {
	Report *model.SparkReport `json:"report,omitempty"`
	Error  *apperror.DTO      `json:"error,omitempty"`
}

// PerformanceService 对外暴露 Spark 探测、安装、采集、报告与关联指标。
type PerformanceService struct {
	manager *service.PerformanceManager
	logger  *applog.Logger
}

// NewPerformanceService 创建 Wails 侧的性能中心门面。
func NewPerformanceService(manager *service.PerformanceManager, logger *applog.Logger) *PerformanceService {
	return &PerformanceService{manager: manager, logger: logger}
}

// Overview 返回有界的 Spark、报告、采集与相关指标状态。
func (s *PerformanceService) Overview(ctx context.Context, serverID string) (result PerformanceOverviewResult) {
	defer s.recoverOverview(ctx, "PerformanceService.Overview", &result)
	overview, err := s.manager.Overview(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return PerformanceOverviewResult{Error: &dto}
	}
	return PerformanceOverviewResult{Overview: &overview}
}

// Probe 探测 Spark 的安装情况、兼容性、权限与采集方式。
func (s *PerformanceService) Probe(ctx context.Context, serverID string) (result SparkCapabilityServiceResult) {
	defer s.recoverCapability(ctx, "PerformanceService.Probe", &result)
	capability, err := s.manager.Probe(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkCapabilityServiceResult{Error: &dto}
	}
	return SparkCapabilityServiceResult{Capability: &capability}
}

// PlanInstall 计算已核准的来源、校验和、目标路径、备份与重启影响。
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

// Install 执行一次显式确认过的 Spark 安装或升级。
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

// Rollback 在显式确认后恢复最近一次记录的 Spark 备份。
func (s *PerformanceService) Rollback(ctx context.Context, serverID string, confirmed bool) (result SparkCapabilityServiceResult) {
	defer s.recoverCapability(ctx, "PerformanceService.Rollback", &result)
	capability, err := s.manager.Rollback(ctx, model.ID(serverID), confirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return SparkCapabilityServiceResult{Error: &dto}
	}
	return SparkCapabilityServiceResult{Capability: &capability}
}

// CollectSnapshot 采集并持久化一条锁定的 TPS/MSPT 样本。
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

// StartHealthReport 启动一次已确认隐私风险的健康报告 Operation。
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

// StartProfiler 启动一次时长显式、已确认隐私风险的性能分析 Operation。
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
