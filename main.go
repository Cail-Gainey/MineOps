package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/Cail-Gainey/MineOps/internal/bootstrap"
	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	ctx := context.Background()
	logger, logWriter, logDirectory, err := applog.NewRuntimeLogger(applog.DefaultRuntimeLogOptions())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "MineOps 日志目录不可用 (%s): %v\n", logDirectory, err)
		os.Exit(1)
	}
	defer func() { _ = logWriter.Close() }()
	applog.SetDefault(logger)
	runtime, err := bootstrap.NewRuntime(ctx, logger, logWriter)
	if err != nil {
		logger.Error(ctx, "初始化 MineOps Runtime 失败", err, applog.Fields{"component": "bootstrap"})
		os.Exit(1)
	}
	err = apperror.Guard(ctx, logger, "main", func() error {
		applicationError := bootstrap.NewApplication(assets, runtime, logger).Run()
		return bootstrap.JoinCloseError(applicationError, runtime.Close())
	})
	if err != nil {
		logger.Error(ctx, "MineOps 退出", err, applog.Fields{"component": "main"})
		os.Exit(1)
	}
}
