// Package gormrepo implements MineOps repositories with GORM and SQLCipher.
package gormrepo

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
)

// Store is the GORM-backed repository registry and transaction boundary.
// 监控时序(Metric 三表与 Spark Snapshot)是纯数值、不含凭据或隐私,单独落在未加密的 metrics 库里,
// 完全不付 SQLCipher 逐页 AES + HMAC 的代价;其余含凭据、口令、玩家身份的表继续留在加密库。
type Store struct {
	database *gorm.DB
	metrics  *gorm.DB
	series   *MetricSeriesCache
}

// NewStore creates a repository store for the encrypted and monitoring GORM handles.
func NewStore(database, metrics *gorm.DB) (*Store, error) {
	if database == nil || metrics == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Repository 加密数据库和监控数据库不能为空")
	}
	return &Store{database: database, metrics: metrics, series: NewMetricSeriesCache()}, nil
}

// Operations returns the Operation repository bound to the current database handle.
func (s *Store) Operations() repository.OperationRepository {
	return &operationRepository{database: s.database}
}

// Settings returns the Settings repository bound to the current database handle.
func (s *Store) Settings() repository.SettingsRepository {
	return &settingsRepository{store: s}
}

// SSHSessions returns the SSH Session repository bound to the current database handle.
func (s *Store) SSHSessions() repository.SSHSessionRepository {
	return &sshSessionRepository{database: s.database}
}

// SSHCredentials returns the encrypted SSH Credential repository bound to the current database handle.
func (s *Store) SSHCredentials() repository.SSHCredentialRepository {
	return &sshCredentialRepository{database: s.database}
}

// KnownHosts returns the Known Hosts repository bound to the current database handle.
func (s *Store) KnownHosts() repository.KnownHostRepository {
	return &knownHostRepository{store: s}
}

// JavaRuntimes returns the remote Java repository bound to the current database handle.
func (s *Store) JavaRuntimes() repository.JavaRuntimeRepository {
	return &javaRuntimeRepository{store: s}
}

// MinecraftServers returns the server repository bound to the current database handle.
func (s *Store) MinecraftServers() repository.MinecraftServerRepository {
	return &minecraftServerRepository{database: s.database}
}

// FirewallRuleLeases returns the remote firewall ownership repository.
func (s *Store) FirewallRuleLeases() repository.FirewallRuleLeaseRepository {
	return &firewallRuleLeaseRepository{database: s.database}
}

// Installations returns the installation repository bound to the current database handle.
func (s *Store) Installations() repository.InstallationRepository {
	return &installationRepository{store: s}
}

// ProxyCredentials returns the SQLCipher-only outbound proxy credential repository.
func (s *Store) ProxyCredentials() repository.ProxyCredentialRepository {
	return &proxyCredentialRepository{database: s.database}
}

// ProcessIdentities returns the durable remote PID identity repository.
func (s *Store) ProcessIdentities() repository.ProcessIdentityRepository {
	return &processIdentityRepository{database: s.database}
}

// Metrics returns the raw and aggregate Metric repository bound to the monitoring database.
func (s *Store) Metrics() repository.MetricRepository {
	return &metricRepository{database: s.metrics, series: s.series}
}

// Spark returns the Minecraft spark capability, snapshot, and report repository.
func (s *Store) Spark() repository.SparkRepository {
	return &sparkRepository{database: s.database, metrics: s.metrics}
}

// Alerts returns the threshold rule and incident repository.
func (s *Store) Alerts() repository.AlertRepository {
	return &alertRepository{database: s.database}
}

// Players returns the player activity and directory repository.
func (s *Store) Players() repository.PlayerRepository {
	return &playerRepository{database: s.database}
}

// Transaction executes a complete use case using repositories bound to one encrypted-database transaction.
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

// MetricsTransaction executes monitoring writes inside one monitoring-database transaction.
func (s *Store) MetricsTransaction(ctx context.Context, action func(repository.Registry) error) error {
	if action == nil {
		return apperror.New(apperror.CodeValidationRequired, "事务操作不能为空")
	}
	return s.metrics.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		return action(&Store{database: s.database, metrics: transaction, series: s.series})
	})
}
