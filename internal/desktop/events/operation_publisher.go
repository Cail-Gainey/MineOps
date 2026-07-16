// Package events bridges application events to the Wails desktop runtime.
package events

import (
	"context"
	"errors"

	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[model.Operation](constants.OperationProgressEventName)
}

// OperationPublisher emits throttled and final Operation snapshots through Wails Events.
type OperationPublisher struct{}

// PublishOperation emits one immutable Operation snapshot to subscribed desktop windows.
func (OperationPublisher) PublishOperation(ctx context.Context, operation model.Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return errors.New("wails application is not available")
	}
	app.Event.Emit(constants.OperationProgressEventName, operation)
	return nil
}
