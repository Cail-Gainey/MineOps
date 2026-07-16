package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	desktevents "github.com/Cail-Gainey/MineOps/internal/desktop/events"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
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

// Runtime contains manually composed stage 1 services and their owned resources.
type Runtime struct {
	Settings              *appsettings.Manager
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

	database *sqlcipher.Connection
	shutdown *service.ShutdownGroup
}

// NewRuntime initializes secure storage, encrypted SQLite, migrations, repositories, Settings, and Operations.
func NewRuntime(ctx context.Context, logger *applog.Logger, logWriter *applog.RotatingWriter) (*Runtime, error) {
	shutdown := service.NewShutdownGroup(ctx, logger)
	exitGuard := service.NewExitGuard()
	logs, err := service.NewLogManager(logWriter)
	if err != nil {
		return nil, err
	}
	desktopPrefs, err := service.NewDesktopPreferences()
	if err != nil {
		return nil, err
	}
	logDirectory, err := applog.ResolveLogDirectory(applog.DetectRuntimeMode())
	if err != nil {
		return nil, err
	}
	dataDirectory := filepath.Join(filepath.Dir(logDirectory), constants.DataDirectoryName)
	databasePath := filepath.Join(dataDirectory, constants.DatabaseFileName)
	keyStore := sqlcipher.SystemKeyStore{}
	if err := sqlcipher.ApplyPendingMaintenance(ctx, dataDirectory, databasePath, keyStore); err != nil {
		return nil, err
	}
	databaseResult, err := sqlcipher.BootstrapDatabase(ctx, databasePath, keyStore)
	if err != nil {
		return nil, err
	}
	store, err := gormrepo.NewStore(databaseResult.Connection.GORM())
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	settings, err := appsettings.NewManager(store)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if _, err := settings.Load(ctx); err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	background, err := service.NewBackgroundManager(settings, dataDirectory)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	backup, err := sqlcipher.NewBackupManager(databaseResult.Connection, databasePath, keyStore)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	storage, err := service.NewStorageManager(backup, databasePath, dataDirectory, databaseResult.DatabaseID, databaseResult.KeyVersion)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	diagnostic, err := service.NewDiagnosticManager(model.SystemClock{}, store, settings, logDirectory, applog.DetectRuntimeMode())
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	operations, err := service.NewOperationRunner(shutdown.Context(), model.SystemClock{}, store, logger, desktevents.OperationPublisher{})
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	sshSessions, err := service.NewSSHSessionManager(model.SystemClock{}, store)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	knownHosts, err := service.NewKnownHostManager(model.SystemClock{}, store)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	sshClients, err := service.NewSSHClientFactory(sshSessions, knownHosts)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	files, err := service.NewFileManager(model.SystemClock{}, store, settings, sshClients, operations)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	httpClient := httpclient.New(httpclient.Config{Retries: 2, MaximumResponseSize: 8 * 1024 * 1024})
	downloads, err := service.NewDownloadManager(model.SystemClock{}, store, settings, httpClient, logger, dataDirectory)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	desktopReleaseCatalog := desktoprelease.NewCatalog(httpClient)
	desktopUpdates, err := service.NewDesktopUpdateManager(model.SystemClock{}, settings, downloads, desktopReleaseCatalog, logger)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("desktop-update", desktopUpdates.Run); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	jdkCatalog := jdkcatalog.NewAdoptiumCatalog(httpClient, downloads)
	javaRuntimes, err := service.NewJavaRuntimeManager(model.SystemClock{}, store, sshClients, settings, jdkCatalog, operations)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	firewall, err := service.NewFirewallManager(model.SystemClock{}, store, sshClients, settings)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	minecraftServers, err := service.NewMinecraftServerManager(model.SystemClock{}, store, settings, sshClients, operations, firewall)
	if err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	installationRunner, err := service.NewInstallationRunner(model.SystemClock{}, store, operations)
	if err != nil {
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
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := operations.RecoverInterrupted(ctx); err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := installationRunner.RecoverInterrupted(ctx); err != nil {
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	processes, err := service.NewRemoteProcessController(model.SystemClock{}, store, sshClients, settings)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	lifecycle, err := service.NewLifecycleManager(model.SystemClock{}, store, operations, processes, firewall)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	playerActivity, err := service.NewPlayerActivityManager(model.SystemClock{}, store, settings, sshClients, processes, logger, desktevents.PlayerPublisher{})
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	lifecycle.SetObserver(playerActivity)
	if err := shutdown.Go("player-activity", playerActivity.Run); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("lifecycle-monitor", func(recoveryCtx context.Context) error { return lifecycle.Monitor(recoveryCtx, 5*time.Second) }); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	metricBus := service.NewMetricBus(model.SystemClock{}, desktevents.MetricPublisher{}, logger)
	metrics, err := service.NewMetricManager(model.SystemClock{}, store, settings, metricBus, databasePath, logger)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("metric-realtime-bus", func(recoveryCtx context.Context) error {
		return metricBus.Run(recoveryCtx, func() time.Duration {
			return time.Duration(settings.Snapshot().Monitoring.RealtimeThrottleMillis) * time.Millisecond
		})
	}); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("metric-maintenance", metrics.Maintain); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	metricCollector, err := service.NewMetricCollector(model.SystemClock{}, store, settings, sshClients, metrics, logger)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("metric-collector", metricCollector.Run); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	alerts, err := service.NewAlertManager(model.SystemClock{}, store, desktevents.AlertPublisher{}, logger)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	metrics.SetObserver(alerts)
	if err := shutdown.Go("alert-evaluator", alerts.Run); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	spark, err := service.NewSparkManager(model.SystemClock{}, store, settings, sshClients, processes, metrics, metricCollector, operations, downloads, logger)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	lifecycle.SetObserver(spark)
	if err := spark.RecoverInterruptedReports(ctx); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	if err := shutdown.Go("spark-collector", spark.Run); err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	performance, err := service.NewPerformanceManager(store, spark, metrics)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	monitoring, err := service.NewMonitoringManager(model.SystemClock{}, store, settings, metrics, metricCollector, spark)
	if err != nil {
		downloads.Close()
		_ = databaseResult.Connection.Close()
		return nil, err
	}
	return &Runtime{
		Settings: settings, Operations: operations, SSHSessions: sshSessions, KnownHosts: knownHosts,
		SSHClients: sshClients, Files: files, JavaRuntimes: javaRuntimes, MinecraftServers: minecraftServers,
		Installations: installations, Downloads: downloads, Processes: processes, Lifecycle: lifecycle, PlayerActivity: playerActivity, Firewall: firewall,
		Metrics: metrics, MetricBus: metricBus, MetricCollector: metricCollector,
		Spark: spark, Performance: performance, Monitoring: monitoring, Alerts: alerts,
		Background: background, Storage: storage, Logs: logs, Diagnostic: diagnostic,
		DesktopPrefs: desktopPrefs, DesktopUpdates: desktopUpdates, DesktopReleaseCatalog: desktopReleaseCatalog,
		ExitGuard: exitGuard, Store: store, DataDirectory: dataDirectory, DatabasePath: databasePath,
		DatabaseID: databaseResult.DatabaseID, KeyVersion: databaseResult.KeyVersion,
		database: databaseResult.Connection, shutdown: shutdown,
	}, nil
}

// Close releases the encrypted database after Wails services have stopped their goroutines.
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
	var databaseError error
	if r.database != nil {
		databaseError = r.database.Close()
	}
	return errors.Join(shutdownError, databaseError)
}

// JoinCloseError combines application and Runtime shutdown failures.
func JoinCloseError(applicationError error, runtimeError error) error {
	return errors.Join(applicationError, runtimeError)
}
