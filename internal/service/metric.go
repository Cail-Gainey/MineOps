package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

const maximumMetricIngestBatch = 5_000

// MetricObserver 接收已持久化的样本,但不持有写入事务。
type MetricObserver interface {
	ObserveMetrics([]model.MetricSample)
}

// MetricManager 统筹原子写入、实时缓存、查询、降采样、保留清理与容量保护。
type MetricManager struct {
	clock        model.Clock
	store        repository.Store
	settings     *appsettings.Manager
	bus          *MetricBus
	databasePath string
	logger       *applog.Logger
	observer     MetricObserver

	maintenanceMu sync.Mutex
	// 降采样水位线（受 maintenanceMu 保护）：仅重算新完成的桶，避免每轮全量重读原始样本。
	minuteRolledUpTo time.Time
	hourRolledUpTo   time.Time
}

// SetObserver 装配持久化之后的有界指标观察者,供阈值判定使用。
func (m *MetricManager) SetObserver(observer MetricObserver) {
	if m != nil {
		m.observer = observer
	}
}

// NewMetricManager 创建 Metric 应用服务。
func NewMetricManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, bus *MetricBus, databasePath string, logger *applog.Logger) (*MetricManager, error) {
	if clock == nil || store == nil || settings == nil || bus == nil || strings.TrimSpace(databasePath) == "" {
		return nil, apperror.New(apperror.CodeValidationRequired, "Metric 依赖和数据库路径不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &MetricManager{clock: clock, store: store, settings: settings, bus: bus, databasePath: databasePath, logger: logger}, nil
}

// Ingest 校验并原子写入一个有界 Metric 批次,随后更新实时总线。
func (m *MetricManager) Ingest(ctx context.Context, samples []model.MetricSample) (model.MetricIngestResult, error) {
	result := model.MetricIngestResult{Received: len(samples), Rejected: len(samples)}
	if len(samples) == 0 || len(samples) > maximumMetricIngestBatch {
		return result, apperror.New(apperror.CodeValidationInvalidArgument, "Metric 批次必须包含 1 到 5000 个样本")
	}
	now, err := m.validateSamples(ctx, samples)
	if err != nil {
		return result, err
	}
	if err := m.ensureCapacity(ctx); err != nil {
		return result, err
	}
	insert := func() error {
		return m.store.MetricsTransaction(ctx, func(registry repository.Registry) error {
			return registry.Metrics().InsertSamples(ctx, samples)
		})
	}
	if err := insert(); err != nil {
		if !isStorageFullError(err) {
			return result, err
		}
		if _, cleanupErr := m.cleanupRetention(ctx, 100); cleanupErr != nil {
			return result, errors.Join(err, cleanupErr)
		}
		if _, cleanupErr := m.cleanupPressure(ctx); cleanupErr != nil {
			return result, errors.Join(err, cleanupErr)
		}
		if retryErr := insert(); retryErr != nil {
			return result, apperror.Wrap(apperror.CodeMetricCapacityExceeded, "Metric 写入失败，清理后数据库或磁盘容量仍不足", retryErr).WithRetryable(true)
		}
	}
	m.publishAccepted(samples)
	result.Accepted = len(samples)
	result.Rejected = 0
	result.AcceptedAt = now
	result.Latest = latestSamplesFromBatch(samples)
	return result, nil
}

func (m *MetricManager) publishAccepted(samples []model.MetricSample) {
	m.bus.Publish(samples)
	if m.observer != nil {
		m.observer.ObserveMetrics(samples)
	}
}

func (m *MetricManager) validateSamples(ctx context.Context, samples []model.MetricSample) (time.Time, error) {
	if len(samples) == 0 || len(samples) > maximumMetricIngestBatch {
		return time.Time{}, apperror.New(apperror.CodeValidationInvalidArgument, "Metric 批次必须包含 1 到 5000 个样本")
	}
	now := m.clock.Now().UTC()
	servers := make(map[model.ID]struct{})
	for index, sample := range samples {
		if err := sample.Validate(now); err != nil {
			return time.Time{}, apperror.Wrap(apperror.CodeMetricCollectionFailed, "Metric 批次校验失败", err).WithDetails(map[string]any{"sampleIndex": index})
		}
		servers[sample.ServerID] = struct{}{}
	}
	for serverID := range servers {
		if _, err := m.store.MinecraftServers().Get(ctx, serverID, false); err != nil {
			return time.Time{}, apperror.Wrap(apperror.CodeMetricCollectionFailed, "Metric 来源 Server 不存在或已删除", err).WithDetails(map[string]any{"serverID": serverID.String()})
		}
	}
	return now, nil
}

// Latest 返回某台 Server 上每个已注册 Metric 的最新值,优先取缓存。
func (m *MetricManager) Latest(ctx context.Context, serverID model.ID) ([]model.MetricSample, error) {
	if !serverID.Valid() {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	if cached := m.bus.Latest(serverID); len(cached) > 0 {
		return cached, nil
	}
	return m.store.Metrics().Latest(ctx, serverID, enums.MetricTypes())
}

// ClearServerHistory 删除某台 Server 的全部原始与聚合 Metric 历史。
func (m *MetricManager) ClearServerHistory(ctx context.Context, serverID model.ID) (int64, error) {
	if !serverID.Valid() {
		return 0, apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	m.maintenanceMu.Lock()
	defer m.maintenanceMu.Unlock()
	deleted, err := m.store.Metrics().DeleteServer(ctx, serverID, metricRetentionBatchSize)
	if err != nil {
		return 0, err
	}
	m.bus.ClearServer(serverID)
	return deleted, nil
}

// StorageStatus 返回当前数据库体积与磁盘可用空间的容量证据。
func (m *MetricManager) StorageStatus(ctx context.Context) (model.MetricStorageStatus, error) {
	settings := m.settings.Snapshot().Monitoring
	usage, err := m.store.Metrics().StorageUsage(ctx)
	if err != nil {
		return model.MetricStorageStatus{}, err
	}
	allocatedBytes, err := metricDatabaseBytes(m.databasePath)
	if err != nil {
		return model.MetricStorageStatus{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取 Metric 数据库大小失败", err)
	}
	available, err := availableDiskBytes(filepath.Dir(m.databasePath))
	if err != nil {
		return model.MetricStorageStatus{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取数据库磁盘可用空间失败", err)
	}
	capacity := int64(settings.DatabaseCapacityMiB) * 1024 * 1024
	minimumFree := int64(settings.MinimumFreeDiskMiB) * 1024 * 1024
	pressure := usage.UsedBytes >= capacity*9/10 || available >= 0 && available < minimumFree
	return model.MetricStorageStatus{
		DatabaseBytes: usage.UsedBytes, AllocatedDatabaseBytes: allocatedBytes,
		CapacityBytes: capacity, AvailableDiskBytes: available,
		MinimumFreeBytes: minimumFree, Pressure: pressure,
	}, nil
}

// Maintain 按热更新的 Settings 周期执行有界的降采样与保留清理。
func (m *MetricManager) Maintain(ctx context.Context) error {
	for {
		interval := time.Duration(m.settings.Snapshot().Monitoring.MaintenanceIntervalSeconds) * time.Second
		if interval < 30*time.Second {
			interval = 30 * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
			if _, err := m.RunMaintenance(ctx); err != nil {
				m.logger.Error(ctx, "Metric 后台维护失败", err, nil)
			}
		}
	}
}

func (m *MetricManager) ensureCapacity(ctx context.Context) error {
	status, err := m.StorageStatus(ctx)
	if err != nil {
		return err
	}
	if status.DatabaseBytes < status.CapacityBytes && (status.AvailableDiskBytes < 0 || status.AvailableDiskBytes >= status.MinimumFreeBytes) {
		return nil
	}
	if _, err := m.cleanupRetention(ctx, 100); err != nil {
		return err
	}
	status, err = m.StorageStatus(ctx)
	if err != nil {
		return err
	}
	if status.DatabaseBytes >= status.CapacityBytes || status.AvailableDiskBytes >= 0 && status.AvailableDiskBytes < status.MinimumFreeBytes {
		if _, err := m.RunMaintenance(ctx); err != nil {
			return err
		}
		status, err = m.StorageStatus(ctx)
		if err != nil {
			return err
		}
	}
	if status.DatabaseBytes >= status.CapacityBytes || status.AvailableDiskBytes >= 0 && status.AvailableDiskBytes < status.MinimumFreeBytes {
		if _, err := m.cleanupPressure(ctx); err != nil {
			return err
		}
		status, err = m.StorageStatus(ctx)
		if err != nil {
			return err
		}
	}
	if status.DatabaseBytes >= status.CapacityBytes || status.AvailableDiskBytes >= 0 && status.AvailableDiskBytes < status.MinimumFreeBytes {
		return apperror.New(apperror.CodeMetricCapacityExceeded, "Metric 数据库或磁盘容量不足").WithDetails(map[string]any{
			"databaseBytes": status.DatabaseBytes, "capacityBytes": status.CapacityBytes,
			"availableDiskBytes": status.AvailableDiskBytes, "minimumFreeBytes": status.MinimumFreeBytes,
		}).WithRetryable(true)
	}
	return nil
}

func metricDatabaseBytes(path string) (int64, error) {
	var total int64
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func isStorageFullError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database or disk is full") || strings.Contains(message, "disk full") || strings.Contains(message, "no space left on device")
}

func latestSamplesFromBatch(samples []model.MetricSample) []model.MetricSample {
	latest := make(map[string]model.MetricSample)
	for _, sample := range samples {
		key := sample.ServerID.String() + "\x00" + metricBusSampleKey(sample)
		if current, found := latest[key]; !found || sample.Timestamp.After(current.Timestamp) {
			latest[key] = cloneMetricSample(sample)
		}
	}
	result := make([]model.MetricSample, 0, len(latest))
	for _, sample := range latest {
		result = append(result, sample)
	}
	return result
}
