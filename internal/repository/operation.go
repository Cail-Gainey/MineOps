package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// OperationQuery 承载持久化 Operation 历史的过滤条件与分页参数。
type OperationQuery struct {
	TargetID model.ID
	Limit    int
	Offset   int
}

// OperationRepository 持久化并查询 Operation 状态。
type OperationRepository interface {
	Create(context.Context, *model.Operation) error
	Update(context.Context, *model.Operation) error
	Get(context.Context, model.ID) (*model.Operation, error)
	ListActive(context.Context, model.ID) ([]model.Operation, error)
	ListHistory(context.Context, OperationQuery) ([]model.Operation, error)
	DeleteHistory(context.Context, model.ID) error
	ClearHistory(context.Context) (int64, error)
}

// SettingsRepository 把每个带版本的设置分类存入加密 SQLite。
type SettingsRepository interface {
	Load(context.Context) (map[enums.SettingsCategory][]byte, error)
	Save(context.Context, enums.SettingsCategory, int, []byte) error
	Delete(context.Context, enums.SettingsCategory) error
}

// Registry 暴露绑定到同一个数据库事务的各个 Repository。
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

// Store 持有各 Repository 与完整用例事务,不向 Service 层暴露 GORM。
type Store interface {
	Registry
	Transaction(context.Context, func(Registry) error) error
	// MetricsTransaction 在未加密的监控库上开事务,供 Metric 与 Spark Snapshot 的原子写入使用。
	MetricsTransaction(context.Context, func(Registry) error) error
}
