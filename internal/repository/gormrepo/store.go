// Package gormrepo 用 GORM 与 SQLCipher 实现 MineOps 的各个 Repository。
package gormrepo

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
)

// Store 是基于 GORM 的 Repository 注册表与事务边界。
// 监控时序(Metric 三表与 Spark Snapshot)是纯数值、不含凭据或隐私,单独落在未加密的 metrics 库里,
// 完全不付 SQLCipher 逐页 AES + HMAC 的代价;其余含凭据、口令、玩家身份的表继续留在加密库。
type Store struct {
	database *gorm.DB
	metrics  *gorm.DB
	series   *MetricSeriesCache
}

// NewStore 基于加密库与监控库两个 GORM 句柄创建 Repository Store。
func NewStore(database, metrics *gorm.DB) (*Store, error) {
	if database == nil || metrics == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Repository 加密数据库和监控数据库不能为空")
	}
	return &Store{database: database, metrics: metrics, series: NewMetricSeriesCache()}, nil
}

// Operations 返回绑定当前数据库句柄的 Operation Repository。
func (s *Store) Operations() repository.OperationRepository {
	return &operationRepository{database: s.database}
}

// Settings 返回绑定当前数据库句柄的 Settings Repository。
func (s *Store) Settings() repository.SettingsRepository {
	return &settingsRepository{store: s}
}

// SSHSessions 返回绑定当前数据库句柄的 SSH Session Repository。
func (s *Store) SSHSessions() repository.SSHSessionRepository {
	return &sshSessionRepository{database: s.database}
}

// SSHCredentials 返回绑定当前数据库句柄的加密 SSH 凭据 Repository。
func (s *Store) SSHCredentials() repository.SSHCredentialRepository {
	return &sshCredentialRepository{database: s.database}
}

// KnownHosts 返回绑定当前数据库句柄的 Known Hosts Repository。
func (s *Store) KnownHosts() repository.KnownHostRepository {
	return &knownHostRepository{store: s}
}

// JavaRuntimes 返回绑定当前数据库句柄的远端 Java 运行时 Repository。
func (s *Store) JavaRuntimes() repository.JavaRuntimeRepository {
	return &javaRuntimeRepository{store: s}
}

// MinecraftServers 返回绑定当前数据库句柄的 Server Repository。
func (s *Store) MinecraftServers() repository.MinecraftServerRepository {
	return &minecraftServerRepository{database: s.database}
}

// FirewallRuleLeases 返回远端防火墙规则归属 Repository。
func (s *Store) FirewallRuleLeases() repository.FirewallRuleLeaseRepository {
	return &firewallRuleLeaseRepository{database: s.database}
}

// Installations 返回绑定当前数据库句柄的安装任务 Repository。
func (s *Store) Installations() repository.InstallationRepository {
	return &installationRepository{store: s}
}

// ProxyCredentials 返回仅存于 SQLCipher 的出站代理凭据 Repository。
func (s *Store) ProxyCredentials() repository.ProxyCredentialRepository {
	return &proxyCredentialRepository{database: s.database}
}

// ProcessIdentities 返回持久化的远端进程身份 Repository。
func (s *Store) ProcessIdentities() repository.ProcessIdentityRepository {
	return &processIdentityRepository{database: s.database}
}

// Metrics 返回绑定到监控数据库的原始与聚合 Metric Repository。
func (s *Store) Metrics() repository.MetricRepository {
	return &metricRepository{database: s.metrics, series: s.series}
}

// Spark 返回 Minecraft spark 的能力、Snapshot 与报告 Repository。
func (s *Store) Spark() repository.SparkRepository {
	return &sparkRepository{database: s.database, metrics: s.metrics}
}

// Alerts 返回阈值规则与告警事件 Repository。
func (s *Store) Alerts() repository.AlertRepository {
	return &alertRepository{database: s.database}
}

// Players 返回玩家活动与名录 Repository。
func (s *Store) Players() repository.PlayerRepository {
	return &playerRepository{database: s.database}
}

// Transaction 在一个加密库事务内执行完整用例,Repository 均绑定该事务。
// 监控库不参与该事务:Registry 里的 Metrics 与 Spark Snapshot 仍走各自的监控库句柄,
// 需要监控侧原子性时用 MetricsTransaction。
func (s *Store) Transaction(ctx context.Context, action func(repository.Registry) error) error {
	if action == nil {
		return apperror.New(apperror.CodeValidationRequired, "事务操作不能为空")
	}
	return s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		return action(&Store{database: transaction, metrics: s.metrics, series: s.series})
	})
}

// MetricsTransaction 在一个监控库事务内执行监控数据写入。
func (s *Store) MetricsTransaction(ctx context.Context, action func(repository.Registry) error) error {
	if action == nil {
		return apperror.New(apperror.CodeValidationRequired, "事务操作不能为空")
	}
	return s.metrics.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		return action(&Store{database: s.database, metrics: transaction, series: s.series})
	})
}
