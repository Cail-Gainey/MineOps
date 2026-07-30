package service

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

const (
	metricMaintenancePageSize = 50_000
	metricRetentionBatchSize  = 10_000
)

type metricAggregateAccumulator struct {
	serverID      model.ID
	sourceID      model.ID
	metric        enums.MetricType
	bucket        time.Time
	tags          map[string]string
	count         int64
	sum           float64
	minimum       float64
	maximum       float64
	latest        float64
	latestTime    time.Time
	values        []float64
	schemaVersion int
}

// RunMaintenance performs one mutually exclusive bounded downsample and retention pass.
func (m *MetricManager) RunMaintenance(ctx context.Context) (model.MetricMaintenanceResult, error) {
	m.maintenanceMu.Lock()
	defer m.maintenanceMu.Unlock()
	now := m.clock.Now().UTC()
	minuteAggregates, err := m.downsampleGranularity(ctx, now, enums.MetricGranularityMinute)
	if err != nil {
		return model.MetricMaintenanceResult{}, err
	}
	hourAggregates, err := m.downsampleGranularity(ctx, now, enums.MetricGranularityHour)
	if err != nil {
		return model.MetricMaintenanceResult{}, err
	}
	deleted, err := m.cleanupRetention(ctx, 10)
	if err != nil {
		return model.MetricMaintenanceResult{}, err
	}
	deleted.MinuteAggregates = minuteAggregates
	deleted.HourAggregates = hourAggregates
	return deleted, nil
}

// downsampleGranularity rolls up only buckets completed since the last watermark, with one-step overlap for stragglers.
func (m *MetricManager) downsampleGranularity(ctx context.Context, now time.Time, granularity enums.MetricGranularity) (int, error) {
	step, lookback, watermark := time.Minute, 2*time.Hour, &m.minuteRolledUpTo
	if granularity == enums.MetricGranularityHour {
		step, lookback, watermark = time.Hour, 48*time.Hour, &m.hourRolledUpTo
	}
	end := now.Truncate(step)
	if !watermark.Before(end) {
		return 0, nil
	}
	start := end.Add(-lookback)
	if watermark.After(start) {
		start = watermark.Add(-step)
	}
	aggregates, err := m.downsampleRange(ctx, start, end, granularity)
	if err != nil {
		return 0, err
	}
	*watermark = end
	return aggregates, nil
}

func (m *MetricManager) downsampleRange(ctx context.Context, start, end time.Time, granularity enums.MetricGranularity) (int, error) {
	if !start.Before(end) || granularity != enums.MetricGranularityMinute && granularity != enums.MetricGranularityHour {
		return 0, nil
	}
	step := time.Minute
	if granularity == enums.MetricGranularityHour {
		step = time.Hour
	}
	groups := make(map[string]*metricAggregateAccumulator)
	cursor := repository.MetricSampleCursor{}
	for {
		samples, nextCursor, err := m.store.Metrics().ListSamplesRange(ctx, start, end, cursor, metricMaintenancePageSize)
		if err != nil {
			return 0, err
		}
		cursor = nextCursor
		for _, sample := range samples {
			bucket := sample.Timestamp.UTC().Truncate(step)
			key := sample.ServerID.String() + "\x00" + metricBusSampleKey(sample) + "\x00" + bucket.Format(time.RFC3339Nano)
			group := groups[key]
			if group == nil {
				group = &metricAggregateAccumulator{
					serverID: sample.ServerID, sourceID: sample.SourceID, metric: sample.Metric, bucket: bucket,
					tags: cloneMetricSample(sample).Tags, minimum: sample.Value, maximum: sample.Value,
					latest: sample.Value, latestTime: sample.Timestamp, schemaVersion: sample.SchemaVersion,
				}
				groups[key] = group
			}
			group.count++
			group.sum += sample.Value
			group.minimum = math.Min(group.minimum, sample.Value)
			group.maximum = math.Max(group.maximum, sample.Value)
			if sample.Timestamp.After(group.latestTime) || sample.Timestamp.Equal(group.latestTime) {
				group.latest, group.latestTime = sample.Value, sample.Timestamp
			}
			group.values = append(group.values, sample.Value)
		}
		if len(samples) < metricMaintenancePageSize {
			break
		}
	}
	aggregates := make([]model.MetricAggregate, 0, len(groups))
	for _, group := range groups {
		sort.Float64s(group.values)
		p95Index := int(math.Ceil(float64(len(group.values))*0.95)) - 1
		if p95Index < 0 {
			p95Index = 0
		}
		aggregates = append(aggregates, model.MetricAggregate{
			ServerID: group.serverID, SourceID: group.sourceID, Metric: group.metric, Bucket: group.bucket,
			Granularity: granularity, Count: group.count, Average: group.sum / float64(group.count),
			Minimum: group.minimum, Maximum: group.maximum, P95: group.values[p95Index], Latest: group.latest,
			Tags: group.tags, SchemaVersion: group.schemaVersion,
		})
	}
	if err := m.store.Metrics().UpsertAggregates(ctx, aggregates); err != nil {
		return 0, apperror.Wrap(apperror.CodeMetricCollectionFailed, "Metric 降采样写入失败", err)
	}
	return len(aggregates), nil
}

func (m *MetricManager) cleanupRetention(ctx context.Context, maximumBatches int) (model.MetricMaintenanceResult, error) {
	if maximumBatches <= 0 {
		maximumBatches = 1
	}
	settings := m.settings.Snapshot().Monitoring
	now := m.clock.Now().UTC()
	result := model.MetricMaintenanceResult{}
	policies := []struct {
		granularity enums.MetricGranularity
		before      time.Time
		total       *int64
	}{
		{enums.MetricGranularityRaw, now.AddDate(0, 0, -settings.RawRetentionDays), &result.RawDeleted},
		{enums.MetricGranularityMinute, now.AddDate(0, 0, -settings.MinuteRetentionDays), &result.MinuteDeleted},
		{enums.MetricGranularityHour, now.AddDate(0, 0, -settings.HourRetentionDays), &result.HourDeleted},
	}
	for _, policy := range policies {
		for range maximumBatches {
			deleted, err := m.store.Metrics().DeleteBefore(ctx, policy.granularity, policy.before, metricRetentionBatchSize)
			if err != nil {
				return result, err
			}
			*policy.total += deleted
			if deleted < metricRetentionBatchSize {
				break
			}
		}
	}
	for range maximumBatches {
		deleted, err := m.store.Spark().DeleteSnapshotsBefore(ctx, now.AddDate(0, 0, -settings.RawRetentionDays), metricRetentionBatchSize)
		if err != nil {
			return result, err
		}
		if deleted < metricRetentionBatchSize {
			break
		}
	}
	return result, nil
}

func (m *MetricManager) cleanupPressure(ctx context.Context) (model.MetricMaintenanceResult, error) {
	now := m.clock.Now().UTC()
	result := model.MetricMaintenanceResult{}
	policies := []struct {
		granularity enums.MetricGranularity
		before      time.Time
		total       *int64
	}{
		{enums.MetricGranularityRaw, now.Add(-time.Hour), &result.RawDeleted},
		{enums.MetricGranularityMinute, now.AddDate(0, 0, -7), &result.MinuteDeleted},
		{enums.MetricGranularityHour, now.AddDate(0, 0, -30), &result.HourDeleted},
	}
	for _, policy := range policies {
		for range 100 {
			deleted, err := m.store.Metrics().DeleteBefore(ctx, policy.granularity, policy.before, metricRetentionBatchSize)
			if err != nil {
				return result, err
			}
			*policy.total += deleted
			if deleted < metricRetentionBatchSize {
				break
			}
		}
	}
	for range 100 {
		deleted, err := m.store.Spark().DeleteSnapshotsBefore(ctx, now.Add(-time.Hour), metricRetentionBatchSize)
		if err != nil {
			return result, err
		}
		if deleted < metricRetentionBatchSize {
			break
		}
	}
	return result, nil
}
