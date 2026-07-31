package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[EventSpikeBatch](constants.EventSpikeBatchName)
}

// EventSpikeBatch 承载事件吞吐验证中的一个有序批次。
type EventSpikeBatch struct {
	BatchID            int   `json:"batchID"`
	FirstSequence      int   `json:"firstSequence"`
	LastSequence       int   `json:"lastSequence"`
	Values             []int `json:"values"`
	EmittedAtUnixMilli int64 `json:"emittedAtUnixMilli"`
}

// EventSpikeStatus 汇报当前事件生成器状态与计数。
type EventSpikeStatus struct {
	Running        bool `json:"running"`
	EmittedBatches int  `json:"emittedBatches"`
	EmittedItems   int  `json:"emittedItems"`
}

// EventSpikeService 发送有界事件批次并负责其关闭生命周期。
type EventSpikeService struct {
	mu      sync.Mutex
	app     *application.App
	rootCtx context.Context
	cancel  context.CancelFunc
	wait    sync.WaitGroup
	batches atomic.Int64
	items   atomic.Int64
}

// ServiceStartup 捕获应用上下文,用于关闭时停止全部事件 goroutine。
func (s *EventSpikeService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.app = application.Get()
	s.rootCtx = ctx
	return nil
}

// ServiceShutdown 取消活动生成器并等待其 goroutine 退出。
func (s *EventSpikeService) ServiceShutdown() error {
	s.stop()
	return nil
}

// Start 按指定间隔开始发送有序批次。
func (s *EventSpikeService) Start(_ context.Context, intervalMilliseconds int, batchSize int) error {
	if intervalMilliseconds < 1 || intervalMilliseconds > 1_000 {
		return errors.New("intervalMilliseconds must be between 1 and 1000")
	}
	if batchSize < 1 || batchSize > 4_096 {
		return errors.New("batchSize must be between 1 and 4096")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rootCtx == nil || s.app == nil {
		return errors.New("event spike service has not started")
	}
	if s.cancel != nil {
		return errors.New("event spike is already running")
	}

	ctx, cancel := context.WithCancel(s.rootCtx)
	s.cancel = cancel
	s.batches.Store(0)
	s.items.Store(0)
	s.wait.Add(1)
	go s.emitBatches(ctx, time.Duration(intervalMilliseconds)*time.Millisecond, batchSize)
	return nil
}

// Stop 取消活动生成器并返回最终计数。
func (s *EventSpikeService) Stop(_ context.Context) EventSpikeStatus {
	s.stop()
	return s.Status()
}

// Status 返回事件生成器状态的原子快照。
func (s *EventSpikeService) Status() EventSpikeStatus {
	s.mu.Lock()
	running := s.cancel != nil
	s.mu.Unlock()

	return EventSpikeStatus{
		Running:        running,
		EmittedBatches: int(s.batches.Load()),
		EmittedItems:   int(s.items.Load()),
	}
}

func (s *EventSpikeService) emitBatches(ctx context.Context, interval time.Duration, batchSize int) {
	defer s.wait.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sequence := 0
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			values := make([]int, batchSize)
			firstSequence := sequence + 1
			for index := range values {
				sequence++
				values[index] = sequence
			}

			batchID := int(s.batches.Add(1))
			s.items.Add(int64(batchSize))
			s.app.Event.Emit(constants.EventSpikeBatchName, EventSpikeBatch{
				BatchID:            batchID,
				FirstSequence:      firstSequence,
				LastSequence:       sequence,
				Values:             values,
				EmittedAtUnixMilli: now.UnixMilli(),
			})
		}
	}
}

func (s *EventSpikeService) stop() {
	s.mu.Lock()
	cancel := s.cancel
	if cancel == nil {
		s.mu.Unlock()
		return
	}

	cancel()
	s.wait.Wait()
	s.cancel = nil
	s.mu.Unlock()
}
