package repository

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// MetricStorageUsage contains SQLite page usage for capacity decisions.
type MetricStorageUsage struct {
	UsedBytes int64
}

// MetricSampleCursor is the (timestamp, id) keyset cursor for range pagination without OFFSET rescans.
type MetricSampleCursor struct {
	Time time.Time
	ID   uint64
}

// MetricRepository persists raw samples and minute/hour aggregates with indexed queries.
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
