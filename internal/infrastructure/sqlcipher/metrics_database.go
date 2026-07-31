package sqlcipher

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/repository/gormrepo"
	"gorm.io/gorm"
)

// MetricsMigrations 返回监控数据库不可变的迁移序列。
// 该库不加密:只放 Metric 序列字典、原始样本、分钟/小时聚合与 Spark Snapshot,全部是数值时序,
// 不含凭据、口令或玩家身份。加密库的迁移序列与此完全独立,两边版本号互不影响。
func MetricsMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "create_metric_series_samples_and_aggregates",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(
					&gormrepo.MetricSeriesRecord{},
					&gormrepo.MetricSampleRecord{},
					&gormrepo.MetricMinuteRecord{},
					&gormrepo.MetricHourRecord{},
				)
			},
		},
		{
			Version: 2,
			Name:    "create_spark_snapshots",
			Apply: func(database *gorm.DB) error {
				return database.AutoMigrate(&gormrepo.SparkSnapshotRecord{})
			},
		},
	}
}

// BootstrapMetricsDatabase 打开未加密的监控数据库并执行迁移。
func BootstrapMetricsDatabase(ctx context.Context, path string) (*Connection, error) {
	connection, err := OpenPlainConnection(ctx, ConnectionOptions{Path: path})
	if err != nil {
		return nil, err
	}
	runner, err := NewMigrationRunner(connection.GORM(), MetricsMigrations())
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if err := runner.Run(ctx); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return connection, nil
}
