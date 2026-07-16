package service

import (
	"context"
	"sort"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

type metricSeriesBuilder struct {
	sourceID   model.ID
	metric     enums.MetricType
	definition model.MetricDefinition
	tags       map[string]string
	points     []model.MetricPoint
}

// Query returns indexed raw or aggregate Metric series with timezone conversion and explicit gap markers.
func (m *MetricManager) Query(ctx context.Context, query model.MetricQuery) (model.MetricQueryResult, error) {
	if query.TimeZone == "" {
		query.TimeZone = "UTC"
	}
	if query.Limit == 0 {
		query.Limit = 10_000
	}
	if err := query.Validate(); err != nil {
		return model.MetricQueryResult{}, err
	}
	if _, err := m.store.MinecraftServers().Get(ctx, query.ServerID, false); err != nil {
		return model.MetricQueryResult{}, err
	}
	definition, err := model.DefinitionForMetric(query.Metric)
	if err != nil {
		return model.MetricQueryResult{}, err
	}
	location, _ := time.LoadLocation(query.TimeZone)
	builders := make(map[string]*metricSeriesBuilder)
	if query.Granularity == enums.MetricGranularityRaw {
		samples, queryErr := m.store.Metrics().ListSamples(ctx, query)
		if queryErr != nil {
			return model.MetricQueryResult{}, queryErr
		}
		for _, sample := range samples {
			key := metricBusSampleKey(sample)
			builder := builders[key]
			if builder == nil {
				builder = &metricSeriesBuilder{sourceID: sample.SourceID, metric: sample.Metric, definition: definition, tags: cloneMetricSample(sample).Tags}
				builders[key] = builder
			}
			builder.points = append(builder.points, model.MetricPoint{Timestamp: sample.Timestamp.In(location), Value: sample.Value})
		}
	} else {
		aggregates, queryErr := m.store.Metrics().ListAggregates(ctx, query)
		if queryErr != nil {
			return model.MetricQueryResult{}, queryErr
		}
		for _, aggregate := range aggregates {
			key := metricBusSampleKey(model.MetricSample{SourceID: aggregate.SourceID, Metric: aggregate.Metric, Tags: aggregate.Tags})
			builder := builders[key]
			if builder == nil {
				builder = &metricSeriesBuilder{
					sourceID: aggregate.SourceID, metric: aggregate.Metric, definition: definition,
					tags: cloneMetricSample(model.MetricSample{Tags: aggregate.Tags}).Tags,
				}
				builders[key] = builder
			}
			builder.points = append(builder.points, model.MetricPoint{
				Timestamp: aggregate.Bucket.In(location), Value: aggregateDisplayValue(aggregate, definition.Aggregation),
				Minimum: aggregate.Minimum, Maximum: aggregate.Maximum, P95: aggregate.P95, Count: aggregate.Count,
			})
		}
	}
	step := time.Duration(m.settings.Snapshot().Monitoring.IntervalSeconds) * time.Second
	switch query.Granularity {
	case enums.MetricGranularityMinute:
		step = time.Minute
	case enums.MetricGranularityHour:
		step = time.Hour
	}
	series := make([]model.MetricSeries, 0, len(builders))
	for _, builder := range builders {
		sort.Slice(builder.points, func(left, right int) bool {
			return builder.points[left].Timestamp.Before(builder.points[right].Timestamp)
		})
		series = append(series, model.MetricSeries{
			SourceID: builder.sourceID, Metric: builder.metric, Definition: builder.definition,
			Granularity: query.Granularity, Tags: builder.tags,
			Points: markMetricGaps(builder.points, query.Start.In(location), query.End.In(location), step),
		})
	}
	sort.Slice(series, func(left, right int) bool {
		if series[left].SourceID != series[right].SourceID {
			return series[left].SourceID < series[right].SourceID
		}
		return metricBusSampleKey(model.MetricSample{SourceID: series[left].SourceID, Metric: series[left].Metric, Tags: series[left].Tags}) <
			metricBusSampleKey(model.MetricSample{SourceID: series[right].SourceID, Metric: series[right].Metric, Tags: series[right].Tags})
	})
	return model.MetricQueryResult{
		ServerID: query.ServerID, Metric: query.Metric, Granularity: query.Granularity, TimeZone: query.TimeZone,
		Start: query.Start.In(location), End: query.End.In(location), Series: series,
	}, nil
}

func aggregateDisplayValue(aggregate model.MetricAggregate, aggregation enums.MetricAggregation) float64 {
	switch aggregation {
	case enums.MetricAggregationLatest:
		return aggregate.Latest
	case enums.MetricAggregationMaximum:
		return aggregate.Maximum
	case enums.MetricAggregationSum:
		return aggregate.Average * float64(aggregate.Count)
	default:
		return aggregate.Average
	}
}

func markMetricGaps(points []model.MetricPoint, start, end time.Time, step time.Duration) []model.MetricPoint {
	if len(points) == 0 || step <= 0 {
		return points
	}
	result := make([]model.MetricPoint, 0, len(points)+4)
	gapThreshold := step + step/2
	if points[0].Timestamp.Sub(start) > gapThreshold {
		result = append(result, model.MetricPoint{Timestamp: start, Missing: true})
	}
	for index, point := range points {
		if index > 0 && point.Timestamp.Sub(points[index-1].Timestamp) > gapThreshold {
			result = append(result, model.MetricPoint{Timestamp: points[index-1].Timestamp.Add(step), Missing: true})
		}
		result = append(result, point)
	}
	if end.Sub(points[len(points)-1].Timestamp) > gapThreshold {
		result = append(result, model.MetricPoint{Timestamp: points[len(points)-1].Timestamp.Add(step), Missing: true})
	}
	return result
}
