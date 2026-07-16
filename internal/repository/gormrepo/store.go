// Package gormrepo implements MineOps repositories with GORM and SQLCipher.
package gormrepo

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
)

// Store is the GORM-backed repository registry and transaction boundary.
type Store struct {
	database *gorm.DB
}

// NewStore creates a repository store for a composition-root GORM handle.
func NewStore(database *gorm.DB) (*Store, error) {
	if database == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Repository 数据库不能为空")
	}
	return &Store{database: database}, nil
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

// Metrics returns the raw and aggregate Metric repository.
func (s *Store) Metrics() repository.MetricRepository {
	return &metricRepository{database: s.database}
}

// Spark returns the Minecraft spark capability, snapshot, and report repository.
func (s *Store) Spark() repository.SparkRepository {
	return &sparkRepository{database: s.database}
}

// Alerts returns the threshold rule and incident repository.
func (s *Store) Alerts() repository.AlertRepository {
	return &alertRepository{database: s.database}
}

// Players returns the player activity and directory repository.
func (s *Store) Players() repository.PlayerRepository {
	return &playerRepository{database: s.database}
}

// Transaction executes a complete use case using repositories bound to one GORM transaction.
func (s *Store) Transaction(ctx context.Context, action func(repository.Registry) error) error {
	if action == nil {
		return apperror.New(apperror.CodeValidationRequired, "事务操作不能为空")
	}
	return s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		return action(&Store{database: transaction})
	})
}
