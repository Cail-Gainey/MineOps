package apperror

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

// Recover converts a panic at a named boundary into an application error and structured crash log.
func Recover(ctx context.Context, logger *applog.Logger, boundary string, target *error) {
	value := recover()
	if value == nil {
		return
	}
	panicError := Wrap(CodeInternal, "发生未预期错误", fmt.Errorf("panic: %v\n%s", value, debug.Stack()))
	if logger != nil {
		logger.Error(ctx, "捕获未处理 panic", panicError, applog.Fields{"boundary": boundary})
	}
	if target != nil {
		*target = errors.Join(*target, panicError)
	}
}

// Guard executes one process or service boundary with unified panic conversion.
func Guard(ctx context.Context, logger *applog.Logger, boundary string, action func() error) (err error) {
	defer Recover(ctx, logger, boundary, &err)
	if action == nil {
		return New(CodeValidationRequired, "受保护操作不能为空")
	}
	return action()
}
