package events

import (
	"context"
	"errors"

	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[model.MetricRealtimeEvent](constants.MetricRealtimeEventName)
}

// MetricPublisher emits throttled latest-value snapshots through Wails Events.
type MetricPublisher struct{}

// PublishMetrics emits one immutable Server metric snapshot to subscribed desktop windows.
func (MetricPublisher) PublishMetrics(ctx context.Context, event model.MetricRealtimeEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return errors.New("wails application is not available")
	}
	app.Event.Emit(constants.MetricRealtimeEventName, event)
	return nil
}
