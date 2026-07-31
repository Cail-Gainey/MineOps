package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// MetricIngestServiceResult 承载一次原子写入的判定结果或稳定错误。
type MetricIngestServiceResult struct {
	Result *model.MetricIngestResult `json:"result,omitempty"`
	Error  *apperror.DTO             `json:"error,omitempty"`
}

// MetricLatestServiceResult 承载缓存或持久化的最新值。
type MetricLatestServiceResult struct {
	Samples []model.MetricSample `json:"samples"`
	Error   *apperror.DTO        `json:"error,omitempty"`
}

// MetricQueryServiceResult 承载可直接展示的 Metric 序列。
type MetricQueryServiceResult struct {
	Result *model.MetricQueryResult `json:"result,omitempty"`
	Error  *apperror.DTO            `json:"error,omitempty"`
}

// MetricMaintenanceServiceResult 承载一轮维护的执行摘要。
type MetricMaintenanceServiceResult struct {
	Result *model.MetricMaintenanceResult `json:"result,omitempty"`
	Error  *apperror.DTO                  `json:"error,omitempty"`
}

// MetricStorageServiceResult 承载当前数据库容量证据。
type MetricStorageServiceResult struct {
	Status *model.MetricStorageStatus `json:"status,omitempty"`
	Error  *apperror.DTO              `json:"error,omitempty"`
}

// MetricService 对外暴露写入、最新值、历史、维护与容量操作。
type MetricService struct {
	manager *service.MetricManager
	logger  *applog.Logger
}

// NewMetricService 创建桌面侧的 Metric 门面。
func NewMetricService(manager *service.MetricManager, logger *applog.Logger) *MetricService {
	return &MetricService{manager: manager, logger: logger}
}

// Ingest 原子接收一个有界的 Metric 批次。
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

// Latest 返回某台 Server 全部可用指标的最新值。
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

// Query 返回原始、分钟或小时序列,并显式标注缺失数据点。
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

// RunMaintenance 触发一轮有界的降采样与保留清理。
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

// StorageStatus 返回数据库容量与磁盘可用空间状态。
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
