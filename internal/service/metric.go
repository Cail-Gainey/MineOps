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

// MetricObserver receives accepted persisted samples without owning the ingest transaction.
type MetricObserver interface {
	ObserveMetrics([]model.MetricSample)
}

// MetricManager coordinates atomic ingest, realtime cache, queries, rollups, retention, and capacity protection.
type MetricManager struct {
	clock        model.Clock
	store        repository.Store
	settings     *appsettings.Manager
	bus          *MetricBus
	databasePath string
	logger       *applog.Logger
	observer     MetricObserver

	maintenanceMu sync.Mutex
}

// SetObserver installs the bounded post-persistence metric observer used by threshold evaluation.
func (m *MetricManager) SetObserver(observer MetricObserver) {
	if m != nil {
		m.observer = observer
	}
}

// NewMetricManager creates the stage 10 Metric application service.
func NewMetricManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, bus *MetricBus, databasePath string, logger *applog.Logger) (*MetricManager, error) {
	if clock == nil || store == nil || settings == nil || bus == nil || strings.TrimSpace(databasePath) == "" {
		return nil, apperror.New(apperror.CodeValidationRequired, "Metric 依赖和数据库路径不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &MetricManager{clock: clock, store: store, settings: settings, bus: bus, databasePath: databasePath, logger: logger}, nil
}

// Ingest validates and atomically persists one bounded Metric batch before updating the realtime bus.
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
		return m.store.Transaction(ctx, func(registry repository.Registry) error {
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

// Latest returns cached or persisted latest values for every registered Metric on one Server.
func (m *MetricManager) Latest(ctx context.Context, serverID model.ID) ([]model.MetricSample, error) {
	if !serverID.Valid() {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	if cached := m.bus.Latest(serverID); len(cached) > 0 {
		return cached, nil
	}
	return m.store.Metrics().Latest(ctx, serverID, enums.MetricTypes())
}

// ClearServerHistory deletes all raw and aggregate Metric history for one Server.
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

// StorageStatus returns current database and free-disk capacity evidence.
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

// Maintain runs bounded rollup and retention passes at the hot-reloaded Settings interval.
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
