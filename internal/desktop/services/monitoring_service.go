package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// MonitoringOverviewResult 承载采集器、Metric、Spark、告警与问题的统一状态。
type MonitoringOverviewResult struct {
	Overview *service.MonitoringOverview `json:"overview,omitempty"`
	Error    *apperror.DTO               `json:"error,omitempty"`
}

// MonitoringService 对外暴露统一的监控查询边界。
type MonitoringService struct {
	manager *service.MonitoringManager
	logger  *applog.Logger
}

// NewMonitoringService 创建 Wails 侧的监控门面。
func NewMonitoringService(manager *service.MonitoringManager, logger *applog.Logger) *MonitoringService {
	return &MonitoringService{manager: manager, logger: logger}
}

// Overview 返回采集状态、最新指标、Spark 状态、活跃告警与采集异常。
func (s *MonitoringService) Overview(ctx context.Context, serverID string) (result MonitoringOverviewResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MonitoringService.Overview", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	overview, err := s.manager.Overview(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return MonitoringOverviewResult{Error: &dto}
	}
	return MonitoringOverviewResult{Overview: &overview}
}

// CollectNow 为所选 Server 立即触发一次 SSH 拉取采集。
func (s *MonitoringService) CollectNow(ctx context.Context, serverID string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MonitoringService.CollectNow", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.CollectNow(ctx, model.ID(serverID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Pause 停止所选 Server 后续的自动 Metric 采集。
func (s *MonitoringService) Pause(ctx context.Context, serverID string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MonitoringService.Pause", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.Pause(ctx, model.ID(serverID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Resume 恢复所选 Server 的自动 Metric 采集。
func (s *MonitoringService) Resume(ctx context.Context, serverID string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MonitoringService.Resume", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.Resume(ctx, model.ID(serverID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// ClearHistory 删除所选 Server 的全部持久化 Metric 数据。
func (s *MonitoringService) ClearHistory(ctx context.Context, serverID string) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MonitoringService.ClearHistory", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := s.manager.ClearHistory(ctx, model.ID(serverID)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Query 通过统一门面返回走索引的指标历史,并显式标注空洞。
func (s *MonitoringService) Query(ctx context.Context, query model.MetricQuery) (result MetricQueryServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MonitoringService.Query", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	queryResult, err := s.manager.Query(ctx, query)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MetricQueryServiceResult{Error: &dto}
	}
	return MetricQueryServiceResult{Result: &queryResult}
}
