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

// PlayerPublisher 通过 Wails 事件发布带版本的玩家活动更新。
type PlayerPublisher struct{}

// PublishPlayer 发布一条不可变的、按 Server 与玩家限定的更新。
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
