package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// MonitoringOverviewResult contains unified Collector, Metric, Spark, Alert, and issue state.
type MonitoringOverviewResult struct {
	Overview *service.MonitoringOverview `json:"overview,omitempty"`
	Error    *apperror.DTO               `json:"error,omitempty"`
}

// MonitoringService exposes the unified stage 12 monitoring query boundary.
type MonitoringService struct {
	manager *service.MonitoringManager
	logger  *applog.Logger
}

// NewMonitoringService creates the Wails Monitoring facade.
func NewMonitoringService(manager *service.MonitoringManager, logger *applog.Logger) *MonitoringService {
	return &MonitoringService{manager: manager, logger: logger}
}

// Overview returns Agent state, latest metrics, Spark state, active alerts, and collection issues.
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

// CollectNow triggers one immediate SSH pull collection pass for the selected Server.
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

// Pause stops future automatic Metric collection passes for the selected Server.
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

// Resume restores automatic Metric collection for the selected Server.
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

// ClearHistory deletes every persisted Metric row for the selected Server.
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

// Query returns indexed metric history with explicit gaps through the unified facade.
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
