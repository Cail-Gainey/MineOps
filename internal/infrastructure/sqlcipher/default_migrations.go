package sqlcipher

import (
	"github.com/Cail-Gainey/MineOps/internal/repository/gormrepo"
	"gorm.io/gorm"
)

// DefaultMigrations returns the immutable production migration sequence.
func DefaultMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "create_operations",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.OperationRecord{})
			},
		},
		{
			Version: 2,
			Name:    "create_settings",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.SettingsRecord{})
			},
		},
		{
			Version: 3,
			Name:    "create_database_metadata",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&databaseMetadataRecord{})
			},
		},
		{
			Version: 4,
			Name:    "create_ssh_sessions_credentials_known_hosts",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(
					&gormrepo.SSHSessionRecord{},
					&gormrepo.SSHCredentialRecord{},
					&gormrepo.KnownHostRecord{},
				)
			},
		},
		{
			Version: 5,
			Name:    "create_java_runtimes",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.JavaRuntimeRecord{})
			},
		},
		{
			Version: 6,
			Name:    "create_minecraft_servers",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.ServerRecord{})
			},
		},
		{
			Version: 7,
			Name:    "create_installation_tasks_and_steps",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.InstallationTaskRecord{}, &gormrepo.InstallationStepRecord{})
			},
		},
		{
			Version: 8,
			Name:    "create_proxy_credentials",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.ProxyCredentialRecord{})
			},
		},
		{
			Version: 9,
			Name:    "add_installation_step_error_details",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.InstallationStepRecord{})
			},
		},
		{
			Version: 10,
			Name:    "create_remote_process_identities",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.ProcessIdentityRecord{})
			},
		},
		{
			Version: 11,
			Name:    "create_metric_raw_minute_hour_tables",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.MetricSampleRecord{}, &gormrepo.MetricMinuteRecord{}, &gormrepo.MetricHourRecord{})
			},
		},
		{
			Version: 12,
			Name:    "add_metric_raw_series_identity",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.MetricSampleRecord{})
			},
		},
		{
			Version: 13,
			Name:    "create_agent_registration_credentials_tokens_and_batches",
			// 历史 Agent 推送架构已移除；本步骤置空。已应用库中的遗留表无害,新库不再创建。
			Apply: func(database *gorm.DB) error {
				return nil
			},
		},
		{
			Version: 14,
			Name:    "add_agent_deployment_identity",
			// 历史 Agent 推送架构已移除;本步骤置空。
			Apply: func(database *gorm.DB) error {
				return nil
			},
		},
		{
			Version: 15,
			Name:    "add_agent_authority_upgrade_signing_keys",
			// 历史 Agent 推送架构已移除;本步骤置空。
			Apply: func(database *gorm.DB) error {
				return nil
			},
		},
		{
			Version: 16,
			Name:    "create_spark_capability_snapshots_reports_and_alerts",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(
					&gormrepo.SparkCapabilityRecord{},
					&gormrepo.SparkSnapshotRecord{},
					&gormrepo.SparkReportRecord{},
					&gormrepo.AlertRuleRecord{},
					&gormrepo.AlertEventRecord{},
				)
			},
		},
		{
			Version: 17,
			Name:    "create_firewall_rule_leases",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.FirewallRuleLeaseRecord{})
			},
		},
		{
			Version: 18,
			Name:    "add_remote_process_tmux_session",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.ProcessIdentityRecord{})
			},
		},
		{
			Version: 19,
			Name:    "add_spark_install_manifest",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.SparkCapabilityRecord{})
			},
		},
		{
			Version: 20,
			Name:    "add_spark_snapshot_capped_tps_markers",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.SparkSnapshotRecord{})
			},
		},
		{
			Version: 21,
			Name:    "create_player_activity_tables",
			Apply: func(database *gorm.DB) error {
				if err := database.AutoMigrate(
					&gormrepo.PlayerIdentityRecord{}, &gormrepo.PlayerActivityEventRecord{},
					&gormrepo.PlayerSessionRecord{}, &gormrepo.PlayerStatisticsRecord{},
					&gormrepo.PlayerDirectorySnapshotRecord{}, &gormrepo.PlayerCollectorStatusRecord{},
				); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Version: 22,
			Name:    "add_ssh_session_host_specs",
			// 主机规格改为 SSH Session 持久化元数据。新增列带 NOT NULL DEFAULT 0,
			// 采集时间可为 NULL,已有 Session 升级后规格为空并在首次读取时惰性补采。
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.SSHSessionRecord{})
			},
		},
		{
			Version: 23,
			Name:    "add_metric_time_indexes",
			// 维护降采样与保留清理均按时间范围扫描，为 Metric 三表补时间列索引，
			// 否则每轮维护对原始样本表全表扫描（SQLCipher 逐页解密+HMAC）。
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.MetricSampleRecord{}, &gormrepo.MetricMinuteRecord{}, &gormrepo.MetricHourRecord{})
			},
		},
	}
}
