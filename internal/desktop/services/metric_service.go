package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// MetricIngestServiceResult contains one atomic ingest decision or a stable error.
type MetricIngestServiceResult struct {
	Result *model.MetricIngestResult `json:"result,omitempty"`
	Error  *apperror.DTO             `json:"error,omitempty"`
}

// MetricLatestServiceResult contains latest cached or persisted values.
type MetricLatestServiceResult struct {
	Samples []model.MetricSample `json:"samples"`
	Error   *apperror.DTO        `json:"error,omitempty"`
}

// MetricQueryServiceResult contains query-ready Metric series.
type MetricQueryServiceResult struct {
	Result *model.MetricQueryResult `json:"result,omitempty"`
	Error  *apperror.DTO            `json:"error,omitempty"`
}

// MetricMaintenanceServiceResult contains one maintenance pass summary.
type MetricMaintenanceServiceResult struct {
	Result *model.MetricMaintenanceResult `json:"result,omitempty"`
	Error  *apperror.DTO                  `json:"error,omitempty"`
}

// MetricStorageServiceResult contains current database capacity evidence.
type MetricStorageServiceResult struct {
	Status *model.MetricStorageStatus `json:"status,omitempty"`
	Error  *apperror.DTO              `json:"error,omitempty"`
}

// MetricService exposes stage 10 ingest, latest, history, maintenance, and capacity operations.
type MetricService struct {
	manager *service.MetricManager
	logger  *applog.Logger
}

// NewMetricService creates the desktop Metric facade.
func NewMetricService(manager *service.MetricManager, logger *applog.Logger) *MetricService {
	return &MetricService{manager: manager, logger: logger}
}

// Ingest atomically accepts one bounded Metric batch.
func (s *MetricService) Ingest(ctx context.Context, samples []model.MetricSample) (result MetricIngestServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MetricService.Ingest", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	accepted, err := s.manager.Ingest(ctx, samples)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MetricIngestServiceResult{Error: &dto}
	}
	return MetricIngestServiceResult{Result: &accepted}
}

// Latest returns every available latest Metric for one Server.
func (s *MetricService) Latest(ctx context.Context, serverID string) (result MetricLatestServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MetricService.Latest", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	samples, err := s.manager.Latest(ctx, model.ID(serverID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return MetricLatestServiceResult{Error: &dto}
	}
	return MetricLatestServiceResult{Samples: samples}
}

// Query returns raw, minute, or hour series with explicit missing-data points.
func (s *MetricService) Query(ctx context.Context, query model.MetricQuery) (result MetricQueryServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MetricService.Query", &err)
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

// RunMaintenance triggers one bounded rollup and retention pass.
func (s *MetricService) RunMaintenance(ctx context.Context) (result MetricMaintenanceServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MetricService.RunMaintenance", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	maintenance, err := s.manager.RunMaintenance(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MetricMaintenanceServiceResult{Error: &dto}
	}
	return MetricMaintenanceServiceResult{Result: &maintenance}
}

// StorageStatus returns database capacity and free-disk status.
func (s *MetricService) StorageStatus(ctx context.Context) (result MetricStorageServiceResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "MetricService.StorageStatus", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	status, err := s.manager.StorageStatus(ctx)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MetricStorageServiceResult{Error: &dto}
	}
	return MetricStorageServiceResult{Status: &status}
}
