package model

import (
	"math"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

const MetricSchemaVersion = 1

// MetricDefinition describes unit, value type, and preferred aggregation for one registered metric.
type MetricDefinition struct {
	Metric      enums.MetricType        `json:"metric"`
	Unit        enums.MetricUnit        `json:"unit"`
	ValueType   enums.MetricValueType   `json:"valueType"`
	Aggregation enums.MetricAggregation `json:"aggregation"`
}

// MetricSample is one validated UTC observation from a Server-related source.
type MetricSample struct {
	ServerID      ID                `json:"serverID"`
	SourceID      ID                `json:"sourceID"`
	Metric        enums.MetricType  `json:"metric"`
	Unit          enums.MetricUnit  `json:"unit"`
	Timestamp     time.Time         `json:"timestamp"`
	Value         float64           `json:"value"`
	Tags          map[string]string `json:"tags,omitempty"`
	SchemaVersion int               `json:"schemaVersion"`
}

// MetricAggregate stores one minute/hour rollup with distribution evidence.
type MetricAggregate struct {
	ServerID      ID                      `json:"serverID"`
	SourceID      ID                      `json:"sourceID"`
	Metric        enums.MetricType        `json:"metric"`
	Bucket        time.Time               `json:"bucket"`
	Granularity   enums.MetricGranularity `json:"granularity"`
	Count         int64                   `json:"count"`
	Average       float64                 `json:"average"`
	Minimum       float64                 `json:"minimum"`
	Maximum       float64                 `json:"maximum"`
	P95           float64                 `json:"p95"`
	Latest        float64                 `json:"latest"`
	Tags          map[string]string       `json:"tags,omitempty"`
	SchemaVersion int                     `json:"schemaVersion"`
}

// MetricQuery contains indexed Server/Metric/time/granularity pagination.
type MetricQuery struct {
	ServerID    ID                      `json:"serverID"`
	SourceID    ID                      `json:"sourceID,omitempty"`
	Metric      enums.MetricType        `json:"metric"`
	Start       time.Time               `json:"start"`
	End         time.Time               `json:"end"`
	Granularity enums.MetricGranularity `json:"granularity"`
	TimeZone    string                  `json:"timeZone"`
	Limit       int                     `json:"limit"`
	Offset      int                     `json:"offset"`
}

// MetricPoint is a query-ready value with an explicit missing-data marker.
type MetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Minimum   float64   `json:"minimum,omitempty"`
	Maximum   float64   `json:"maximum,omitempty"`
	P95       float64   `json:"p95,omitempty"`
	Count     int64     `json:"count,omitempty"`
	Missing   bool      `json:"missing"`
}

// MetricSeries is one source/tag-specific query series with explicit gaps.
type MetricSeries struct {
	SourceID    ID                      `json:"sourceID"`
	Metric      enums.MetricType        `json:"metric"`
	Definition  MetricDefinition        `json:"definition"`
	Granularity enums.MetricGranularity `json:"granularity"`
	Tags        map[string]string       `json:"tags,omitempty"`
	Points      []MetricPoint           `json:"points"`
}

// MetricQueryResult contains normalized query bounds and grouped series.
type MetricQueryResult struct {
	ServerID    ID                      `json:"serverID"`
	Metric      enums.MetricType        `json:"metric"`
	Granularity enums.MetricGranularity `json:"granularity"`
	TimeZone    string                  `json:"timeZone"`
	Start       time.Time               `json:"start"`
	End         time.Time               `json:"end"`
	Series      []MetricSeries          `json:"series"`
}

// MetricIngestResult reports one atomic batch acceptance decision.
type MetricIngestResult struct {
	Received   int            `json:"received"`
	Accepted   int            `json:"accepted"`
	Rejected   int            `json:"rejected"`
	AcceptedAt time.Time      `json:"acceptedAt"`
	Latest     []MetricSample `json:"latest"`
}

// MetricRealtimeEvent is the throttled latest-value snapshot for one Server.
type MetricRealtimeEvent struct {
	ServerID  ID             `json:"serverID"`
	Samples   []MetricSample `json:"samples"`
	EmittedAt time.Time      `json:"emittedAt"`
}

// MetricMaintenanceResult summarizes one bounded background maintenance pass.
type MetricMaintenanceResult struct {
	MinuteAggregates int   `json:"minuteAggregates"`
	HourAggregates   int   `json:"hourAggregates"`
	RawDeleted       int64 `json:"rawDeleted"`
	MinuteDeleted    int64 `json:"minuteDeleted"`
	HourDeleted      int64 `json:"hourDeleted"`
}

// MetricStorageStatus describes database capacity protection state.
type MetricStorageStatus struct {
	DatabaseBytes          int64 `json:"databaseBytes"`
	AllocatedDatabaseBytes int64 `json:"allocatedDatabaseBytes"`
	CapacityBytes          int64 `json:"capacityBytes"`
	AvailableDiskBytes     int64 `json:"availableDiskBytes"`
	MinimumFreeBytes       int64 `json:"minimumFreeBytes"`
	Pressure               bool  `json:"pressure"`
}

// DefinitionForMetric returns the stable registered semantics for one metric.
func DefinitionForMetric(metric enums.MetricType) (MetricDefinition, error) {
	definitions := map[enums.MetricType]MetricDefinition{
		enums.MetricHostCPU:             {Metric: metric, Unit: enums.MetricUnitPercent, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostMemory:          {Metric: metric, Unit: enums.MetricUnitPercent, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostLoad1:           {Metric: metric, Unit: enums.MetricUnitRatio, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostMemoryUsed:      {Metric: metric, Unit: enums.MetricUnitBytes, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostMemoryTotal:     {Metric: metric, Unit: enums.MetricUnitBytes, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationLatest},
		enums.MetricHostSwapUsed:        {Metric: metric, Unit: enums.MetricUnitBytes, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostDiskUsed:        {Metric: metric, Unit: enums.MetricUnitBytes, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostDiskTotal:       {Metric: metric, Unit: enums.MetricUnitBytes, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationLatest},
		enums.MetricHostDiskRead:        {Metric: metric, Unit: enums.MetricUnitBytesPerSec, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostDiskWrite:       {Metric: metric, Unit: enums.MetricUnitBytesPerSec, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostNetworkReceive:  {Metric: metric, Unit: enums.MetricUnitBytesPerSec, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricHostNetworkTransmit: {Metric: metric, Unit: enums.MetricUnitBytesPerSec, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricProcessCPU:          {Metric: metric, Unit: enums.MetricUnitPercent, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricProcessRSS:          {Metric: metric, Unit: enums.MetricUnitBytes, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricProcessThreads:      {Metric: metric, Unit: enums.MetricUnitCount, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationMaximum},
		enums.MetricProcessFDs:          {Metric: metric, Unit: enums.MetricUnitCount, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationMaximum},
		enums.MetricProcessUptime:       {Metric: metric, Unit: enums.MetricUnitSeconds, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationLatest},
		enums.MetricProcessRunning:      {Metric: metric, Unit: enums.MetricUnitRatio, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationLatest},
		enums.MetricMinecraftTPS:        {Metric: metric, Unit: enums.MetricUnitTicksPerSec, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricMinecraftMSPT:       {Metric: metric, Unit: enums.MetricUnitMilliseconds, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationAverage},
		enums.MetricMinecraftPlayers:    {Metric: metric, Unit: enums.MetricUnitCount, ValueType: enums.MetricValueGauge, Aggregation: enums.MetricAggregationMaximum},
	}
	definition, found := definitions[metric]
	if !found {
		return MetricDefinition{}, apperror.New(apperror.CodeValidationInvalidArgument, "Metric 未注册")
	}
	return definition, nil
}

// Validate checks identity, UTC timestamp, finite value, tags, and schema version.
func (s MetricSample) Validate(now time.Time) error {
	if !s.ServerID.Valid() || !s.SourceID.Valid() || !s.Metric.Valid() || s.SchemaVersion != MetricSchemaVersion {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Sample 身份、类型或 Schema 无效")
	}
	if s.Timestamp.IsZero() || s.Timestamp.After(now.UTC().Add(5*time.Minute)) || math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Sample 时间或数值无效")
	}
	definition, err := DefinitionForMetric(s.Metric)
	if err != nil {
		return err
	}
	if !s.Unit.Valid() || s.Unit != definition.Unit {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Sample 单位与注册定义不匹配")
	}
	if len(s.Tags) > 32 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Tags 数量超过限制")
	}
	for key, value := range s.Tags {
		if strings.TrimSpace(key) == "" || len(key) > 80 || len(value) > 256 || strings.ContainsAny(key+value, "\x00\r\n") {
			return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Tag 无效")
		}
	}
	return nil
}

// Validate checks the indexed query bounds, granularity, timezone, and pagination.
func (q MetricQuery) Validate() error {
	if !q.ServerID.Valid() || !q.Metric.Valid() || !q.Granularity.Valid() || q.Start.IsZero() || q.End.IsZero() || !q.Start.Before(q.End) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Query 身份、类型、粒度或时间范围无效")
	}
	if q.SourceID != "" && !q.SourceID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Query Source ID 无效")
	}
	if q.Limit < 0 || q.Limit > 100_000 || q.Offset < 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Query 分页参数无效")
	}
	if strings.TrimSpace(q.TimeZone) != "" {
		if _, err := time.LoadLocation(q.TimeZone); err != nil {
			return apperror.New(apperror.CodeValidationInvalidArgument, "Metric Query 时区无效")
		}
	}
	return nil
}
