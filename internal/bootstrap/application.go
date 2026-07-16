// Package bootstrap assembles MineOps application dependencies and desktop services.
package bootstrap

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	goruntime "runtime"
	"sync/atomic"
	"time"

	desktopservices "github.com/Cail-Gainey/MineOps/internal/desktop/services"
	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/desktoprelease"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func init() {
	application.RegisterEvent[application.SecondInstanceData](constants.SecondInstanceEventName)
	application.RegisterEvent[desktopservices.FileDropEvent](constants.FileDropEventName)
}

// NewApplication creates the MineOps desktop application composition root.
func NewApplication(assets embed.FS, runtime *Runtime, logger *applog.Logger) *application.App {
	var mainWindow *application.WebviewWindow
	var quitting atomic.Bool
	operationService := desktopservices.NewOperationService(runtime.Operations, runtime.Store, logger)
	fileService := desktopservices.NewFileService(runtime.Files, runtime.Settings, logger, runtime.DataDirectory)
	consoleService := desktopservices.NewConsoleService(runtime.Processes, runtime.Store, logger)
	terminalService := desktopservices.NewTerminalService(runtime.SSHClients, runtime.Store, runtime.Settings, logger)
	exitGuardService := desktopservices.NewExitGuardService(runtime.ExitGuard, logger)
	unsubscribeSettings := func() {}
	shutdownCoordinator, shutdownErr := service.NewShutdownCoordinator(logger,
		service.ShutdownStep{Name: "operations", Timeout: 12 * time.Second, Action: func(context.Context) error { return operationService.ServiceShutdown() }},
		service.ShutdownStep{Name: "terminal", Timeout: 10 * time.Second, Action: func(context.Context) error { return terminalService.ServiceShutdown() }},
		service.ShutdownStep{Name: "console", Timeout: 10 * time.Second, Action: func(context.Context) error { return consoleService.ServiceShutdown() }},
		service.ShutdownStep{Name: "metrics-background", Timeout: 12 * time.Second, Action: runtime.shutdown.Shutdown},
		service.ShutdownStep{Name: "settings-events", Timeout: 2 * time.Second, Action: func(context.Context) error { unsubscribeSettings(); return nil }},
	)
	if shutdownErr != nil {
		panic(shutdownErr)
	}
	var app *application.App
	emitQuitBlocked := func(items []service.UnsavedItem) {
		if app != nil {
			app.Event.Emit(constants.QuitBlockedEventName, desktopservices.QuitBlockedEvent{Items: items})
		}
	}
	shouldQuit := func() bool {
		allowed, items := runtime.ExitGuard.RequestQuit()
		if !allowed {
			emitQuitBlocked(items)
		} else {
			quitting.Store(true)
		}
		return allowed
	}
	app = application.New(application.Options{
		Name:        constants.ApplicationName,
		Description: "Minecraft server operations desktop console",
		ShouldQuit:  shouldQuit,
		OnShutdown: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			if _, err := shutdownCoordinator.Shutdown(ctx); err != nil {
				logger.Error(context.WithoutCancel(ctx), "应用关闭协调存在失败或超时组件", err, nil)
			}
		},
		Services: []application.Service{
			application.NewService(desktopservices.NewErrorService(logger)),
			application.NewService(operationService),
			application.NewService(desktopservices.NewInstallationService(runtime.Installations, logger, runtime.DataDirectory)),
			application.NewService(desktopservices.NewDownloadService(runtime.Downloads, logger)),
			application.NewService(desktopservices.NewLifecycleService(runtime.Lifecycle, logger)),
			application.NewService(consoleService),
			application.NewService(desktopservices.NewSSHSessionService(runtime.SSHSessions, runtime.Store, runtime.Settings, runtime.SSHClients, logger)),
			application.NewService(desktopservices.NewKnownHostService(runtime.KnownHosts, runtime.Store, logger)),
			application.NewService(fileService),
			application.NewService(terminalService),
			application.NewService(desktopservices.NewJavaRuntimeService(runtime.JavaRuntimes, logger)),
			application.NewService(desktopservices.NewMinecraftServerService(runtime.MinecraftServers, runtime.Store, logger)),
			application.NewService(desktopservices.NewPlayerService(runtime.PlayerActivity, logger)),
			application.NewService(desktopservices.NewSettingsService(runtime.Settings, logger)),
			application.NewService(desktopservices.NewBackgroundService(runtime.Background, logger)),
			application.NewService(desktopservices.NewStorageService(runtime.Storage, logger)),
			application.NewService(desktopservices.NewLoggingService(runtime.Logs, logger)),
			application.NewService(desktopservices.NewDiagnosticService(runtime.Diagnostic, logger)),
			application.NewService(desktopservices.NewDesktopUpdateService(runtime.DesktopUpdates, logger)),
			application.NewService(desktopservices.NewMetricService(runtime.Metrics, logger)),
			application.NewService(desktopservices.NewMonitoringService(runtime.Monitoring, logger)),
			application.NewService(desktopservices.NewPerformanceService(runtime.Performance, logger)),
			application.NewService(desktopservices.NewAlertService(runtime.Alerts, logger)),
			application.NewService(exitGuardService),
			application.NewService(&desktopservices.BindingSpikeService{}),
			application.NewService(&desktopservices.EventSpikeService{}),
			application.NewService(&desktopservices.SQLCipherSpikeService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:      "com.gainey.mineops",
			ExitCode:      0,
			EncryptionKey: sha256.Sum256([]byte("com.gainey.mineops.single-instance.v1")),
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if mainWindow != nil {
					mainWindow.Restore()
					mainWindow.Show()
					mainWindow.Focus()
				}
				if currentApp := application.Get(); currentApp != nil {
					currentApp.Event.Emit(constants.SecondInstanceEventName, data)
				}
			},
		},
	})
	updatePublicKey, keyErr := base64.StdEncoding.DecodeString(desktoprelease.PublicKeyBase64)
	if keyErr != nil {
		logger.Error(context.Background(), "解析 Desktop Update Public Key 失败", keyErr, nil)
	}
	desktopUpdateDriver, updateErr := desktopservices.NewWailsDesktopUpdateDriver(
		app, runtime.DesktopReleaseCatalog, runtime.Settings, runtime.Downloads, runtime.ExitGuard, updatePublicKey,
	)
	if updateErr != nil {
		logger.Error(context.Background(), "初始化 Desktop 自更新驱动失败", updateErr, nil)
	} else {
		runtime.DesktopUpdates.AttachDriver(desktopUpdateDriver)
	}
	fileService.SetApplication(app)
	// macOS 自定义应用菜单:不装默认 View 菜单的 Reload/ForceReload/ZoomIn/ZoomOut,只保留全屏;
	// 必须保留 Edit 菜单 role,否则输入框的 ⌘C/⌘V/⌘X/⌘A 会全部失效。仅 darwin 设置,避免 Windows/Linux 出现菜单栏。
	if goruntime.GOOS == "darwin" {
		applicationMenu := application.NewMenu()
		applicationMenu.AddRole(application.AppMenu)
		applicationMenu.AddRole(application.FileMenu)
		applicationMenu.AddRole(application.EditMenu)
		viewMenu := applicationMenu.AddSubmenu("View")
		viewMenu.AddRole(application.ToggleFullscreen)
		applicationMenu.AddRole(application.WindowMenu)
		app.Menu.SetApplicationMenu(applicationMenu)
	}
	if level, err := applog.ParseLevel(runtime.Settings.Snapshot().Logging.Level); err == nil {
		logger.SetLevel(level)
	}
	if err := runtime.Logs.Apply(runtime.Settings.Snapshot().Logging); err != nil {
		logger.Error(context.Background(), "应用启动日志 Settings 失败", err, nil)
	}
	if applog.DetectRuntimeMode() == applog.RuntimePackaged {
		if err := runtime.DesktopPrefs.SyncApplicationVersion(context.Background()); err != nil {
			logger.Error(context.Background(), "同步桌面安装版本失败", err, nil)
		}
	}
	unsubscribeSettings = runtime.Settings.Subscribe(func(change appsettings.Change) {
		if change.Category == enums.SettingsGeneral {
			if err := runtime.DesktopPrefs.ApplyGeneral(context.Background(), change.Snapshot.General); err != nil {
				logger.Error(context.Background(), "应用通用桌面 Settings 失败", err, nil)
			}
		}
		if change.Category == enums.SettingsLogging {
			if level, err := applog.ParseLevel(change.Snapshot.Logging.Level); err == nil {
				logger.SetLevel(level)
			}
			if err := runtime.Logs.Apply(change.Snapshot.Logging); err != nil {
				logger.Error(context.Background(), "应用日志 Settings 失败", err, nil)
			}
		}
		app.Event.Emit(constants.SettingsChangedEventName, desktopservices.SettingsChangedEvent{Category: change.Category.String()})
	})

	// 桌面外壳去浏览器化:锁定缩放、关闭默认右键菜单与 macOS 前后退/捏合手势(详见 openspec desktop-shell-chrome)。
	macPreferences := application.MacWebviewPreferences{}
	macPreferences.AllowsMagnification.Set(false)
	macPreferences.AllowsBackForwardNavigationGestures.Set(false)
	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "main",
		Title:                      "MineOps",
		Width:                      1280,
		Height:                     800,
		MinWidth:                   1024,
		MinHeight:                  640,
		EnableFileDrop:             true,
		DefaultContextMenuDisabled: true,
		ZoomControlEnabled:         false,
		BackgroundColour:           application.NewRGB(15, 23, 42),
		URL:                        "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
			WebviewPreferences:      macPreferences,
		},
	})
	mainWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if quitting.Load() {
			return
		}
		if runtime.Settings.Snapshot().General.CloseBehavior == "minimize" {
			event.Cancel()
			mainWindow.Hide()
			return
		}
		allowed, items := runtime.ExitGuard.RequestQuit()
		if !allowed {
			event.Cancel()
			emitQuitBlocked(items)
		}
	})
	mainWindow.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		details := event.Context().DropTargetDetails()
		result := desktopservices.FileDropEvent{FileCount: len(files)}
		if len(files) == 0 || details == nil {
			dto := apperror.ToDTO(apperror.New(apperror.CodeValidationRequired, "拖放文件或目标信息为空"))
			result.Error = &dto
			app.Event.Emit(constants.FileDropEventName, result)
			return
		}
		sshSessionID := details.Attributes["data-ssh-session-id"]
		remoteDirectory := details.Attributes["data-remote-directory"]
		operationID, err := runtime.Files.StartUpload(app.Context(), model.ID(sshSessionID), files, remoteDirectory, remoteDirectory)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		} else {
			result.OperationID = operationID.String()
		}
		app.Event.Emit(constants.FileDropEventName, result)
	})

	return app
}
