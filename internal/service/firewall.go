package service

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// FirewallPortChangePlan captures the rule prepared before a server.properties port update.
type FirewallPortChangePlan struct {
	ServerID      model.ID
	OldPort       uint16
	NewStatus     port.FirewallStatus
	Managed       bool
	Created       bool
	RemoveOldPort bool
}

// FirewallManager detects, applies, owns, and safely releases remote Linux TCP rules.
type FirewallManager struct {
	clock    model.Clock
	store    repository.Store
	clients  *SSHClientFactory
	settings *appsettings.Manager
}

// NewFirewallManager creates the UFW/firewalld coordinator.
func NewFirewallManager(clock model.Clock, store repository.Store, clients *SSHClientFactory, settings *appsettings.Manager) (*FirewallManager, error) {
	if clock == nil || store == nil || clients == nil || settings == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "FirewallManager 依赖不能为空")
	}
	return &FirewallManager{clock: clock, store: store, clients: clients, settings: settings}, nil
}

// Detect reads server-port and distinguishes UFW, firewalld, or no active firewall.
func (m *FirewallManager) Detect(ctx context.Context, serverID model.ID) (port.FirewallStatus, error) {
	server, _, client, err := m.connect(ctx, serverID)
	if err != nil {
		return port.FirewallStatus{}, err
	}
	defer func() { _ = client.Close() }()
	propertiesResult, propertiesErr := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `if [ -f "$1" ]; then cat -- "$1"; fi`, "mineops", path.Join(server.RemotePath, "server.properties")},
		Timeout: 10 * time.Second, MaximumOutput: 2 * 1024 * 1024,
	})
	if propertiesErr != nil {
		return port.FirewallStatus{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取 server.properties 失败", propertiesErr)
	}
	serverPort, err := model.ParseProperties(propertiesResult.Stdout).ServerPort()
	if err != nil {
		return port.FirewallStatus{}, err
	}
	return m.DetectPort(ctx, serverID, serverPort)
}

// DetectPort probes the configured provider for one explicit TCP port.
func (m *FirewallManager) DetectPort(ctx context.Context, serverID model.ID, serverPort uint16) (port.FirewallStatus, error) {
	provider := m.settings.Snapshot().Firewall.Provider
	return m.detectPort(ctx, nil, serverID, serverPort, provider)
}

func (m *FirewallManager) detectPort(ctx context.Context, shared *SSHClient, serverID model.ID, serverPort uint16, provider string) (port.FirewallStatus, error) {
	server, session, err := m.target(ctx, serverID)
	if err != nil {
		return port.FirewallStatus{}, err
	}
	if provider == "disabled" {
		return port.FirewallStatus{ServerID: server.ID, SSHSessionID: session.ID, Backend: enums.FirewallBackendNone, Port: serverPort, Intent: "全局防火墙 Provider 已禁用"}, nil
	}
	client, release, err := m.clientFor(ctx, shared, session)
	if err != nil {
		return port.FirewallStatus{}, err
	}
	defer release()
	detectScript := `set -eu
provider=$1
port=$2
privileged=0
if [ "$(id -u)" = "0" ] || sudo -n true >/dev/null 2>&1; then privileged=1; fi
ufw_installed=0; ufw_active=0; ufw_allowed=0
if command -v ufw >/dev/null 2>&1; then
  ufw_installed=1
  status=$(ufw status 2>/dev/null || sudo -n ufw status 2>/dev/null || true)
  printf '%s' "$status" | grep -qi 'Status: active' && ufw_active=1
  printf '%s' "$status" | grep -Eq "(^|[[:space:]])$port/tcp([[:space:]]|$)" && ufw_allowed=1
fi
firewalld_installed=0; firewalld_active=0; firewalld_allowed=0
if command -v firewall-cmd >/dev/null 2>&1; then
  firewalld_installed=1
  firewall-cmd --state >/dev/null 2>&1 && firewalld_active=1
  if [ "$firewalld_active" = "1" ]; then firewall-cmd --query-port="$port/tcp" >/dev/null 2>&1 && firewalld_allowed=1; fi
fi
if [ "$provider" = "ufw" ]; then
  [ "$ufw_installed" = "1" ] || exit 44
  printf 'ufw %s %s %s\n' "$ufw_active" "$ufw_allowed" "$privileged"
  exit 0
fi
if [ "$provider" = "firewalld" ]; then
  [ "$firewalld_installed" = "1" ] || exit 44
  printf 'firewalld %s %s %s\n' "$firewalld_active" "$firewalld_allowed" "$privileged"
  exit 0
fi
if [ "$ufw_active" = "1" ]; then printf 'ufw %s %s %s\n' "$ufw_active" "$ufw_allowed" "$privileged"; exit 0; fi
if [ "$firewalld_active" = "1" ]; then printf 'firewalld %s %s %s\n' "$firewalld_active" "$firewalld_allowed" "$privileged"; exit 0; fi
if [ "$ufw_installed" = "1" ]; then printf 'ufw 0 %s %s\n' "$ufw_allowed" "$privileged"; exit 0; fi
if [ "$firewalld_installed" = "1" ]; then printf 'firewalld 0 %s %s\n' "$firewalld_allowed" "$privileged"; exit 0; fi
printf 'none 0 0 %s\n' "$privileged"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", detectScript, "mineops", provider, strconv.Itoa(int(serverPort))},
		Timeout: 15 * time.Second, MaximumOutput: 64 * 1024,
	})
	if err != nil {
		return port.FirewallStatus{}, apperror.Wrap(apperror.CodeFirewallUnsupported, "探测远程防火墙失败", err).WithDetails(map[string]any{"provider": provider})
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) != 4 {
		return port.FirewallStatus{}, apperror.New(apperror.CodeFirewallUnsupported, "防火墙探测响应无效")
	}
	status := port.FirewallStatus{
		ServerID: server.ID, SSHSessionID: session.ID, Backend: enums.FirewallBackend(fields[0]),
		Active: fields[1] == "1", AlreadyAllowed: fields[2] == "1", Privileged: fields[3] == "1", Port: serverPort,
	}
	if !status.Backend.Valid() {
		return port.FirewallStatus{}, apperror.New(apperror.CodeFirewallUnsupported, "未知防火墙 Backend")
	}
	status.Intent = "不修改防火墙"
	if status.Backend != enums.FirewallBackendNone && status.Active && !status.AlreadyAllowed {
		status.Intent = "幂等放行 TCP " + strconv.Itoa(int(serverPort))
	}
	return status, nil
}

// EnsureAllowed applies an idempotent UFW or firewalld runtime/permanent rule.
func (m *FirewallManager) EnsureAllowed(ctx context.Context, status port.FirewallStatus) error {
	return m.ensureAllowed(ctx, nil, status)
}

func (m *FirewallManager) ensureAllowed(ctx context.Context, shared *SSHClient, status port.FirewallStatus) error {
	if status.Backend == enums.FirewallBackendNone || !status.Active || status.AlreadyAllowed {
		return nil
	}
	if !status.Privileged {
		return apperror.New(apperror.CodeFirewallPermissionDenied, "防火墙修改需要 root 或无密码 sudo -n").WithDetails(map[string]any{"backend": status.Backend, "port": status.Port})
	}
	_, session, err := m.target(ctx, status.ServerID)
	if err != nil {
		return err
	}
	client, release, err := m.clientFor(ctx, shared, session)
	if err != nil {
		return err
	}
	defer release()
	script := `set -eu
backend=$1
port=$2
if [ "$(id -u)" = "0" ]; then runner=""; else runner="sudo -n"; fi
if [ "$backend" = "ufw" ]; then
  if [ -n "$runner" ]; then sudo -n ufw allow "$port/tcp"; else ufw allow "$port/tcp"; fi
  exit 0
fi
if [ "$backend" = "firewalld" ]; then
  if [ -n "$runner" ]; then
    sudo -n firewall-cmd --query-port="$port/tcp" >/dev/null 2>&1 || sudo -n firewall-cmd --add-port="$port/tcp"
    sudo -n firewall-cmd --permanent --query-port="$port/tcp" >/dev/null 2>&1 || sudo -n firewall-cmd --permanent --add-port="$port/tcp"
  else
    firewall-cmd --query-port="$port/tcp" >/dev/null 2>&1 || firewall-cmd --add-port="$port/tcp"
    firewall-cmd --permanent --query-port="$port/tcp" >/dev/null 2>&1 || firewall-cmd --permanent --add-port="$port/tcp"
  fi
  exit 0
fi
exit 43`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", status.Backend.String(), strconv.Itoa(int(status.Port))},
		Timeout: 30 * time.Second, MaximumOutput: 128 * 1024,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeFirewallApplyFailed, "应用防火墙 TCP 规则失败", err).WithDetails(map[string]any{"backend": status.Backend, "port": status.Port, "stderr": result.Stderr})
	}
	return nil
}

// RemoveAllowed removes one MineOps-owned UFW or firewalld rule.
func (m *FirewallManager) RemoveAllowed(ctx context.Context, status port.FirewallStatus) error {
	return m.removeAllowed(ctx, nil, status)
}

func (m *FirewallManager) removeAllowed(ctx context.Context, shared *SSHClient, status port.FirewallStatus) error {
	if status.Backend == enums.FirewallBackendNone || !status.Active || !status.AlreadyAllowed {
		return nil
	}
	if !status.Privileged {
		return apperror.New(apperror.CodeFirewallPermissionDenied, "防火墙修改需要 root 或无密码 sudo -n").WithDetails(map[string]any{"backend": status.Backend, "port": status.Port})
	}
	_, session, err := m.target(ctx, status.ServerID)
	if err != nil {
		return err
	}
	client, release, err := m.clientFor(ctx, shared, session)
	if err != nil {
		return err
	}
	defer release()
	script := `set -eu
backend=$1
port=$2
if [ "$(id -u)" = "0" ]; then runner=""; else runner="sudo -n"; fi
if [ "$backend" = "ufw" ]; then
  if [ -n "$runner" ]; then sudo -n ufw --force delete allow "$port/tcp"; else ufw --force delete allow "$port/tcp"; fi
  exit 0
fi
if [ "$backend" = "firewalld" ]; then
  if [ -n "$runner" ]; then
    sudo -n firewall-cmd --query-port="$port/tcp" >/dev/null 2>&1 && sudo -n firewall-cmd --remove-port="$port/tcp" || true
    sudo -n firewall-cmd --permanent --query-port="$port/tcp" >/dev/null 2>&1 && sudo -n firewall-cmd --permanent --remove-port="$port/tcp" || true
  else
    firewall-cmd --query-port="$port/tcp" >/dev/null 2>&1 && firewall-cmd --remove-port="$port/tcp" || true
    firewall-cmd --permanent --query-port="$port/tcp" >/dev/null 2>&1 && firewall-cmd --permanent --remove-port="$port/tcp" || true
  fi
  exit 0
fi
exit 43`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", status.Backend.String(), strconv.Itoa(int(status.Port))},
		Timeout: 30 * time.Second, MaximumOutput: 128 * 1024,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeFirewallApplyFailed, "移除防火墙 TCP 规则失败", err).WithDetails(map[string]any{"backend": status.Backend, "port": status.Port, "stderr": result.Stderr})
	}
	return nil
}

// AcquirePort ensures a rule and records whether MineOps owns it.
func (m *FirewallManager) AcquirePort(ctx context.Context, serverID model.ID, serverPort uint16) (port.FirewallStatus, error) {
	return m.acquirePort(ctx, nil, serverID, serverPort)
}

// AcquirePortWithClient ensures a rule over an already authenticated client so callers can reuse one SSH connection.
func (m *FirewallManager) AcquirePortWithClient(ctx context.Context, client *SSHClient, serverID model.ID, serverPort uint16) (port.FirewallStatus, error) {
	if client == nil {
		return port.FirewallStatus{}, apperror.New(apperror.CodeValidationRequired, "防火墙操作的 SSH Client 不能为空")
	}
	return m.acquirePort(ctx, client, serverID, serverPort)
}

func (m *FirewallManager) acquirePort(ctx context.Context, shared *SSHClient, serverID model.ID, serverPort uint16) (port.FirewallStatus, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return port.FirewallStatus{}, err
	}
	if server.FirewallPolicy == enums.FirewallDisabled || m.settings.Snapshot().Firewall.Provider == "disabled" {
		return port.FirewallStatus{ServerID: serverID, Backend: enums.FirewallBackendNone, Port: serverPort, Intent: "防火墙策略已禁用"}, nil
	}
	status, err := m.detectPort(ctx, shared, serverID, serverPort, m.settings.Snapshot().Firewall.Provider)
	if err != nil {
		return port.FirewallStatus{}, err
	}
	if status.Backend == enums.FirewallBackendNone || !status.Active {
		return status, nil
	}
	managed := !status.AlreadyAllowed
	if status.AlreadyAllowed {
		leases, listErr := m.store.FirewallRuleLeases().ListByRule(ctx, status.SSHSessionID, status.Backend, status.Port)
		if listErr != nil {
			return status, listErr
		}
		for _, lease := range leases {
			managed = managed || lease.Managed
		}
	}
	if err := m.ensureAllowed(ctx, shared, status); err != nil {
		return status, err
	}
	now := m.clock.Now().UTC()
	lease := model.FirewallRuleLease{
		ServerID: serverID, SSHSessionID: status.SSHSessionID, Backend: status.Backend, Port: status.Port,
		Managed: managed, CreatedAt: now, UpdatedAt: now,
	}
	if err := m.store.FirewallRuleLeases().Upsert(ctx, &lease); err != nil {
		if managed && !status.AlreadyAllowed {
			status.AlreadyAllowed = true
			_ = m.removeAllowed(context.WithoutCancel(ctx), shared, status)
		}
		return status, err
	}
	return status, nil
}

// PreparePortChange validates confirmation and opens the new rule before configuration is saved.
func (m *FirewallManager) PreparePortChange(ctx context.Context, serverID model.ID, oldPort, newPort uint16, confirmed bool) (*FirewallPortChangePlan, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, err
	}
	settings := m.settings.Snapshot().Firewall
	if oldPort == newPort || !settings.SyncOnPortChange || settings.Provider == "disabled" || server.FirewallPolicy == enums.FirewallDisabled {
		return nil, nil
	}
	requiresConfirmation := server.FirewallPolicy == enums.FirewallPrompt || settings.RemoveOldPort && settings.RequireDestructiveConfirm
	if requiresConfirmation && !confirmed {
		return nil, apperror.New(apperror.CodeValidationConflict, "修改服务端口前需要确认防火墙同步").WithDetails(map[string]any{
			"requiresFirewallPortChangeConfirmation": true, "oldPort": oldPort, "newPort": newPort,
			"removeOldPort": settings.RemoveOldPort,
		})
	}
	status, err := m.DetectPort(ctx, serverID, newPort)
	if err != nil {
		return nil, err
	}
	managed := !status.AlreadyAllowed
	if status.AlreadyAllowed && status.Backend != enums.FirewallBackendNone {
		leases, listErr := m.store.FirewallRuleLeases().ListByRule(ctx, status.SSHSessionID, status.Backend, status.Port)
		if listErr != nil {
			return nil, listErr
		}
		for _, lease := range leases {
			managed = managed || lease.Managed
		}
	}
	if err := m.EnsureAllowed(ctx, status); err != nil {
		return nil, err
	}
	return &FirewallPortChangePlan{
		ServerID: serverID, OldPort: oldPort, NewStatus: status, Managed: managed,
		Created: !status.AlreadyAllowed && status.Active, RemoveOldPort: settings.RemoveOldPort,
	}, nil
}

// AbortPortChange compensates a newly created rule when configuration saving fails.
func (m *FirewallManager) AbortPortChange(ctx context.Context, plan *FirewallPortChangePlan) {
	if plan == nil || !plan.Created || plan.NewStatus.Backend == enums.FirewallBackendNone {
		return
	}
	leases, err := m.store.FirewallRuleLeases().ListByRule(ctx, plan.NewStatus.SSHSessionID, plan.NewStatus.Backend, plan.NewStatus.Port)
	if err == nil && len(leases) == 0 {
		status := plan.NewStatus
		status.AlreadyAllowed = true
		_ = m.RemoveAllowed(context.WithoutCancel(ctx), status)
	}
}

// CommitPortChange records the new lease and safely releases the old MineOps-owned rule.
func (m *FirewallManager) CommitPortChange(ctx context.Context, plan *FirewallPortChangePlan) error {
	if plan == nil || plan.NewStatus.Backend == enums.FirewallBackendNone || !plan.NewStatus.Active {
		return nil
	}
	now := m.clock.Now().UTC()
	lease := model.FirewallRuleLease{
		ServerID: plan.ServerID, SSHSessionID: plan.NewStatus.SSHSessionID, Backend: plan.NewStatus.Backend, Port: plan.NewStatus.Port,
		Managed: plan.Managed, CreatedAt: now, UpdatedAt: now,
	}
	if err := m.store.FirewallRuleLeases().Upsert(ctx, &lease); err != nil {
		return err
	}
	if !plan.RemoveOldPort || plan.OldPort == 0 || plan.OldPort == plan.NewStatus.Port {
		return nil
	}
	return m.releaseServerPort(ctx, plan.ServerID, plan.OldPort, false)
}

// CleanupPending removes deferred old rules after a Server has successfully started on its new port.
func (m *FirewallManager) CleanupPending(ctx context.Context, serverID model.ID) error {
	leases, err := m.store.FirewallRuleLeases().ListByServer(ctx, serverID)
	if err != nil {
		return err
	}
	for _, lease := range leases {
		if lease.PendingRemoval {
			if err := m.releaseLease(ctx, lease, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// PrepareStart enforces Disabled/Prompt/Automatic without removing shared rules.
func (m *FirewallManager) PrepareStart(ctx context.Context, serverID model.ID, confirmed bool) (port.FirewallStatus, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return port.FirewallStatus{}, err
	}
	if server.FirewallPolicy == enums.FirewallDisabled {
		return port.FirewallStatus{ServerID: serverID, Backend: enums.FirewallBackendNone, Intent: "防火墙策略已禁用"}, nil
	}
	status, err := m.Detect(ctx, serverID)
	if err != nil {
		return port.FirewallStatus{}, err
	}
	if server.FirewallPolicy == enums.FirewallPrompt && status.Active && !status.AlreadyAllowed && !confirmed {
		return status, apperror.New(apperror.CodeValidationConflict, "启动前需要确认防火墙放行").WithDetails(map[string]any{
			"requiresFirewallConfirmation": true, "backend": status.Backend, "port": status.Port,
			"intent": status.Intent, "privileged": status.Privileged,
		})
	}
	if server.FirewallPolicy == enums.FirewallAutomatic || confirmed {
		return m.AcquirePort(ctx, serverID, status.Port)
	}
	return status, nil
}

func (m *FirewallManager) releaseServerPort(ctx context.Context, serverID model.ID, oldPort uint16, force bool) error {
	leases, err := m.store.FirewallRuleLeases().ListByServer(ctx, serverID)
	if err != nil {
		return err
	}
	for _, lease := range leases {
		if lease.Port == oldPort {
			return m.releaseLease(ctx, lease, force)
		}
	}
	return nil
}

func (m *FirewallManager) releaseLease(ctx context.Context, lease model.FirewallRuleLease, force bool) error {
	server, err := m.store.MinecraftServers().Get(ctx, lease.ServerID, false)
	if err != nil {
		return err
	}
	processActive := false
	if !force {
		if _, identityErr := m.store.ProcessIdentities().GetByServer(ctx, lease.ServerID); identityErr == nil {
			processActive = true
		} else if apperror.ToDTO(identityErr).Code != apperror.CodeIONotFound.String() {
			return identityErr
		}
	}
	if !force && (processActive || server.State == enums.LifecycleStarting || server.State == enums.LifecycleRunning || server.State == enums.LifecycleStopping) {
		lease.PendingRemoval = true
		lease.UpdatedAt = m.clock.Now().UTC()
		return m.store.FirewallRuleLeases().Upsert(ctx, &lease)
	}
	if !lease.Managed {
		return m.store.FirewallRuleLeases().Delete(ctx, lease.ServerID, lease.Backend, lease.Port)
	}
	references, err := m.store.FirewallRuleLeases().ListByRule(ctx, lease.SSHSessionID, lease.Backend, lease.Port)
	if err != nil {
		return err
	}
	if len(references) > 1 {
		return m.store.FirewallRuleLeases().Delete(ctx, lease.ServerID, lease.Backend, lease.Port)
	}
	status, err := m.detectPort(ctx, nil, lease.ServerID, lease.Port, lease.Backend.String())
	if err != nil {
		lease.PendingRemoval = true
		lease.UpdatedAt = m.clock.Now().UTC()
		_ = m.store.FirewallRuleLeases().Upsert(context.WithoutCancel(ctx), &lease)
		return err
	}
	if err := m.RemoveAllowed(ctx, status); err != nil {
		lease.PendingRemoval = true
		lease.UpdatedAt = m.clock.Now().UTC()
		_ = m.store.FirewallRuleLeases().Upsert(context.WithoutCancel(ctx), &lease)
		return err
	}
	return m.store.FirewallRuleLeases().Delete(ctx, lease.ServerID, lease.Backend, lease.Port)
}

func (m *FirewallManager) connect(ctx context.Context, serverID model.ID) (*model.MinecraftServer, *model.SSHSession, *SSHClient, error) {
	server, session, err := m.target(ctx, serverID)
	if err != nil {
		return nil, nil, nil, err
	}
	client, _, err := m.clientFor(ctx, nil, session)
	if err != nil {
		return nil, nil, nil, err
	}
	return server, session, client, nil
}

// target reads the Minecraft Server and its SSH Session rows without opening a connection.
func (m *FirewallManager) target(ctx context.Context, serverID model.ID) (*model.MinecraftServer, *model.SSHSession, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, nil, err
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return nil, nil, err
	}
	return server, session, nil
}

// clientFor returns the caller's shared client with a no-op release, or a freshly dialed client the caller must release.
func (m *FirewallManager) clientFor(ctx context.Context, shared *SSHClient, session *model.SSHSession) (*SSHClient, func(), error) {
	if shared != nil {
		return shared, func() {}, nil
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return nil, nil, err
	}
	return client, func() { _ = client.Close() }, nil
}

var _ port.FirewallDetector = (*FirewallManager)(nil)
var _ port.FirewallController = (*FirewallManager)(nil)
