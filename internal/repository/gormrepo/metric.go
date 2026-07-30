package gormrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MetricSampleRecord is the indexed raw metric representation.
type MetricSampleRecord struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	ServerID      string    `gorm:"index:idx_metric_raw_query,priority:1;index:idx_metric_raw_series,priority:1;size:36"`
	SourceID      string    `gorm:"index:idx_metric_raw_source,priority:1;index:idx_metric_raw_series,priority:2;size:36"`
	Metric        string    `gorm:"index:idx_metric_raw_query,priority:2;index:idx_metric_raw_source,priority:2;index:idx_metric_raw_series,priority:3;size:96"`
	Timestamp     time.Time `gorm:"index:idx_metric_raw_query,priority:3;index:idx_metric_raw_source,priority:3;index:idx_metric_raw_series,priority:5;index:idx_metric_raw_time"`
	Value         float64
	TagsKey       string            `gorm:"index:idx_metric_raw_series,priority:4;size:64"`
	Tags          map[string]string `gorm:"serializer:json"`
	SchemaVersion int
}

// TableName keeps latest-series SQL aligned with the migration contract.
func (MetricSampleRecord) TableName() string { return "metric_sample_records" }

// MetricMinuteRecord is one minute aggregate row with minute-specific SQLite indexes.
type MetricMinuteRecord struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	ServerID      string    `gorm:"uniqueIndex:idx_metric_minute_unique,priority:1;index:idx_metric_minute_query,priority:1;size:36"`
	SourceID      string    `gorm:"uniqueIndex:idx_metric_minute_unique,priority:2;size:36"`
	Metric        string    `gorm:"uniqueIndex:idx_metric_minute_unique,priority:3;index:idx_metric_minute_query,priority:2;size:96"`
	Bucket        time.Time `gorm:"uniqueIndex:idx_metric_minute_unique,priority:4;index:idx_metric_minute_query,priority:3;index:idx_metric_minute_bucket"`
	TagsKey       string    `gorm:"uniqueIndex:idx_metric_minute_unique,priority:5;size:64"`
	Count         int64
	Average       float64
	Minimum       float64
	Maximum       float64
	P95           float64
	Latest        float64
	Tags          map[string]string `gorm:"serializer:json"`
	SchemaVersion int
}

// MetricHourRecord is one hour aggregate row with hour-specific SQLite indexes.
type MetricHourRecord struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	ServerID      string    `gorm:"uniqueIndex:idx_metric_hour_unique,priority:1;index:idx_metric_hour_query,priority:1;size:36"`
	SourceID      string    `gorm:"uniqueIndex:idx_metric_hour_unique,priority:2;size:36"`
	Metric        string    `gorm:"uniqueIndex:idx_metric_hour_unique,priority:3;index:idx_metric_hour_query,priority:2;size:96"`
	Bucket        time.Time `gorm:"uniqueIndex:idx_metric_hour_unique,priority:4;index:idx_metric_hour_query,priority:3;index:idx_metric_hour_bucket"`
	TagsKey       string    `gorm:"uniqueIndex:idx_metric_hour_unique,priority:5;size:64"`
	Count         int64
	Average       float64
	Minimum       float64
	Maximum       float64
	P95           float64
	Latest        float64
	Tags          map[string]string `gorm:"serializer:json"`
	SchemaVersion int
}

type metricAggregateRecord struct {
	ServerID      string
	SourceID      string
	Metric        string
	Bucket        time.Time
	TagsKey       string
	Count         int64
	Average       float64
	Minimum       float64
	Maximum       float64
	P95           float64
	Latest        float64
	Tags          map[string]string
	SchemaVersion int
}

type metricRepository struct{ database *gorm.DB }

func (r *metricRepository) InsertSamples(ctx context.Context, samples []model.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	records := make([]MetricSampleRecord, len(samples))
	for index, sample := range samples {
		records[index] = MetricSampleRecord{
			ServerID: sample.ServerID.String(), SourceID: sample.SourceID.String(), Metric: sample.Metric.String(),
			Timestamp: sample.Timestamp.UTC(), Value: sample.Value, TagsKey: metricTagsKey(sample.Tags),
			Tags: cloneMetricTags(sample.Tags), SchemaVersion: sample.SchemaVersion,
		}
	}
	if err := r.database.WithContext(ctx).CreateInBatches(records, 500).Error; err != nil {
		return apperror.Wrap(apperror.CodeMetricCollectionFailed, "批量写入 Metric Samples 失败", err)
	}
	return nil
}

func (r *metricRepository) ListSamples(ctx context.Context, query model.MetricQuery) ([]model.MetricSample, error) {
	database := r.database.WithContext(ctx).Where("server_id = ? AND metric = ? AND timestamp >= ? AND timestamp <= ?", query.ServerID.String(), query.Metric.String(), query.Start.UTC(), query.End.UTC()).Order("timestamp asc")
	if query.SourceID.Valid() {
		database = database.Where("source_id = ?", query.SourceID.String())
	}
	database = applyMetricPagination(database, query.Limit, query.Offset)
	var records []MetricSampleRecord
	if err := database.Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Metric Samples 失败", err)
	}
	result := make([]model.MetricSample, len(records))
	for index, record := range records {
		result[index] = recordToMetricSample(record)
	}
	return result, nil
}

func (r *metricRepository) ListSamplesRange(ctx context.Context, start, end time.Time, cursor repository.MetricSampleCursor, limit int) ([]model.MetricSample, repository.MetricSampleCursor, error) {
	if limit <= 0 || limit > 100_000 {
		limit = 100_000
	}
	var records []MetricSampleRecord
	if err := r.database.WithContext(ctx).
		Where("timestamp >= ? AND timestamp < ? AND (timestamp, id) > (?, ?)", start.UTC(), end.UTC(), cursor.Time.UTC(), cursor.ID).
		Order("timestamp asc, id asc").Limit(limit).Find(&records).Error; err != nil {
		return nil, cursor, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询降采样原始 Metric 失败", err)
	}
	next := cursor
	result := make([]model.MetricSample, len(records))
	for index, record := range records {
		result[index] = recordToMetricSample(record)
		next = repository.MetricSampleCursor{Time: record.Timestamp, ID: record.ID}
	}
	return result, next, nil
}

func (r *metricRepository) UpsertAggregates(ctx context.Context, aggregates []model.MetricAggregate) error {
	if len(aggregates) == 0 {
		return nil
	}
	minutes := make([]MetricMinuteRecord, 0, len(aggregates))
	hours := make([]MetricHourRecord, 0, len(aggregates))
	for _, aggregate := range aggregates {
		record := aggregateToRecord(aggregate)
		switch aggregate.Granularity {
		case enums.MetricGranularityMinute:
			minutes = append(minutes, minuteRecordFromAggregate(record))
		case enums.MetricGranularityHour:
			hours = append(hours, hourRecordFromAggregate(record))
		}
	}
	updates := clause.OnConflict{
		Columns:   []clause.Column{{Name: "server_id"}, {Name: "source_id"}, {Name: "metric"}, {Name: "bucket"}, {Name: "tags_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"count", "average", "minimum", "maximum", "p95", "latest", "tags", "schema_version"}),
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

func (r *metricRepository) ListAggregates(ctx context.Context, query model.MetricQuery) ([]model.MetricAggregate, error) {
	database := r.database.WithContext(ctx).Where("server_id = ? AND metric = ? AND bucket >= ? AND bucket <= ?", query.ServerID.String(), query.Metric.String(), query.Start.UTC(), query.End.UTC()).Order("bucket asc")
	if query.SourceID.Valid() {
		database = database.Where("source_id = ?", query.SourceID.String())
	}
	database = applyMetricPagination(database, query.Limit, query.Offset)
	var records []metricAggregateRecord
	if query.Granularity == enums.MetricGranularityMinute {
		var rows []MetricMinuteRecord
		if err := database.Find(&rows).Error; err != nil {
			return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Minute Metric Aggregates 失败", err)
		}
		for _, row := range rows {
			records = append(records, aggregateFromMinuteRecord(row))
		}
	} else {
		var rows []MetricHourRecord
		if err := database.Find(&rows).Error; err != nil {
			return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询 Hour Metric Aggregates 失败", err)
		}
		for _, row := range rows {
			records = append(records, aggregateFromHourRecord(row))
		}
	}
	result := make([]model.MetricAggregate, len(records))
	for index, record := range records {
		result[index] = recordToMetricAggregate(record, query.Granularity)
	}
	return result, nil
}

func (r *metricRepository) Latest(ctx context.Context, serverID model.ID, metrics []enums.MetricType) ([]model.MetricSample, error) {
	if len(metrics) == 0 {
		return nil, nil
	}
	metricNames := make([]string, len(metrics))
	for index, metric := range metrics {
		metricNames[index] = metric.String()
	}
	// 每个序列只取最新一行:先在 idx_metric_raw_series 覆盖索引上按序列分组求 max(timestamp) 拿到 rowid,
	// 再按主键回表取整行。SQLite 保证聚合查询中的裸列(此处 id)取自命中 max() 的那一行,
	// 因此 Select 里必须保留 max(timestamp)。旧写法用 NOT EXISTS 关联子查询,外层每一行都要再探一次索引并回表;
	// 250 万行同构库实测 CPU 3.14s,改写后 0.45s(SQLCipher 逐页解密 + HMAC 下差距更大,且此处只扫覆盖索引不碰表页)。
	// 唯一行为差异:同一序列存在时间戳完全相同的重复样本时,旧写法取 id 更大的一行,这里由 SQLite 任选其一。
	// 等价保留 id 次序需要 row_number() 窗口函数,但要为分区排序建临时 B 树,同库实测 CPU 1.23s,不值得。
	var heads []struct {
		ID uint64 `gorm:"column:id"`
	}
	if err := r.database.WithContext(ctx).Model(&MetricSampleRecord{}).
		Select("id, max(timestamp)").
		Where("server_id = ? AND metric IN ?", serverID.String(), metricNames).
		Group("source_id, metric, tags_key").
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
	if err := r.database.WithContext(ctx).Where("id IN ?", latestIDs).
		Order("metric asc, source_id asc, tags_key asc").Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeMetricQueryFailed, "查询最新 Metric 失败", err)
	}
	result := make([]model.MetricSample, len(records))
	for index, record := range records {
		result[index] = recordToMetricSample(record)
	}
	return result, nil
}

func (r *metricRepository) DeleteBefore(ctx context.Context, granularity enums.MetricGranularity, before time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 50_000 {
		limit = 10_000
	}
	var ids []uint64
	modelValue := any(&MetricSampleRecord{})
	timeColumn := "timestamp"
	switch granularity {
	case enums.MetricGranularityMinute:
		modelValue, timeColumn = &MetricMinuteRecord{}, "bucket"
	case enums.MetricGranularityHour:
		modelValue, timeColumn = &MetricHourRecord{}, "bucket"
	}
	if err := r.database.WithContext(ctx).Model(modelValue).Where(timeColumn+" < ?", before.UTC()).Order(timeColumn+" asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
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

func (r *metricRepository) DeleteServer(ctx context.Context, serverID model.ID, batchSize int) (int64, error) {
	if !serverID.Valid() {
		return 0, apperror.New(apperror.CodeValidationInvalidArgument, "Metric Server ID 无效")
	}
	if batchSize <= 0 || batchSize > 50_000 {
		batchSize = 10_000
	}
	var total int64
	for _, modelValue := range []any{&MetricSampleRecord{}, &MetricMinuteRecord{}, &MetricHourRecord{}} {
		for {
			var ids []uint64
			if err := r.database.WithContext(ctx).Model(modelValue).Where("server_id = ?", serverID.String()).Limit(batchSize).Pluck("id", &ids).Error; err != nil {
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
	return total, nil
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

func aggregateToRecord(aggregate model.MetricAggregate) metricAggregateRecord {
	return metricAggregateRecord{
		ServerID: aggregate.ServerID.String(), SourceID: aggregate.SourceID.String(), Metric: aggregate.Metric.String(),
		Bucket: aggregate.Bucket.UTC(), TagsKey: metricTagsKey(aggregate.Tags), Count: aggregate.Count,
		Average: aggregate.Average, Minimum: aggregate.Minimum, Maximum: aggregate.Maximum,
		P95: aggregate.P95, Latest: aggregate.Latest, Tags: cloneMetricTags(aggregate.Tags), SchemaVersion: aggregate.SchemaVersion,
	}
}

func recordToMetricSample(record MetricSampleRecord) model.MetricSample {
	definition, _ := model.DefinitionForMetric(enums.MetricType(record.Metric))
	return model.MetricSample{
		ServerID: model.ID(record.ServerID), SourceID: model.ID(record.SourceID), Metric: enums.MetricType(record.Metric),
		Unit: definition.Unit, Timestamp: record.Timestamp, Value: record.Value, Tags: cloneMetricTags(record.Tags), SchemaVersion: record.SchemaVersion,
	}
}

func recordToMetricAggregate(record metricAggregateRecord, granularity enums.MetricGranularity) model.MetricAggregate {
	return model.MetricAggregate{
		ServerID: model.ID(record.ServerID), SourceID: model.ID(record.SourceID), Metric: enums.MetricType(record.Metric),
		Bucket: record.Bucket, Granularity: granularity, Count: record.Count, Average: record.Average,
		Minimum: record.Minimum, Maximum: record.Maximum, P95: record.P95, Latest: record.Latest,
		Tags: cloneMetricTags(record.Tags), SchemaVersion: record.SchemaVersion,
	}
}

func minuteRecordFromAggregate(record metricAggregateRecord) MetricMinuteRecord {
	return MetricMinuteRecord{
		ServerID: record.ServerID, SourceID: record.SourceID, Metric: record.Metric, Bucket: record.Bucket,
		TagsKey: record.TagsKey, Count: record.Count, Average: record.Average, Minimum: record.Minimum,
		Maximum: record.Maximum, P95: record.P95, Latest: record.Latest, Tags: record.Tags, SchemaVersion: record.SchemaVersion,
	}
}

func hourRecordFromAggregate(record metricAggregateRecord) MetricHourRecord {
	return MetricHourRecord{
		ServerID: record.ServerID, SourceID: record.SourceID, Metric: record.Metric, Bucket: record.Bucket,
		TagsKey: record.TagsKey, Count: record.Count, Average: record.Average, Minimum: record.Minimum,
		Maximum: record.Maximum, P95: record.P95, Latest: record.Latest, Tags: record.Tags, SchemaVersion: record.SchemaVersion,
	}
}

func aggregateFromMinuteRecord(record MetricMinuteRecord) metricAggregateRecord {
	return metricAggregateRecord{
		ServerID: record.ServerID, SourceID: record.SourceID, Metric: record.Metric, Bucket: record.Bucket,
		TagsKey: record.TagsKey, Count: record.Count, Average: record.Average, Minimum: record.Minimum,
		Maximum: record.Maximum, P95: record.P95, Latest: record.Latest, Tags: record.Tags, SchemaVersion: record.SchemaVersion,
	}
}

func aggregateFromHourRecord(record MetricHourRecord) metricAggregateRecord {
	return metricAggregateRecord{
		ServerID: record.ServerID, SourceID: record.SourceID, Metric: record.Metric, Bucket: record.Bucket,
		TagsKey: record.TagsKey, Count: record.Count, Average: record.Average, Minimum: record.Minimum,
		Maximum: record.Maximum, P95: record.P95, Latest: record.Latest, Tags: record.Tags, SchemaVersion: record.SchemaVersion,
	}
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
