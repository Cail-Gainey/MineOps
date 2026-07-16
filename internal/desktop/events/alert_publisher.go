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

// AlertPublisher emits durable threshold incident transitions through Wails Events.
type AlertPublisher struct{}

// PublishAlert emits one active, recovered, or acknowledged alert event.
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
