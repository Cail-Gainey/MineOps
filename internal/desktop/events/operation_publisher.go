// Package events 把应用事件桥接到 Wails 桌面运行时。
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

// OperationPublisher 通过 Wails 事件发布经节流的与终态的 Operation 快照。
type OperationPublisher struct{}

// PublishOperation 向已订阅的桌面窗口发布一份不可变的 Operation 快照。
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
