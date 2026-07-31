package gormrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MetricSeriesRecord 是 (Server, Source, Metric, Tags) 四元组身份的字典行。
// 原始与聚合表只保留 series_id 小整数外键:四元组文本键(36+36+96+64 字节)原本要在每一行、
// 以及每一条复合索引里各重复存一遍,是 Metric 表体积的主要来源。
type MetricSeriesRecord struct {
	ID            uint32            `gorm:"primaryKey;autoIncrement"`
	ServerID      string            `gorm:"uniqueIndex:idx_metric_series_identity,priority:1;index:idx_metric_series_lookup,priority:1;size:36"`
	SourceID      string            `gorm:"uniqueIndex:idx_metric_series_identity,priority:2;size:36"`
	Metric        string            `gorm:"uniqueIndex:idx_metric_series_identity,priority:3;index:idx_metric_series_lookup,priority:2;size:96"`
	TagsKey       string            `gorm:"uniqueIndex:idx_metric_series_identity,priority:4;size:64"`
	Tags          map[string]string `gorm:"serializer:json"`
	SchemaVersion int
}

// TableName 固定 Metric 序列字典的表名。
func (MetricSeriesRecord) TableName() string { return "metric_series_records" }

// MetricSampleRecord 是一条原始样本:序列外键 + epoch 毫秒 + 数值。
type MetricSampleRecord struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	SeriesID    uint32 `gorm:"index:idx_metric_raw_series_time,priority:1"`
	TimestampMS int64  `gorm:"index:idx_metric_raw_series_time,priority:2;index:idx_metric_raw_time"`
	Value       float64
}

// TableName 让取最新序列的 SQL 与迁移契约保持一致。
func (MetricSampleRecord) TableName() string { return "metric_sample_records" }

// MetricMinuteRecord 是一条分钟聚合,以序列与 epoch 毫秒桶为键。
type MetricMinuteRecord struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	SeriesID uint32 `gorm:"uniqueIndex:idx_metric_minute_unique,priority:1"`
	BucketMS int64  `gorm:"uniqueIndex:idx_metric_minute_unique,priority:2;index:idx_metric_minute_bucket"`
	Count    int64
	Average  float64
	Minimum  float64
	Maximum  float64
	P95      float64
	Latest   float64
}

// TableName 固定分钟聚合的表名。
func (MetricMinuteRecord) TableName() string { return "metric_minute_records" }

// MetricHourRecord 是一条小时聚合,以序列与 epoch 毫秒桶为键。
type MetricHourRecord struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	SeriesID uint32 `gorm:"uniqueIndex:idx_metric_hour_unique,priority:1"`
	BucketMS int64  `gorm:"uniqueIndex:idx_metric_hour_unique,priority:2;index:idx_metric_hour_bucket"`
	Count    int64
	Average  float64
	Minimum  float64
	Maximum  float64
	P95      float64
	Latest   float64
}

// TableName 固定小时聚合的表名。
func (MetricHourRecord) TableName() string { return "metric_hour_records" }

// MetricSeriesCache 把序列身份解析成字典 ID,避免每批写入都回表查询。
type MetricSeriesCache struct {
	mu         sync.RWMutex
	idByKey    map[string]uint32
	recordByID map[uint32]MetricSeriesRecord
}

// NewMetricSeriesCache 创建进程级的 Metric 序列字典缓存。
func NewMetricSeriesCache() *MetricSeriesCache {
	return &MetricSeriesCache{idByKey: make(map[string]uint32), recordByID: make(map[uint32]MetricSeriesRecord)}
}

func (c *MetricSeriesCache) store(record MetricSeriesRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.idByKey[metricSeriesKey(record.ServerID, record.SourceID, record.Metric, record.TagsKey)] = record.ID
	c.recordByID[record.ID] = record
}

func (c *MetricSeriesCache) lookupID(key string) (uint32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, found := c.idByKey[key]
	return id, found
}

func (c *MetricSeriesCache) lookupRecord(id uint32) (MetricSeriesRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	record, found := c.recordByID[id]
	return record, found
}

func (c *MetricSeriesCache) evict(ids []uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		if record, found := c.recordByID[id]; found {
			delete(c.idByKey, metricSeriesKey(record.ServerID, record.SourceID, record.Metric, record.TagsKey))
			delete(c.recordByID, id)
		}
	}
}

type metricRepository struct {
	database *gorm.DB
	series   *MetricSeriesCache
}

// InsertSamples 解析每条样本的序列身份,并追加一个有界的原始样本批次。
func (r *metricRepository) InsertSamples(ctx context.Context, samples []model.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	records := make([]MetricSampleRecord, len(samples))
	for index, sample := range samples {
		seriesID, err := r.resolveSeries(ctx, sample.ServerID.String(), sample.SourceID.String(), sample.Metric.String(), sample.Tags, sample.SchemaVersion)
		if err != nil {
			return err
		}
		records[index] = MetricSampleRecord{SeriesID: seriesID, TimestampMS: sample.Timestamp.UTC().UnixMilli(), Value: sample.Value}
	}
	if err := r.database.WithContext(ctx).CreateInBatches(records, 500).Error; err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "批量写入 Metric Samples 失败", err)
	}
	return nil
}

// resolveSeries 返回某个身份对应的字典 ID,首次出现时插入新行。
func (r *metricRepository) resolveSeries(ctx context.Context, serverID, sourceID, metric string, tags map[string]string, schemaVersion int) (uint32, error) {
	tagsKey := metricTagsKey(tags)
	key := metricSeriesKey(serverID, sourceID, metric, tagsKey)
	if id, found := r.series.lookupID(key); found {
		return id, nil
	}
	record := MetricSeriesRecord{
		ServerID: serverID, SourceID: sourceID, Metric: metric, TagsKey: tagsKey,
		Tags: cloneMetricTags(tags), SchemaVersion: schemaVersion,
	}
	// DoNothing 冲突后 RowsAffected 为 0 且不回填主键,并发首见同一序列时必须再查一次拿 ID。
	if err := r.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "server_id"}, {Name: "source_id"}, {Name: "metric"}, {Name: "tags_key"}},
		DoNothing: true,
	}).Create(&record).Error; err != nil {
		return 0, apperror.Wrap(apperror.CodeMetricCollectionFailed, "写入 Metric 序列字典失败", err)
	}
	if record.ID == 0 {
		if err := r.database.WithContext(ctx).
			Where("server_id = ? AND source_id = ? AND metric = ? AND tags_key = ?", serverID, sourceID, metric, tagsKey).
			First(&record).Error; err != nil {
			return 0, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Metric 序列字典失败", err)
		}
	}
	r.series.store(record)
	return record.ID, nil
}

// seriesForQuery 返回匹配 Server、Metric 与可选 Source 过滤条件的字典行。
func (r *metricRepository) seriesForQuery(ctx context.Context, serverID model.ID, metrics []string, sourceID model.ID) ([]MetricSeriesRecord, error) {
	database := r.database.WithContext(ctx).Where("server_id = ?", serverID.String())
	if len(metrics) > 0 {
		database = database.Where("metric IN ?", metrics)
	}
	if sourceID.Valid() {
		database = database.Where("source_id = ?", sourceID.String())
	}
	var records []MetricSeriesRecord
	if err := database.Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Metric 序列字典失败", err)
	}
	for _, record := range records {
		r.series.store(record)
	}
	return records, nil
}

// ListSamples 返回某个 Server、Metric 与可选 Source 在时间区间内的原始样本。
func (r *metricRepository) ListSamples(ctx context.Context, query model.MetricQuery) ([]model.MetricSample, error) {
	series, err := r.seriesForQuery(ctx, query.ServerID, []string{query.Metric.String()}, query.SourceID)
	if err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, nil
	}
	database := r.database.WithContext(ctx).
		Where("series_id IN ? AND timestamp_ms >= ? AND timestamp_ms <= ?", metricSeriesIDs(series), query.Start.UTC().UnixMilli(), query.End.UTC().UnixMilli()).
		Order("timestamp_ms asc")
	database = applyMetricPagination(database, query.Limit, query.Offset)
	var records []MetricSampleRecord
	if err := database.Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Metric Samples 失败", err)
	}
	return r.samplesFromRecords(records), nil
}

// ListSamplesRange 用游标分页返回全部序列的原始样本,供降采样使用。
func (r *metricRepository) ListSamplesRange(ctx context.Context, start, end time.Time, cursor repository.MetricSampleCursor, limit int) ([]model.MetricSample, repository.MetricSampleCursor, error) {
	if limit <= 0 || limit > 100_000 {
		limit = 100_000
	}
	var records []MetricSampleRecord
	if err := r.database.WithContext(ctx).
		Where("timestamp_ms >= ? AND timestamp_ms < ? AND (timestamp_ms, id) > (?, ?)", start.UTC().UnixMilli(), end.UTC().UnixMilli(), cursor.Time.UTC().UnixMilli(), cursor.ID).
		Order("timestamp_ms asc, id asc").Limit(limit).Find(&records).Error; err != nil {
		return nil, cursor, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询降采样原始 Metric 失败", err)
	}
	if err := r.warmSeries(ctx, records); err != nil {
		return nil, cursor, err
	}
	next := cursor
	if len(records) > 0 {
		last := records[len(records)-1]
		next = repository.MetricSampleCursor{Time: time.UnixMilli(last.TimestampMS).UTC(), ID: last.ID}
	}
	return r.samplesFromRecords(records), next, nil
}

// UpsertAggregates 按序列与桶键幂等写入分钟和小时聚合。
func (r *metricRepository) UpsertAggregates(ctx context.Context, aggregates []model.MetricAggregate) error {
	if len(aggregates) == 0 {
		return nil
	}
	minutes := make([]MetricMinuteRecord, 0, len(aggregates))
	hours := make([]MetricHourRecord, 0, len(aggregates))
	for _, aggregate := range aggregates {
		seriesID, err := r.resolveSeries(ctx, aggregate.ServerID.String(), aggregate.SourceID.String(), aggregate.Metric.String(), aggregate.Tags, aggregate.SchemaVersion)
		if err != nil {
			return err
		}
		bucket := aggregate.Bucket.UTC().UnixMilli()
		switch aggregate.Granularity {
		case enums.MetricGranularityMinute:
			minutes = append(minutes, MetricMinuteRecord{
				SeriesID: seriesID, BucketMS: bucket, Count: aggregate.Count, Average: aggregate.Average,
				Minimum: aggregate.Minimum, Maximum: aggregate.Maximum, P95: aggregate.P95, Latest: aggregate.Latest,
			})
		case enums.MetricGranularityHour:
			hours = append(hours, MetricHourRecord{
				SeriesID: seriesID, BucketMS: bucket, Count: aggregate.Count, Average: aggregate.Average,
				Minimum: aggregate.Minimum, Maximum: aggregate.Maximum, P95: aggregate.P95, Latest: aggregate.Latest,
			})
		}
	}
	updates := clause.OnConflict{
		Columns:   []clause.Column{{Name: "series_id"}, {Name: "bucket_ms"}},
		DoUpdates: clause.AssignmentColumns([]string{"count", "average", "minimum", "maximum", "p95", "latest"}),
	}
	if len(minutes) > 0 {
		if err := r.database.WithContext(ctx).Clauses(updates).CreateInBatches(minutes, 500).Error; err != nil {
			return apperror.Wrap(apperror.CodeMetricCollectionFailed, "写入 Minute Metric Aggregates 失败", err)
		}
	}
	if len(hours) > 0 {
		if err := r.database.WithContext(ctx).Clauses(updates).CreateInBatches(hours, 500).Error; err != nil {
			return apperror.Wrap(apperror.CodeMetricCollectionFailed, "写入 Hour Metric Aggregates 失败", err)
		}
	}
	return nil
}

// ListAggregates 返回某个 Server 与 Metric 在时间区间内的分钟或小时聚合。
func (r *metricRepository) ListAggregates(ctx context.Context, query model.MetricQuery) ([]model.MetricAggregate, error) {
	series, err := r.seriesForQuery(ctx, query.ServerID, []string{query.Metric.String()}, query.SourceID)
	if err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, nil
	}
	database := r.database.WithContext(ctx).
		Where("series_id IN ? AND bucket_ms >= ? AND bucket_ms <= ?", metricSeriesIDs(series), query.Start.UTC().UnixMilli(), query.End.UTC().UnixMilli()).
		Order("bucket_ms asc")
	database = applyMetricPagination(database, query.Limit, query.Offset)
	result := make([]model.MetricAggregate, 0)
	if query.Granularity == enums.MetricGranularityMinute {
		var rows []MetricMinuteRecord
		if err := database.Find(&rows).Error; err != nil {
			return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Minute Metric Aggregates 失败", err)
		}
		for _, row := range rows {
			result = append(result, r.aggregateFromRow(row.SeriesID, row.BucketMS, row.Count, row.Average, row.Minimum, row.Maximum, row.P95, row.Latest, query.Granularity))
		}
		return result, nil
	}
	var rows []MetricHourRecord
	if err := database.Find(&rows).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Hour Metric Aggregates 失败", err)
	}
	for _, row := range rows {
		result = append(result, r.aggregateFromRow(row.SeriesID, row.BucketMS, row.Count, row.Average, row.Minimum, row.Maximum, row.P95, row.Latest, query.Granularity))
	}
	return result, nil
}

// Latest 返回某个 Server 下每个序列的最新一条样本。
func (r *metricRepository) Latest(ctx context.Context, serverID model.ID, metrics []enums.MetricType) ([]model.MetricSample, error) {
	if len(metrics) == 0 {
		return nil, nil
	}
	metricNames := make([]string, len(metrics))
	for index, metric := range metrics {
		metricNames[index] = metric.String()
	}
	series, err := r.seriesForQuery(ctx, serverID, metricNames, model.ID(""))
	if err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, nil
	}
	// 每个序列取最新一行:先在 (series_id, timestamp_ms) 索引上分组求 max 拿到 rowid,再按主键回表。
	// SQLite 保证聚合查询中的裸列(此处 id)取自命中 max() 的那一行,因此 Select 必须保留 max(timestamp_ms)。
	var heads []struct {
		ID uint64 `gorm:"column:id"`
	}
	if err := r.database.WithContext(ctx).Model(&MetricSampleRecord{}).
		Select("id, max(timestamp_ms)").
		Where("series_id IN ?", metricSeriesIDs(series)).
		Group("series_id").
		Scan(&heads).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询最新 Metric 序列失败", err)
	}
	if len(heads) == 0 {
		return nil, nil
	}
	latestIDs := make([]uint64, len(heads))
	for index, head := range heads {
		latestIDs[index] = head.ID
	}
	var records []MetricSampleRecord
	if err := r.database.WithContext(ctx).Where("id IN ?", latestIDs).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询最新 Metric 失败", err)
	}
	samples := r.samplesFromRecords(records)
	sortMetricSamples(samples)
	return samples, nil
}

// DeleteBefore 删除早于保留期界线的一批数据,单次批量有上限。
func (r *metricRepository) DeleteBefore(ctx context.Context, granularity enums.MetricGranularity, before time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 50_000 {
		limit = 10_000
	}
	modelValue := any(&MetricSampleRecord{})
	timeColumn := "timestamp_ms"
	switch granularity {
	case enums.MetricGranularityMinute:
		modelValue, timeColumn = &MetricMinuteRecord{}, "bucket_ms"
	case enums.MetricGranularityHour:
		modelValue, timeColumn = &MetricHourRecord{}, "bucket_ms"
	}
	var ids []uint64
	if err := r.database.WithContext(ctx).Model(modelValue).Where(timeColumn+" < ?", before.UTC().UnixMilli()).Order(timeColumn+" asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询待清理 Metric 失败", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.database.WithContext(ctx).Delete(modelValue, "id IN ?", ids)
	if result.Error != nil {
		return 0, apperror.Wrap(apperror.CodeIOWriteFailed, "清理 Metric 数据失败", result.Error)
	}
	return result.RowsAffected, nil
}

// StorageUsage 返回监控数据库的 SQLite 页使用量,供容量判定使用。
func (r *metricRepository) StorageUsage(ctx context.Context) (repository.MetricStorageUsage, error) {
	var pageSize, pageCount, freePages int64
	for _, query := range []struct {
		statement string
		target    *int64
	}{
		{"PRAGMA page_size", &pageSize},
		{"PRAGMA page_count", &pageCount},
		{"PRAGMA freelist_count", &freePages},
	} {
		if err := r.database.WithContext(ctx).Raw(query.statement).Scan(query.target).Error; err != nil {
			return repository.MetricStorageUsage{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取 Metric SQLite 容量信息失败", err)
		}
	}
	usedPages := pageCount - freePages
	if usedPages < 0 {
		usedPages = 0
	}
	return repository.MetricStorageUsage{UsedBytes: usedPages * pageSize}, nil
}

// DeleteServer 删除某个 Server 的全部样本、聚合与序列字典行。
func (r *metricRepository) DeleteServer(ctx context.Context, serverID model.ID, batchSize int) (int64, error) {
	if !serverID.Valid() {
		return 0, apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	if batchSize <= 0 || batchSize > 50_000 {
		batchSize = 10_000
	}
	series, err := r.seriesForQuery(ctx, serverID, nil, model.ID(""))
	if err != nil {
		return 0, err
	}
	if len(series) == 0 {
		return 0, nil
	}
	seriesIDs := metricSeriesIDs(series)
	var total int64
	for _, modelValue := range []any{&MetricSampleRecord{}, &MetricMinuteRecord{}, &MetricHourRecord{}} {
		for {
			var ids []uint64
			if err := r.database.WithContext(ctx).Model(modelValue).Where("series_id IN ?", seriesIDs).Limit(batchSize).Pluck("id", &ids).Error; err != nil {
				return total, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询待删除 Server Metric 失败", err)
			}
			if len(ids) == 0 {
				break
			}
			result := r.database.WithContext(ctx).Delete(modelValue, "id IN ?", ids)
			if result.Error != nil {
				return total, apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Server Metric 历史失败", result.Error)
			}
			total += result.RowsAffected
			if len(ids) < batchSize {
				break
			}
		}
	}
	if err := r.database.WithContext(ctx).Delete(&MetricSeriesRecord{}, "id IN ?", seriesIDs).Error; err != nil {
		return total, apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Server Metric 序列字典失败", err)
	}
	r.series.evict(seriesIDs)
	return total, nil
}

// warmSeries 补载记录引用到、但缓存中缺失的序列字典行。
func (r *metricRepository) warmSeries(ctx context.Context, records []MetricSampleRecord) error {
	missing := make([]uint32, 0)
	seen := make(map[uint32]struct{})
	for _, record := range records {
		if _, found := seen[record.SeriesID]; found {
			continue
		}
		seen[record.SeriesID] = struct{}{}
		if _, cached := r.series.lookupRecord(record.SeriesID); !cached {
			missing = append(missing, record.SeriesID)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var series []MetricSeriesRecord
	if err := r.database.WithContext(ctx).Where("id IN ?", missing).Find(&series).Error; err != nil {
		return apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Metric 序列字典失败", err)
	}
	for _, record := range series {
		r.series.store(record)
	}
	return nil
}

func (r *metricRepository) samplesFromRecords(records []MetricSampleRecord) []model.MetricSample {
	result := make([]model.MetricSample, 0, len(records))
	for _, record := range records {
		series, found := r.series.lookupRecord(record.SeriesID)
		if !found {
			continue
		}
		definition, _ := model.DefinitionForMetric(enums.MetricType(series.Metric))
		result = append(result, model.MetricSample{
			ServerID: model.ID(series.ServerID), SourceID: model.ID(series.SourceID), Metric: enums.MetricType(series.Metric),
			Unit: definition.Unit, Timestamp: time.UnixMilli(record.TimestampMS).UTC(), Value: record.Value,
			Tags: cloneMetricTags(series.Tags), SchemaVersion: series.SchemaVersion,
		})
	}
	return result
}

func (r *metricRepository) aggregateFromRow(seriesID uint32, bucketMS int64, count int64, average, minimum, maximum, p95, latest float64, granularity enums.MetricGranularity) model.MetricAggregate {
	series, _ := r.series.lookupRecord(seriesID)
	return model.MetricAggregate{
		ServerID: model.ID(series.ServerID), SourceID: model.ID(series.SourceID), Metric: enums.MetricType(series.Metric),
		Bucket: time.UnixMilli(bucketMS).UTC(), Granularity: granularity, Count: count, Average: average,
		Minimum: minimum, Maximum: maximum, P95: p95, Latest: latest,
		Tags: cloneMetricTags(series.Tags), SchemaVersion: series.SchemaVersion,
	}
}

func metricSeriesIDs(series []MetricSeriesRecord) []uint32 {
	ids := make([]uint32, len(series))
	for index, record := range series {
		ids[index] = record.ID
	}
	return ids
}

func metricSeriesKey(serverID, sourceID, metric, tagsKey string) string {
	return serverID + "\x00" + sourceID + "\x00" + metric + "\x00" + tagsKey
}

func sortMetricSamples(samples []model.MetricSample) {
	sort.Slice(samples, func(left, right int) bool {
		if samples[left].Metric != samples[right].Metric {
			return samples[left].Metric < samples[right].Metric
		}
		return samples[left].SourceID < samples[right].SourceID
	})
}

func applyMetricPagination(database *gorm.DB, limit, offset int) *gorm.DB {
	if limit <= 0 || limit > 100_000 {
		limit = 10_000
	}
	if offset < 0 {
		offset = 0
	}
	return database.Limit(limit).Offset(offset)
}

func metricTagsKey(tags map[string]string) string {
	payload, _ := json.Marshal(tags)
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func cloneMetricTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	result := make(map[string]string, len(tags))
	for key, value := range tags {
		result[key] = value
	}
	return result
}
