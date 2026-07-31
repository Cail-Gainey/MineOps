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

// MetricPublisher 通过 Wails 事件发布经节流的最新值快照。
type MetricPublisher struct{}

// PublishMetrics 向已订阅的桌面窗口发布一份不可变的 Server 指标快照。
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
