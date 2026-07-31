package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// OperationQuery contains durable Operation history filters and pagination.
type OperationQuery struct {
	TargetID model.ID
	Limit    int
	Offset   int
}

// OperationRepository persists and queries durable Operation state.
type OperationRepository interface {
	Create(context.Context, *model.Operation) error
	Update(context.Context, *model.Operation) error
	Get(context.Context, model.ID) (*model.Operation, error)
	ListActive(context.Context, model.ID) ([]model.Operation, error)
	ListHistory(context.Context, OperationQuery) ([]model.Operation, error)
	DeleteHistory(context.Context, model.ID) error
	ClearHistory(context.Context) (int64, error)
}

// SettingsRepository stores each versioned settings category in encrypted SQLite.
type SettingsRepository interface {
	Load(context.Context) (map[enums.SettingsCategory][]byte, error)
	Save(context.Context, enums.SettingsCategory, int, []byte) error
	Delete(context.Context, enums.SettingsCategory) error
}

// Registry exposes repositories bound to one database transaction.
type Registry interface {
	Operations() OperationRepository
	Settings() SettingsRepository
	SSHSessions() SSHSessionRepository
	SSHCredentials() SSHCredentialRepository
	KnownHosts() KnownHostRepository
	JavaRuntimes() JavaRuntimeRepository
	MinecraftServers() MinecraftServerRepository
	FirewallRuleLeases() FirewallRuleLeaseRepository
	Installations() InstallationRepository
	ProxyCredentials() ProxyCredentialRepository
	ProcessIdentities() ProcessIdentityRepository
	Metrics() MetricRepository
	Spark() SparkRepository
	Alerts() AlertRepository
	Players() PlayerRepository
}

// Store owns repositories and complete-use-case transactions without exposing GORM to services.
type Store interface {
	Registry
	Transaction(context.Context, func(Registry) error) error
	// MetricsTransaction 在未加密的监控库上开事务,供 Metric 与 Spark Snapshot 的原子写入使用。
	MetricsTransaction(context.Context, func(Registry) error) error
}
