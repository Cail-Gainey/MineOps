package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// RemoteProcessController 启动可恢复的分离进程,并经 SSH 校验 Linux PID 复用证据。
type RemoteProcessController struct {
	clock    model.Clock
	store    repository.Store
	clients  *SSHClientFactory
	settings *appsettings.Manager
}

// TmuxInstallPlan 描述探测到的远端包管理器与固定的 tmux 安装命令。
type TmuxInstallPlan struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version,omitempty"`
	OS             string `json:"os,omitempty"`
	PackageManager string `json:"packageManager,omitempty"`
	InstallCommand string `json:"installCommand,omitempty"`
	CanInstall     bool   `json:"canInstall"`
	RequiresSudo   bool   `json:"requiresSudo"`
}

// NewRemoteProcessController 创建生命周期与控制台共用的进程适配器。
func NewRemoteProcessController(clock model.Clock, store repository.Store, clients *SSHClientFactory, settings *appsettings.Manager) (*RemoteProcessController, error) {
	if clock == nil || store == nil || clients == nil || settings == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "RemoteProcessController 依赖不能为空")
	}
	return &RemoteProcessController{clock: clock, store: store, clients: clients, settings: settings}, nil
}

// Start 在专用 tmux 会话内拉起一个可恢复的 Minecraft 进程。
func (c *RemoteProcessController) Start(ctx context.Context, spec port.ProcessLaunchSpec) (port.InteractiveProcess, error) {
	if !spec.ServerID.Valid() || !spec.SSHSessionID.Valid() || strings.TrimSpace(spec.Executable) == "" || !path.IsAbs(spec.WorkingDirectory) {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Process Launch Spec 无效")
	}
	for _, argument := range spec.Arguments {
		if strings.ContainsAny(argument, "\x00\r\n") {
			return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Process Argument 包含非法控制字符")
		}
	}
	for key, value := range spec.Environment {
		if !validEnvironmentName(key) || strings.ContainsAny(value, "\x00\r\n") {
			return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Process Environment 无效")
		}
	}
	server, err := c.store.MinecraftServers().Get(ctx, spec.ServerID, false)
	if err != nil {
		return nil, err
	}
	if server.SSHSessionID != spec.SSHSessionID || server.RemotePath != spec.WorkingDirectory {
		return nil, apperror.New(apperror.CodeValidationConflict, "Process Launch Spec 与 Server 绑定不一致")
	}
	if existing, getErr := c.store.ProcessIdentities().GetByServer(ctx, spec.ServerID); getErr == nil {
		probe, probeErr := c.Probe(ctx, *existing)
		if probeErr != nil {
			return nil, probeErr
		}
		if probe.Identity.State == enums.RemoteProcessRunning {
			return nil, apperror.New(apperror.CodeValidationConflict, "Minecraft Server 进程已经运行").WithDetails(map[string]any{"pid": existing.PID})
		}
	} else if apperror.ToDTO(getErr).Code != apperror.CodeIONotFound.String() {
		return nil, getErr
	}
	session, client, err := c.connect(ctx, spec.SSHSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	fingerprint := processFingerprint(spec)
	tmuxSession := tmuxSessionName(spec.ServerID)
	exitFile := path.Join(spec.WorkingDirectory, ".mineops-exit-code")
	arguments := strings.Join(spec.Arguments, "\n")
	environment := encodeProcessEnvironment(spec.Environment)
	innerScript := `set -u
exit_file=$1
executable=$2
arguments=$3
environment=$4
set --
while IFS= read -r argument; do [ -z "$argument" ] || set -- "$@" "$argument"; done <<EOF
$arguments
EOF
if [ -n "$environment" ]; then
  while IFS= read -r assignment; do [ -z "$assignment" ] || export "$assignment"; done <<EOF
$environment
EOF
fi
rm -f -- "$exit_file"
set +e
"$executable" "$@"
exit_code=$?
printf '%s\n' "$exit_code" > "$exit_file"
exit "$exit_code"`
	commandLine := buildTmuxShellCommand(innerScript, fingerprint, exitFile, spec.Executable, arguments, environment)
	_, _ = client.RunCommand(ctx, RemoteCommand{Executable: "tmux", Arguments: []string{"kill-session", "-t", tmuxSession}, Timeout: 5 * time.Second, MaximumOutput: 16 * 1024})
	_, err = client.RunCommand(ctx, RemoteCommand{
		Executable: "tmux", Arguments: []string{"new-session", "-d", "-s", tmuxSession, "-c", spec.WorkingDirectory, commandLine},
		Timeout: 15 * time.Second, MaximumOutput: 64 * 1024,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeProcessStartFailed, "启动远程 Minecraft tmux Session 失败", err).WithDetails(map[string]any{"tmuxSession": tmuxSession})
	}
	identityResult, err := c.waitTmuxIdentity(ctx, client, tmuxSession)
	if err != nil {
		_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{Executable: "tmux", Arguments: []string{"kill-session", "-t", tmuxSession}, Timeout: 5 * time.Second})
		return nil, err
	}
	fields := strings.Fields(identityResult)
	if len(fields) != 3 {
		return nil, apperror.New(apperror.CodeProcessStartFailed, "远程 tmux 进程身份响应无效").WithDetails(map[string]any{"stdout": identityResult})
	}
	pid, pidErr := strconv.Atoi(fields[0])
	processGroupID, groupErr := strconv.Atoi(fields[1])
	startTicks, ticksErr := strconv.ParseUint(fields[2], 10, 64)
	if pidErr != nil || groupErr != nil || ticksErr != nil {
		return nil, apperror.New(apperror.CodeProcessStartFailed, "远程进程 PID 身份无法解析")
	}
	identity, err := model.NewRemoteProcessIdentity(c.clock, model.RemoteProcessIdentity{
		ServerID: spec.ServerID, SSHSessionID: session.ID, PID: pid, ProcessGroupID: processGroupID,
		LinuxStartTicks: startTicks, CommandFingerprint: fingerprint, WorkingDirectory: spec.WorkingDirectory,
		TmuxSession: tmuxSession,
	})
	if err != nil {
		return nil, err
	}
	if err := c.store.ProcessIdentities().Save(ctx, identity); err != nil {
		_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{Executable: "tmux", Arguments: []string{"kill-session", "-t", tmuxSession}, Timeout: 5 * time.Second})
		return nil, err
	}
	interactive := &remoteInteractiveProcess{controller: c, identity: *identity}
	if len(spec.ReadyPatterns) > 0 {
		if err := c.waitReady(ctx, client, identity, spec.ReadyPatterns, 2*time.Minute); err != nil {
			_ = c.Stop(context.WithoutCancel(ctx), *identity, true)
			return nil, err
		}
	}
	return interactive, nil
}

// Probe 校验持久化的 tmux 会话及其所属 pane 的进程身份。
func (c *RemoteProcessController) Probe(ctx context.Context, identity model.RemoteProcessIdentity) (port.ProcessProbeResult, error) {
	if identity.TmuxSession == "" {
		return c.probeLegacyProcess(ctx, identity)
	}
	_, client, err := c.connect(ctx, identity.SSHSessionID)
	if err != nil {
		return port.ProcessProbeResult{}, err
	}
	defer func() { _ = client.Close() }()
	script := `set -eu
tmux_session=$1
if ! tmux has-session -t "$tmux_session" 2>/dev/null; then printf 'exited\n'; exit 0; fi
pid=$(tmux display-message -p -t "$tmux_session:0.0" '#{pane_pid}')
if [ -z "$pid" ] || [ ! -r "/proc/$pid/stat" ]; then printf 'mismatched\n'; exit 0; fi
start_ticks=$(awk '{print $22}' "/proc/$pid/stat")
cwd=$(readlink "/proc/$pid/cwd")
command=$(tr '\000' ' ' < "/proc/$pid/cmdline")
process_group=$(ps -o pgid= -p "$pid" | tr -d ' ')
printf 'running\n%s\n%s\n%s\n%s\n%s\n' "$pid" "$start_ticks" "$process_group" "$cwd" "$command"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", identity.TmuxSession},
		Timeout: 10 * time.Second, MaximumOutput: 128 * 1024,
	})
	if err != nil {
		return port.ProcessProbeResult{}, apperror.Wrap(apperror.CodeProcessExitFailed, "探测远程进程失败", err)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	state := enums.RemoteProcessExited
	if len(lines) > 0 && lines[0] == "mismatched" {
		state = enums.RemoteProcessMismatched
	} else if len(lines) >= 6 && lines[0] == "running" {
		pid, _ := strconv.Atoi(strings.TrimSpace(lines[1]))
		startTicks, _ := strconv.ParseUint(strings.TrimSpace(lines[2]), 10, 64)
		processGroupID, _ := strconv.Atoi(strings.TrimSpace(lines[3]))
		workingDirectory := path.Clean(strings.TrimSpace(lines[4]))
		commandLine := strings.Join(lines[5:], "\n")
		if pid == identity.PID && startTicks == identity.LinuxStartTicks && processGroupID == identity.ProcessGroupID && workingDirectory == identity.WorkingDirectory && strings.Contains(commandLine, identity.CommandFingerprint) {
			state = enums.RemoteProcessRunning
		} else {
			state = enums.RemoteProcessMismatched
		}
	}
	lastOutput := identity.LastOutput
	if state == enums.RemoteProcessRunning {
		if captured, captureErr := c.captureTmuxPane(ctx, client, identity.TmuxSession); captureErr == nil {
			lastOutput = captured
		}
	}
	var exitCode *int
	if state == enums.RemoteProcessExited {
		exitPath := path.Join(identity.WorkingDirectory, ".mineops-exit-code")
		exitResult, exitErr := client.RunCommand(ctx, RemoteCommand{Executable: "cat", Arguments: []string{exitPath}, Timeout: 5 * time.Second, MaximumOutput: 4096})
		if exitErr == nil {
			if value, parseErr := strconv.Atoi(strings.TrimSpace(exitResult.Stdout)); parseErr == nil {
				exitCode = &value
			}
		}
	}
	if err := identity.ApplyProbe(c.clock, state, lastOutput, exitCode); err != nil {
		return port.ProcessProbeResult{}, err
	}
	if err := c.store.ProcessIdentities().Save(ctx, &identity); err != nil {
		return port.ProcessProbeResult{}, err
	}
	return port.ProcessProbeResult{Identity: identity, Output: lastOutput}, nil
}

func (c *RemoteProcessController) probeLegacyProcess(ctx context.Context, identity model.RemoteProcessIdentity) (port.ProcessProbeResult, error) {
	_, client, err := c.connect(ctx, identity.SSHSessionID)
	if err != nil {
		return port.ProcessProbeResult{}, err
	}
	defer func() { _ = client.Close() }()
	script := `set -eu
pid=$1
if [ ! -r "/proc/$pid/stat" ]; then printf 'exited\n'; exit 0; fi
start_ticks=$(awk '{print $22}' "/proc/$pid/stat")
cwd=$(readlink "/proc/$pid/cwd")
command=$(tr '\000' ' ' < "/proc/$pid/cmdline")
process_group=$(ps -o pgid= -p "$pid" | tr -d ' ')
printf 'running\n%s\n%s\n%s\n%s\n' "$start_ticks" "$process_group" "$cwd" "$command"`
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", script, "mineops", strconv.Itoa(identity.PID)}, Timeout: 10 * time.Second, MaximumOutput: 128 * 1024})
	if err != nil {
		return port.ProcessProbeResult{}, apperror.Wrap(apperror.CodeProcessExitFailed, "探测旧版远程进程失败", err)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	state := enums.RemoteProcessExited
	if len(lines) >= 5 && lines[0] == "running" {
		startTicks, _ := strconv.ParseUint(strings.TrimSpace(lines[1]), 10, 64)
		processGroupID, _ := strconv.Atoi(strings.TrimSpace(lines[2]))
		workingDirectory := path.Clean(strings.TrimSpace(lines[3]))
		commandLine := strings.Join(lines[4:], "\n")
		if startTicks == identity.LinuxStartTicks && processGroupID == identity.ProcessGroupID && workingDirectory == identity.WorkingDirectory && strings.Contains(commandLine, identity.CommandFingerprint) {
			state = enums.RemoteProcessRunning
		} else {
			state = enums.RemoteProcessMismatched
		}
	}
	lastOutput := c.tailOutput(ctx, client, identity.ConsoleLog, 64*1024)
	var exitCode *int
	if state == enums.RemoteProcessExited {
		exitPath := path.Join(identity.WorkingDirectory, ".mineops-exit-code")
		if exitResult, exitErr := client.RunCommand(ctx, RemoteCommand{Executable: "cat", Arguments: []string{exitPath}, Timeout: 5 * time.Second, MaximumOutput: 4096}); exitErr == nil {
			if value, parseErr := strconv.Atoi(strings.TrimSpace(exitResult.Stdout)); parseErr == nil {
				exitCode = &value
			}
		}
	}
	if err := identity.ApplyProbe(c.clock, state, lastOutput, exitCode); err != nil {
		return port.ProcessProbeResult{}, err
	}
	if err := c.store.ProcessIdentities().Save(ctx, &identity); err != nil {
		return port.ProcessProbeResult{}, err
	}
	return port.ProcessProbeResult{Identity: identity, Output: lastOutput}, nil
}

// Stop 经 tmux 下发 Minecraft `stop` 命令,必要时升级为结束会话与进程组。
func (c *RemoteProcessController) Stop(ctx context.Context, identity model.RemoteProcessIdentity, force bool) error {
	if identity.TmuxSession == "" {
		return c.stopLegacyProcess(ctx, identity, force)
	}
	probe, err := c.Probe(ctx, identity)
	if err != nil {
		return err
	}
	if probe.Identity.State == enums.RemoteProcessExited {
		return nil
	}
	if probe.Identity.State == enums.RemoteProcessMismatched {
		return apperror.New(apperror.CodeValidationConflict, "PID 已被复用，拒绝停止不匹配的远程进程")
	}
	_, client, err := c.connect(ctx, identity.SSHSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if !force {
		server, serverErr := c.store.MinecraftServers().Get(ctx, identity.ServerID, false)
		if serverErr != nil {
			return serverErr
		}
		writeErr := c.sendTmuxLine(ctx, client, identity.TmuxSession, runtimeProfileForServerType(server.Type).stopCommand)
		if writeErr == nil {
			for attempt := 0; attempt < 60; attempt++ {
				probeResult, probeErr := c.Probe(ctx, identity)
				if probeErr == nil && probeResult.Identity.State == enums.RemoteProcessExited {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
		}
	}
	_, _ = client.RunCommand(ctx, RemoteCommand{Executable: "tmux", Arguments: []string{"kill-session", "-t", identity.TmuxSession}, Timeout: 10 * time.Second, MaximumOutput: 16 * 1024})
	for attempt := 0; attempt < 10; attempt++ {
		probeResult, probeErr := c.Probe(ctx, identity)
		if probeErr == nil && probeResult.Identity.State == enums.RemoteProcessExited {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if err := c.terminateProcessGroup(ctx, client, identity.ProcessGroupID, "KILL"); err != nil {
		return err
	}
	probeResult, probeErr := c.Probe(ctx, identity)
	if probeErr == nil && probeResult.Identity.State == enums.RemoteProcessExited {
		return nil
	}
	return apperror.New(apperror.CodeProcessExitFailed, "tmux Session 已终止但远程进程仍未确认退出")
}

func (c *RemoteProcessController) stopLegacyProcess(ctx context.Context, identity model.RemoteProcessIdentity, force bool) error {
	probe, err := c.Probe(ctx, identity)
	if err != nil || probe.Identity.State == enums.RemoteProcessExited {
		return err
	}
	if probe.Identity.State == enums.RemoteProcessMismatched {
		return apperror.New(apperror.CodeValidationConflict, "PID 已被复用，拒绝停止不匹配的旧版远程进程")
	}
	_, client, err := c.connect(ctx, identity.SSHSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if !force {
		server, serverErr := c.store.MinecraftServers().Get(ctx, identity.ServerID, false)
		if serverErr != nil {
			return serverErr
		}
		writeScript := `set -eu
fifo=$1
stop_command=$2
test -p "$fifo"
printf '%s\n' "$stop_command" > "$fifo"`
		_, writeErr := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", writeScript, "mineops", identity.ConsoleFIFO, runtimeProfileForServerType(server.Type).stopCommand}, Timeout: 10 * time.Second})
		if writeErr == nil {
			for attempt := 0; attempt < 60; attempt++ {
				probeResult, probeErr := c.Probe(ctx, identity)
				if probeErr == nil && probeResult.Identity.State == enums.RemoteProcessExited {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
		}
	}
	if err := c.terminateProcessGroup(ctx, client, identity.ProcessGroupID, "TERM"); err != nil {
		return err
	}
	for attempt := 0; attempt < 10; attempt++ {
		probeResult, probeErr := c.Probe(ctx, identity)
		if probeErr == nil && probeResult.Identity.State == enums.RemoteProcessExited {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return c.terminateProcessGroup(ctx, client, identity.ProcessGroupID, "KILL")
}

// Attach 对一个已持久化的远端进程身份创建控制台附着。
func (c *RemoteProcessController) Attach(identity model.RemoteProcessIdentity) port.InteractiveProcess {
	return &remoteInteractiveProcess{controller: c, identity: identity}
}

// EnsureTmux 校验 tmux,并在显式确认后经受支持的包管理器安装它。
func (c *RemoteProcessController) EnsureTmux(ctx context.Context, sshSessionID model.ID, installConfirmed bool) (TmuxInstallPlan, error) {
	_, client, err := c.connect(ctx, sshSessionID)
	if err != nil {
		return TmuxInstallPlan{}, err
	}
	defer func() { _ = client.Close() }()
	plan, err := c.detectTmux(ctx, client)
	if err != nil || plan.Installed {
		return plan, err
	}
	details := map[string]any{
		"requiresTmuxInstallConfirmation": true,
		"tmuxInstall":                     plan,
		"intent":                          "MineOps 将通过远程系统包管理器安装 tmux，并在安装成功后继续启动 Minecraft Server。",
		"os":                              plan.OS,
		"packageManager":                  plan.PackageManager,
		"command":                         plan.InstallCommand,
		"needsSudo":                       plan.RequiresSudo,
		"canInstall":                      plan.CanInstall,
	}
	if !plan.CanInstall {
		details["requiresTmuxInstallConfirmation"] = false
		return plan, apperror.New(apperror.CodeInstallationPreflightFailed, "远程服务器未安装 tmux，且无法自动安装").WithDetails(details)
	}
	if !installConfirmed {
		return plan, apperror.New(apperror.CodeValidationConflict, "启动 Minecraft Server 前需要确认安装 tmux").WithDetails(details)
	}
	script := tmuxInstallScript(plan.PackageManager)
	command := RemoteCommand{Executable: "sh", Arguments: []string{"-c", script, "mineops"}, Timeout: 5 * time.Minute, MaximumOutput: 256 * 1024}
	if plan.RequiresSudo {
		command = RemoteCommand{Executable: "sudo", Arguments: []string{"-n", "sh", "-c", script, "mineops"}, Timeout: 5 * time.Minute, MaximumOutput: 256 * 1024}
	}
	if result, installErr := client.RunCommand(ctx, command); installErr != nil {
		return plan, apperror.Wrap(apperror.CodeInstallationPreflightFailed, "自动安装 tmux 失败", installErr).WithDetails(map[string]any{"tmuxInstall": plan, "stderr": result.Stderr})
	}
	verified, err := c.detectTmux(ctx, client)
	if err != nil || !verified.Installed {
		if err == nil {
			err = apperror.New(apperror.CodeInstallationPreflightFailed, "tmux 安装后验证失败")
		}
		return plan, err
	}
	return verified, nil
}

func (c *RemoteProcessController) detectTmux(ctx context.Context, client *SSHClient) (TmuxInstallPlan, error) {
	script := `set -u
if command -v tmux >/dev/null 2>&1; then printf 'installed\t%s\n' "$(tmux -V 2>/dev/null)"; exit 0; fi
os=$(sed -n 's/^ID=//p' /etc/os-release 2>/dev/null | head -n 1 | tr -d '"' || true)
manager=
case "$os" in
  debian|ubuntu) command -v apt-get >/dev/null 2>&1 && manager=apt-get ;;
  fedora|rhel|rocky|almalinux) command -v dnf >/dev/null 2>&1 && manager=dnf ;;
  centos|amzn) command -v yum >/dev/null 2>&1 && manager=yum ;;
  arch|manjaro) command -v pacman >/dev/null 2>&1 && manager=pacman ;;
  alpine) command -v apk >/dev/null 2>&1 && manager=apk ;;
  opensuse*|sles) command -v zypper >/dev/null 2>&1 && manager=zypper ;;
esac
if [ -z "$manager" ]; then for candidate in apt-get dnf yum pacman apk zypper; do if command -v "$candidate" >/dev/null 2>&1; then manager=$candidate; break; fi; done; fi
privilege=none
if [ "$(id -u)" = 0 ]; then privilege=root; elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then privilege=sudo; fi
printf 'missing\t%s\t%s\t%s\n' "$os" "$manager" "$privilege"`
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", script, "mineops"}, Timeout: 15 * time.Second, MaximumOutput: 16 * 1024})
	if err != nil {
		return TmuxInstallPlan{}, apperror.Wrap(apperror.CodeInstallationPreflightFailed, "探测远程 tmux 失败", err)
	}
	return parseTmuxDiscovery(result.Stdout)
}

func parseTmuxDiscovery(raw string) (TmuxInstallPlan, error) {
	fields := strings.Split(strings.TrimSpace(raw), "\t")
	if len(fields) == 2 && fields[0] == "installed" {
		return TmuxInstallPlan{Installed: true, Version: fields[1], CanInstall: true}, nil
	}
	if len(fields) != 4 || fields[0] != "missing" {
		return TmuxInstallPlan{}, apperror.New(apperror.CodeInstallationPreflightFailed, "远程 tmux 探测响应无效")
	}
	manager, privilege := fields[2], fields[3]
	requiresSudo := privilege != "root"
	command := tmuxInstallScript(manager)
	if command != "" && requiresSudo {
		command = "sudo -n sh -c " + quotePOSIX(command)
	}
	return TmuxInstallPlan{OS: fields[1], PackageManager: manager, InstallCommand: command, CanInstall: manager != "" && (privilege == "root" || privilege == "sudo"), RequiresSudo: requiresSudo}, nil
}

func tmuxInstallScript(manager string) string {
	switch manager {
	case "apt-get":
		return "apt-get update && apt-get install -y tmux"
	case "dnf":
		return "dnf install -y tmux"
	case "yum":
		return "yum install -y tmux"
	case "pacman":
		return "pacman -S --noconfirm tmux"
	case "apk":
		return "apk add --no-cache tmux"
	case "zypper":
		return "zypper --non-interactive install tmux"
	default:
		return ""
	}
}

func (c *RemoteProcessController) connect(ctx context.Context, sshSessionID model.ID) (*model.SSHSession, *SSHClient, error) {
	session, err := c.store.SSHSessions().Get(ctx, sshSessionID)
	if err != nil {
		return nil, nil, err
	}
	client, err := c.clients.Connect(ctx, session, c.settings.Snapshot().SSH)
	if err != nil {
		return nil, nil, err
	}
	return session, client, nil
}

func (c *RemoteProcessController) waitReady(ctx context.Context, client *SSHClient, identity *model.RemoteProcessIdentity, patterns []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probe, err := c.Probe(ctx, *identity)
		if err != nil {
			return err
		}
		if probe.Identity.State != enums.RemoteProcessRunning {
			return apperror.New(apperror.CodeProcessExitFailed, "Minecraft 进程在就绪前退出").WithDetails(map[string]any{"output": probe.Output})
		}
		for _, pattern := range patterns {
			if strings.Contains(probe.Output, pattern) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return apperror.New(apperror.CodeProcessStartFailed, "Minecraft 进程等待就绪超时").WithRetryable(true)
}

func (c *RemoteProcessController) waitTmuxIdentity(ctx context.Context, client *SSHClient, session string) (string, error) {
	script := `set -eu
tmux_session=$1
pid=$(tmux display-message -p -t "$tmux_session:0.0" '#{pane_pid}')
test -r "/proc/$pid/stat"
process_group=$(ps -o pgid= -p "$pid" | tr -d ' ')
start_ticks=$(awk '{print $22}' "/proc/$pid/stat")
printf '%s %s %s\n' "$pid" "$process_group" "$start_ticks"`
	var lastError error
	for attempt := 0; attempt < 20; attempt++ {
		result, err := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", script, "mineops", session}, Timeout: 5 * time.Second, MaximumOutput: 4096})
		if err == nil {
			return strings.TrimSpace(result.Stdout), nil
		}
		lastError = err
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return "", apperror.Wrap(apperror.CodeProcessStartFailed, "读取远程 tmux 进程身份失败", lastError)
}

func (c *RemoteProcessController) captureTmuxPane(ctx context.Context, client *SSHClient, session string) (string, error) {
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "tmux", Arguments: []string{"capture-pane", "-p", "-J", "-S", "-2000", "-t", session + ":0.0"}, Timeout: 10 * time.Second, MaximumOutput: 512 * 1024})
	if err != nil {
		return "", apperror.Wrap(apperror.CodeIOReadFailed, "读取 tmux Console 输出失败", err)
	}
	if result.Truncated {
		return "", apperror.New(apperror.CodeIOReadFailed, "tmux Console 输出超过安全读取上限")
	}
	return result.Stdout, nil
}

func (c *RemoteProcessController) sendTmuxLine(ctx context.Context, client *SSHClient, session, line string) error {
	if strings.ContainsAny(line, "\x00\r\n") {
		return apperror.New(apperror.CodeValidationInvalidArgument, "tmux Console 命令包含非法控制字符")
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "tmux", Arguments: []string{"send-keys", "-t", session + ":0.0", "-l", "--", line}, Timeout: 10 * time.Second, MaximumOutput: 16 * 1024}); err != nil {
		return apperror.Wrap(apperror.CodeProcessExitFailed, "写入 tmux Console 命令失败", err)
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "tmux", Arguments: []string{"send-keys", "-t", session + ":0.0", "Enter"}, Timeout: 10 * time.Second, MaximumOutput: 16 * 1024}); err != nil {
		return apperror.Wrap(apperror.CodeProcessExitFailed, "提交 tmux Console 命令失败", err)
	}
	return nil
}

func (c *RemoteProcessController) tailOutput(ctx context.Context, client *SSHClient, consoleLog string, maximum int) string {
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "tail", Arguments: []string{"-c", strconv.Itoa(maximum), consoleLog},
		Timeout: 5 * time.Second, MaximumOutput: maximum,
	})
	if err != nil && strings.TrimSpace(result.Stdout) == "" {
		return ""
	}
	return result.Stdout
}

func (c *RemoteProcessController) terminateProcessGroup(ctx context.Context, client *SSHClient, processGroupID int, signal string) error {
	_, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "kill", Arguments: []string{"-" + signal, "--", "-" + strconv.Itoa(processGroupID)},
		Timeout: 10 * time.Second, MaximumOutput: 16 * 1024,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeProcessExitFailed, "终止远程进程组失败", err).WithDetails(map[string]any{"signal": signal, "processGroupID": processGroupID})
	}
	return nil
}

type remoteInteractiveProcess struct {
	controller *RemoteProcessController
	identity   model.RemoteProcessIdentity

	mu       sync.Mutex
	detached bool
	closed   bool
	client   *SSHClient
}

// Identity 返回该远端进程的持久化身份。
func (p *remoteInteractiveProcess) Identity() model.RemoteProcessIdentity { return p.identity }

// Input 向远端进程写入一段输入。
func (p *remoteInteractiveProcess) Input(ctx context.Context, value []byte) error {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return apperror.New(apperror.CodeValidationConflict, "Console Session 已关闭")
	}
	if len(value) == 0 || len(value) > 64*1024 || strings.ContainsRune(string(value), '\x00') {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Console Input 无效")
	}
	if p.identity.TmuxSession == "" {
		return apperror.New(apperror.CodeValidationConflict, "旧版进程 Console 不支持 tmux 命令输入")
	}
	_, client, err := p.controller.connect(ctx, p.identity.SSHSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	lines := strings.Split(strings.ReplaceAll(string(value), "\r\n", "\n"), "\n")
	for index, line := range lines {
		if index == len(lines)-1 && line == "" {
			break
		}
		if err := p.controller.sendTmuxLine(ctx, client, p.identity.TmuxSession, line); err != nil {
			return err
		}
	}
	return nil
}

// Attach 从指定偏移开始把远端进程输出转写到 writer,返回新的偏移。
func (p *remoteInteractiveProcess) Attach(ctx context.Context, offset int64, writer io.Writer) (int64, error) {
	if writer == nil || offset < 0 || p.identity.TmuxSession == "" {
		return offset, apperror.New(apperror.CodeValidationInvalidArgument, "Console Attach Writer 或 Offset 无效")
	}
	_, client, err := p.controller.connect(ctx, p.identity.SSHSessionID)
	if err != nil {
		return offset, err
	}
	p.mu.Lock()
	if p.detached || p.closed {
		p.mu.Unlock()
		_ = client.Close()
		return offset, nil
	}
	p.client = client
	p.mu.Unlock()
	defer func() {
		_ = client.Close()
		p.mu.Lock()
		if p.client == client {
			p.client = nil
		}
		p.mu.Unlock()
	}()

	previous := ""
	for {
		p.mu.Lock()
		detached := p.detached || p.closed
		p.mu.Unlock()
		if detached {
			return offset, nil
		}
		snapshot, captureErr := p.controller.captureTmuxPane(ctx, client, p.identity.TmuxSession)
		if captureErr != nil {
			p.mu.Lock()
			detached = p.detached || p.closed
			p.mu.Unlock()
			if detached || ctx.Err() != nil {
				return offset, nil
			}
			probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			probe, probeErr := p.controller.Probe(probeCtx, p.identity)
			cancel()
			if probeErr == nil && probe.Identity.State == enums.RemoteProcessExited {
				return offset, nil
			}
			return offset, captureErr
		}
		delta := tmuxSnapshotDelta(previous, snapshot)
		if delta != "" {
			written, writeErr := io.WriteString(writer, delta)
			offset += int64(written)
			if writeErr != nil {
				return offset, writeErr
			}
		}
		previous = snapshot
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return offset, nil
		case <-timer.C:
		}
	}
}

// Detach 断开本地转写但保留远端进程继续运行。
func (p *remoteInteractiveProcess) Detach() error {
	p.mu.Lock()
	p.detached = true
	client := p.client
	p.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
	return nil
}

// Close 关闭本地资源并停止转写。
func (p *remoteInteractiveProcess) Close() error {
	p.mu.Lock()
	p.closed = true
	client := p.client
	p.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
	return nil
}

func tmuxSnapshotDelta(previous, current string) string {
	if current == "" || current == previous {
		return ""
	}
	if previous == "" {
		return current
	}
	if strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	previousLines := tmuxSnapshotLines(previous)
	currentLines := tmuxSnapshotLines(current)
	maximum := len(previousLines)
	if len(currentLines) < maximum {
		maximum = len(currentLines)
	}
	for overlap := maximum; overlap > 0; overlap-- {
		matched := true
		for index := 0; index < overlap; index++ {
			if previousLines[len(previousLines)-overlap+index] != currentLines[index] {
				matched = false
				break
			}
		}
		if matched {
			return strings.Join(currentLines[overlap:], "")
		}
	}
	return current
}

func tmuxSnapshotLines(value string) []string {
	lines := strings.SplitAfter(value, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func processFingerprint(spec port.ProcessLaunchSpec) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, spec.ServerID.String()+"\x00"+spec.Executable+"\x00"+spec.WorkingDirectory+"\x00")
	for _, argument := range spec.Arguments {
		_, _ = io.WriteString(hash, argument+"\x00")
	}
	_, _ = io.WriteString(hash, encodeProcessEnvironment(spec.Environment))
	return hex.EncodeToString(hash.Sum(nil))[:24]
}

func tmuxSessionName(serverID model.ID) string {
	return "mineops-" + strings.ReplaceAll(serverID.String(), "-", "")
}

func buildTmuxShellCommand(script, fingerprint string, arguments ...string) string {
	parts := []string{"exec", quotePOSIX("sh"), quotePOSIX("-c"), quotePOSIX(script), quotePOSIX("mineops:" + fingerprint)}
	for _, argument := range arguments {
		parts = append(parts, quotePOSIX(argument))
	}
	return strings.Join(parts, " ")
}

func encodeProcessEnvironment(environment map[string]string) string {
	if len(environment) == 0 {
		return ""
	}
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+environment[key])
	}
	return strings.Join(lines, "\n")
}

var _ port.ProcessController = (*RemoteProcessController)(nil)
var _ port.ProcessProbe = (*RemoteProcessController)(nil)
var _ port.InteractiveProcess = (*remoteInteractiveProcess)(nil)
