package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const (
	metricBusMaximumServers          = 256
	metricBusMaximumSamplesPerServer = 256
)

// MetricEventPublisher 在写入事务之外发布经节流的最新值快照。
type MetricEventPublisher interface {
	PublishMetrics(context.Context, model.MetricRealtimeEvent) error
}

type metricBusValue struct {
	sample    model.MetricSample
	updatedAt time.Time
}

type metricBusServer struct {
	values    map[string]metricBusValue
	updatedAt time.Time
}

// MetricBus 维护有界的最新值缓存,并以受控速率发布发生变化的 Server 快照。
type MetricBus struct {
	clock     model.Clock
	publisher MetricEventPublisher
	logger    *applog.Logger

	mu      sync.Mutex
	servers map[model.ID]*metricBusServer
	dirty   map[model.ID]struct{}
}

// NewMetricBus 创建有界的实时指标缓存。
func NewMetricBus(clock model.Clock, publisher MetricEventPublisher, logger *applog.Logger) *MetricBus {
	if logger == nil {
		logger = applog.Default()
	}
	return &MetricBus{
		clock: clock, publisher: publisher, logger: logger,
		servers: make(map[model.ID]*metricBusServer), dirty: make(map[model.ID]struct{}),
	}
}

// Publish 更新最新值,不因 Wails 事件投递而阻塞。
func (b *MetricBus) Publish(samples []model.MetricSample) {
	if b == nil || len(samples) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sample := range samples {
		server := b.servers[sample.ServerID]
		if server == nil {
			if len(b.servers) >= metricBusMaximumServers {
				b.evictOldestServer()
			}
			server = &metricBusServer{values: make(map[string]metricBusValue)}
			b.servers[sample.ServerID] = server
		}
		key := metricBusSampleKey(sample)
		if current, found := server.values[key]; found && current.sample.Timestamp.After(sample.Timestamp) {
			continue
		}
		now := b.clock.Now().UTC()
		server.values[key] = metricBusValue{sample: cloneMetricSample(sample), updatedAt: now}
		server.updatedAt = now
		if len(server.values) > metricBusMaximumSamplesPerServer {
			evictOldestMetricValue(server.values)
		}
		b.dirty[sample.ServerID] = struct{}{}
	}
}

// Latest 返回某台 Server 缓存最新值的稳定副本。
func (b *MetricBus) Latest(serverID model.ID) []model.MetricSample {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	server := b.servers[serverID]
	result := metricBusServerSamples(server)
	b.mu.Unlock()
	return result
}

// ClearServer 清除某台 Server 的缓存最新值与待发实时事件。
func (b *MetricBus) ClearServer(serverID model.ID) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.servers, serverID)
	delete(b.dirty, serverID)
	b.mu.Unlock()
}

// Run 持续发布发生变化的快照,直到应用上下文被取消。
func (b *MetricBus) Run(ctx context.Context, throttle func() time.Duration) error {
	for {
		delay := time.Second
		if throttle != nil {
			delay = throttle()
			if delay < 100*time.Millisecond {
				delay = 100 * time.Millisecond
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
			b.flush(ctx)
		}
	}
}

func (b *MetricBus) flush(ctx context.Context) {
	b.mu.Lock()
	events := make([]model.MetricRealtimeEvent, 0, len(b.dirty))
	for serverID := range b.dirty {
		events = append(events, model.MetricRealtimeEvent{
			ServerID: serverID, Samples: metricBusServerSamples(b.servers[serverID]), EmittedAt: b.clock.Now().UTC(),
		})
	}
	clear(b.dirty)
	b.mu.Unlock()
	if b.publisher == nil {
		return
	}
	for _, event := range events {
		if err := b.publisher.PublishMetrics(ctx, event); err != nil {
			b.logger.Warn(ctx, "实时 Metric Event 推送失败", applog.Fields{"server_id": event.ServerID.String(), "error": err.Error()})
		}
	}
}

func (b *MetricBus) evictOldestServer() {
	var oldestID model.ID
	var oldestTime time.Time
	for serverID, server := range b.servers {
		if oldestID == "" || server.updatedAt.Before(oldestTime) {
			oldestID, oldestTime = serverID, server.updatedAt
		}
	}
	delete(b.servers, oldestID)
	delete(b.dirty, oldestID)
}

func evictOldestMetricValue(values map[string]metricBusValue) {
	oldestKey := ""
	var oldestTime time.Time
	for key, value := range values {
		if oldestKey == "" || value.updatedAt.Before(oldestTime) {
			oldestKey, oldestTime = key, value.updatedAt
		}
	}
	delete(values, oldestKey)
}

func metricBusServerSamples(server *metricBusServer) []model.MetricSample {
	if server == nil {
		return nil
	}
	result := make([]model.MetricSample, 0, len(server.values))
	for _, value := range server.values {
		result = append(result, cloneMetricSample(value.sample))
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Metric != result[right].Metric {
			return result[left].Metric < result[right].Metric
		}
		if result[left].SourceID != result[right].SourceID {
			return result[left].SourceID < result[right].SourceID
		}
		return result[left].Timestamp.Before(result[right].Timestamp)
	})
	return result
}

func metricBusSampleKey(sample model.MetricSample) string {
	payload, _ := json.Marshal(sample.Tags)
	hash := sha256.Sum256(payload)
	return sample.SourceID.String() + "\x00" + sample.Metric.String() + "\x00" + hex.EncodeToString(hash[:])
}

func cloneMetricSample(sample model.MetricSample) model.MetricSample {
	cloned := sample
	if len(sample.Tags) > 0 {
		cloned.Tags = make(map[string]string, len(sample.Tags))
		for key, value := range sample.Tags {
			cloned.Tags[key] = value
		}
	}
	return cloned
}
