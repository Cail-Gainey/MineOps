package repository

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// MetricStorageUsage 承载用于容量判定的 SQLite 页使用量。
type MetricStorageUsage struct {
	UsedBytes int64
}

// MetricSampleCursor 是 (timestamp, id) 游标,用于区间分页并避免 OFFSET 重复扫描。
type MetricSampleCursor struct {
	Time time.Time
	ID   uint64
}

// MetricRepository 持久化原始样本与分钟/小时聚合,并提供走索引的查询。
type MetricRepository interface {
	InsertSamples(context.Context, []model.MetricSample) error
	ListSamples(context.Context, model.MetricQuery) ([]model.MetricSample, error)
	ListSamplesRange(context.Context, time.Time, time.Time, MetricSampleCursor, int) ([]model.MetricSample, MetricSampleCursor, error)
	UpsertAggregates(context.Context, []model.MetricAggregate) error
	ListAggregates(context.Context, model.MetricQuery) ([]model.MetricAggregate, error)
	Latest(context.Context, model.ID, []enums.MetricType) ([]model.MetricSample, error)
	DeleteBefore(context.Context, enums.MetricGranularity, time.Time, int) (int64, error)
	StorageUsage(context.Context) (MetricStorageUsage, error)
	DeleteServer(context.Context, model.ID, int) (int64, error)
}
