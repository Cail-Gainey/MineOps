package events

import (
	"context"
	"errors"

	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[model.PlayerEvent](constants.PlayerEventName)
}

// PlayerPublisher emits versioned player activity updates through Wails Events.
type PlayerPublisher struct{}

// PublishPlayer emits one immutable Server/player-scoped update.
func (PlayerPublisher) PublishPlayer(ctx context.Context, event model.PlayerEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return errors.New("wails application is not available")
	}
	app.Event.Emit(constants.PlayerEventName, event)
	return nil
}
