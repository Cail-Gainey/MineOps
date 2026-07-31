package events

import (
	"context"
	"errors"

	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[model.AlertEvent](constants.AlertEventName)
}

// AlertPublisher 通过 Wails 事件发布持久化的阈值告警状态迁移。
type AlertPublisher struct{}

// PublishAlert 发布一条活跃、已恢复或已确认的告警事件。
func (AlertPublisher) PublishAlert(ctx context.Context, event model.AlertEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return errors.New("wails application is not available")
	}
	app.Event.Emit(constants.AlertEventName, event)
	return nil
}
