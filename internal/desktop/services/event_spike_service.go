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

// EventSpikeBatch contains one ordered batch from the stage 0 event throughput spike.
type EventSpikeBatch struct {
	BatchID            int   `json:"batchID"`
	FirstSequence      int   `json:"firstSequence"`
	LastSequence       int   `json:"lastSequence"`
	Values             []int `json:"values"`
	EmittedAtUnixMilli int64 `json:"emittedAtUnixMilli"`
}

// EventSpikeStatus reports the current event generator state and counters.
type EventSpikeStatus struct {
	Running        bool `json:"running"`
	EmittedBatches int  `json:"emittedBatches"`
	EmittedItems   int  `json:"emittedItems"`
}

// EventSpikeService emits bounded event batches and owns their shutdown lifecycle.
type EventSpikeService struct {
	mu      sync.Mutex
	app     *application.App
	rootCtx context.Context
	cancel  context.CancelFunc
	wait    sync.WaitGroup
	batches atomic.Int64
	items   atomic.Int64
}

// ServiceStartup captures the application context used to stop all event goroutines on shutdown.
func (s *EventSpikeService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.app = application.Get()
	s.rootCtx = ctx
	return nil
}

// ServiceShutdown cancels the active generator and waits until its goroutine exits.
func (s *EventSpikeService) ServiceShutdown() error {
	s.stop()
	return nil
}

// Start begins emitting ordered batches at the requested interval.
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

// Stop cancels the active generator and returns its final counters.
func (s *EventSpikeService) Stop(_ context.Context) EventSpikeStatus {
	s.stop()
	return s.Status()
}

// Status returns an atomic snapshot of the event generator state.
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
