package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	desktevents "github.com/Cail-Gainey/MineOps/internal/desktop/events"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/appthread"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/desktoprelease"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/httpclient"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/jdkcatalog"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/servercatalog"
	"github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository/gormrepo"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// Runtime 持有手工组装的基础服务及其自有资源。
type Runtime struct {
	Settings              *appsettings.Manager
	Threads               *appthread.Pool
	InstallWaves          *appthread.Pool
	RemoteProbes          *appthread.Pool
	Operations            *service.OperationRunner
	SSHSessions           *service.SSHSessionManager
	KnownHosts            *service.KnownHostManager
	SSHClients            *service.SSHClientFactory
	JavaRuntimes          *service.JavaRuntimeManager
	MinecraftServers      *service.MinecraftServerManager
	Installations         *service.InstallationManager
	Downloads             *service.DownloadManager
	Processes             *service.RemoteProcessController
	Lifecycle             *service.LifecycleManager
	PlayerActivity        *service.PlayerActivityManager
	Firewall              *service.FirewallManager
	Metrics               *service.MetricManager
	MetricBus             *service.MetricBus
	MetricCollector       *service.MetricCollector
	Spark                 *service.SparkManager
	Performance           *service.PerformanceManager
	Monitoring            *service.MonitoringManager
	Alerts                *service.AlertManager
	Background            *service.BackgroundManager
	Storage               *service.StorageManager
	Logs                  *service.LogManager
	Diagnostic            *service.DiagnosticManager
	DesktopPrefs          *service.DesktopPreferences
	DesktopUpdates        *service.DesktopUpdateManager
	DesktopReleaseCatalog *desktoprelease.Catalog
	Files                 *service.FileManager
	ExitGuard             *service.ExitGuard
	Store                 *gormrepo.Store
	DataDirectory         string
	DatabasePath          string
	DatabaseID            string
	KeyVersion            int

	database        *sqlcipher.Connection
	metricsDatabase *sqlcipher.Connection
	shutdown        *service.ShutdownGroup
}

// NewRuntime 初始化安全存储、加密 SQLite、迁移、Repository、Settings 与 Operations。
func NewRuntime(ctx context.Context, logger *applog.Logger, logWriter *applog.RotatingWriter) (*Runtime, error) {
	shutdown := service.NewShutdownGroup(ctx, logger)
	exitGuard := service.NewExitGuard()
	threadPool := appthread.NewPool(appthread.Options{Name: "global", Logger: logger})
	appthread.SetDefault(threadPool)
	// 安装波次与远程探测各用独立的小池:两者会互相嵌套(波次里的 resolve_java 又会扇出探测),
	// 共用同一个池会自我抢占配额。上限取 4 是为了不打满远端 sshd 默认的 MaxSessions 10。
	installWavePool := appthread.NewPool(appthread.Options{Name: "install-wave", MaxConcurrency: 4, Logger: logger})
	remoteProbePool := appthread.NewPool(appthread.Options{Name: "remote-probe", MaxConcurrency: 4, Logger: logger})
	logs, err := service.NewLogManager(logWriter)
	if err != nil {
		return nil, err
	}
	desktopPrefs, err := service.NewDesktopPreferences()
	if err != nil {
		return nil, err
	}
	runtimeMode := applog.DetectRuntimeMode()
	storageMode := applog.ResolveStorageMode(runtimeMode)
	logDirectory, err := applog.ResolveLogDirectory(storageMode)
	if err != nil {
		return nil, err
	}
	dataDirectory := filepath.Join(filepath.Dir(logDirectory), constants.DataDirectoryName)
	databasePath := filepath.Join(dataDirectory, constants.DatabaseFileName)
	metricsDatabasePath := filepath.Join(dataDirectory, constants.MetricsDatabaseFileName)
	keyStore := sqlcipher.SystemKeyStore{}
	if storageMode == applog.RuntimeDevelopment {
		keyStore = sqlcipher.NewDevelopmentSystemKeyStore()
	}
	if err := sqlcipher.ApplyPendingMaintenance(ctx, dataDirectory, databasePath, keyStore); err != nil {
		return nil, err
	}
	databaseResult, err := sqlcipher.BootstrapDatabase(ctx, databasePath, keyStore)
	if err != nil {
		return nil, err
	}
	metricsConnection, err := sqlcipher.BootstrapMetricsDatabase(ctx, metricsDatabasePath)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	store, err := gormrepo.NewStore(databaseResult.Connection.GORM(), metricsConnection.GORM())
	if err != nil {
		_ = metricsConnection.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	settings, err := appsettings.NewManager(store)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if _, err := settings.Load(ctx); err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	background, err := service.NewBackgroundManager(settings, dataDirectory)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	backup, err := sqlcipher.NewBackupManager(databaseResult.Connection, databasePath, keyStore)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	storage, err := service.NewStorageManager(backup, databasePath, dataDirectory, databaseResult.DatabaseID, databaseResult.KeyVersion)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	diagnostic, err := service.NewDiagnosticManager(model.SystemClock{}, store, settings, logDirectory, runtimeMode)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	operations, err := service.NewOperationRunner(shutdown.Context(), model.SystemClock{}, store, logger, desktevents.OperationPublisher{})
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	sshSessions, err := service.NewSSHSessionManager(model.SystemClock{}, store)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	knownHosts, err := service.NewKnownHostManager(model.SystemClock{}, store)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	sshClients, err := service.NewSSHClientFactory(sshSessions, knownHosts)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	files, err := service.NewFileManager(model.SystemClock{}, store, settings, sshClients, operations)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	httpClient := httpclient.New(httpclient.Config{Retries: 2, MaximumResponseSize: 8 * 1024 * 1024})
	downloads, err := service.NewDownloadManager(model.SystemClock{}, store, settings, httpClient, threadPool, logger, dataDirectory)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	desktopReleaseCatalog := desktoprelease.NewCatalog(httpClient)
	desktopUpdates, err := service.NewDesktopUpdateManager(model.SystemClock{}, settings, downloads, desktopReleaseCatalog, logger)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("desktop-update", desktopUpdates.Run); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	jdkCatalog := jdkcatalog.NewAdoptiumCatalog(httpClient, downloads)
	javaRuntimes, err := service.NewJavaRuntimeManager(model.SystemClock{}, store, sshClients, settings, jdkCatalog, operations, remoteProbePool)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	firewall, err := service.NewFirewallManager(model.SystemClock{}, store, sshClients, settings)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	minecraftServers, err := service.NewMinecraftServerManager(model.SystemClock{}, store, settings, sshClients, operations, firewall)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	installationRunner, err := service.NewInstallationRunner(model.SystemClock{}, store, operations, installWavePool)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	catalogs := servercatalog.NewRegistry(
		servercatalog.NewMojangCatalog(httpClient, downloads),
		servercatalog.NewPaperCatalog(httpClient, downloads, enums.ServerPaper),
		servercatalog.NewPaperCatalog(httpClient, downloads, enums.ServerFolia),
		servercatalog.NewPaperCatalog(httpClient, downloads, enums.ServerVelocity),
		servercatalog.NewPaperCatalog(httpClient, downloads, enums.ServerWaterfall),
		servercatalog.NewPurpurCatalog(httpClient, downloads),
		servercatalog.NewFabricCatalog(httpClient, downloads),
		servercatalog.NewQuiltCatalog(httpClient, downloads),
		servercatalog.NewSpigotCatalog(httpClient, downloads),
		servercatalog.NewBungeeCatalog(downloads),
		servercatalog.NewForgeCatalog(httpClient, downloads, enums.ServerForge),
		servercatalog.NewForgeCatalog(httpClient, downloads, enums.ServerNeoForge),
	)
	catalogs.EnableGenericInstallers(
		enums.ServerVanilla, enums.ServerPaper, enums.ServerFolia, enums.ServerVelocity,
		enums.ServerWaterfall, enums.ServerPurpur, enums.ServerFabric, enums.ServerQuilt,
		enums.ServerSpigot, enums.ServerBungee,
		enums.ServerForge, enums.ServerNeoForge,
	)
	installations, err := service.NewInstallationManager(model.SystemClock{}, store, settings, sshClients, javaRuntimes, jdkCatalog, catalogs, installationRunner, firewall)
	if err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := operations.RecoverInterrupted(ctx); err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := installationRunner.RecoverInterrupted(ctx); err != nil {
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	processes, err := service.NewRemoteProcessController(model.SystemClock{}, store, sshClients, settings)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	lifecycle, err := service.NewLifecycleManager(model.SystemClock{}, store, operations, processes, firewall)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	playerActivity, err := service.NewPlayerActivityManager(model.SystemClock{}, store, settings, sshClients, processes, logger, desktevents.PlayerPublisher{})
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	lifecycle.SetObserver(playerActivity)
	if err := shutdown.Go("player-activity", playerActivity.Run); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("lifecycle-monitor", func(recoveryCtx context.Context) error { return lifecycle.Monitor(recoveryCtx, 5*time.Second) }); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	metricBus := service.NewMetricBus(model.SystemClock{}, desktevents.MetricPublisher{}, logger)
	metrics, err := service.NewMetricManager(model.SystemClock{}, store, settings, metricBus, metricsDatabasePath, logger)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("metric-realtime-bus", func(recoveryCtx context.Context) error {
		return metricBus.Run(recoveryCtx, func() time.Duration {
			return time.Duration(settings.Snapshot().Monitoring.RealtimeThrottleMillis) * time.Millisecond
		})
	}); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("metric-maintenance", metrics.Maintain); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	metricCollector, err := service.NewMetricCollector(model.SystemClock{}, store, settings, sshClients, metrics, logger)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("metric-collector", metricCollector.Run); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	alerts, err := service.NewAlertManager(model.SystemClock{}, store, desktevents.AlertPublisher{}, logger)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	metrics.SetObserver(alerts)
	if err := shutdown.Go("alert-evaluator", alerts.Run); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	spark, err := service.NewSparkManager(model.SystemClock{}, store, settings, sshClients, processes, metrics, metricCollector, operations, downloads, logger)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	lifecycle.SetObserver(spark)
	if err := spark.RecoverInterruptedReports(ctx); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("spark-collector", spark.Run); err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	performance, err := service.NewPerformanceManager(store, spark, metrics)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	monitoring, err := service.NewMonitoringManager(model.SystemClock{}, store, settings, metrics, metricCollector, spark)
	if err != nil {
		downloads.Close()
		_ = metricsConnection.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	return &Runtime{
		Settings: settings, Threads: threadPool, InstallWaves: installWavePool, RemoteProbes: remoteProbePool,
		Operations: operations, SSHSessions: sshSessions, KnownHosts: knownHosts,
		SSHClients: sshClients, Files: files, JavaRuntimes: javaRuntimes, MinecraftServers: minecraftServers,
		Installations: installations, Downloads: downloads, Processes: processes, Lifecycle: lifecycle, PlayerActivity: playerActivity, Firewall: firewall,
		Metrics: metrics, MetricBus: metricBus, MetricCollector: metricCollector,
		Spark: spark, Performance: performance, Monitoring: monitoring, Alerts: alerts,
		Background: background, Storage: storage, Logs: logs, Diagnostic: diagnostic,
		DesktopPrefs: desktopPrefs, DesktopUpdates: desktopUpdates, DesktopReleaseCatalog: desktopReleaseCatalog,
		ExitGuard: exitGuard, Store: store, DataDirectory: dataDirectory, DatabasePath: databasePath,
		DatabaseID: databaseResult.DatabaseID, KeyVersion: databaseResult.KeyVersion,
		database: databaseResult.Connection, metricsDatabase: metricsConnection, shutdown: shutdown,
	}, nil
}

// Close 在 Wails 服务停止各自 goroutine 之后释放数据库连接。
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	if r.Downloads != nil {
		r.Downloads.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(constants.DefaultShutdownTimeoutSec)*time.Second)
	defer cancel()
	var shutdownError error
	if r.shutdown != nil {
		shutdownError = r.shutdown.Shutdown(ctx)
	}
	var threadError error
	for _, pool := range []*appthread.Pool{r.InstallWaves, r.RemoteProbes, r.Threads} {
		if pool != nil {
			threadError = errors.Join(threadError, pool.Close(ctx))
		}
	}
	var databaseError error
	if r.metricsDatabase != nil {
		databaseError = r.metricsDatabase.Close()
	}
	if r.database != nil {
		databaseError = errors.Join(databaseError, r.database.Close())
	}
	return errors.Join(shutdownError, threadError, databaseError)
}

// JoinCloseError 合并应用与 Runtime 的关闭失败。
func JoinCloseError(applicationError error, runtimeError error) error {
	return errors.Join(applicationError, runtimeError)
}
