package service

import (
	"context"
	"fmt"
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

// ServerCatalogRegistry is the application-facing dynamic distribution catalog contract.
type ServerCatalogRegistry interface {
	List() []port.ServerDistribution
	ResolveVersions(context.Context, enums.MinecraftServerType) ([]port.ServerVersion, error)
	ResolveArtifact(context.Context, enums.MinecraftServerType, string, string) (port.ServerArtifact, error)
}

// InstallationManager exposes catalog resolution and the durable ten-step remote installation workflow.
type InstallationManager struct {
	clock        model.Clock
	store        repository.Store
	settings     *appsettings.Manager
	clients      *SSHClientFactory
	javaRuntimes *JavaRuntimeManager
	jdkCatalog   port.JDKCatalog
	catalogs     ServerCatalogRegistry
	runner       *InstallationRunner
	firewall     *FirewallManager
}

// NewInstallationManager creates the installation application service and registers every standard step.
func NewInstallationManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, clients *SSHClientFactory, javaRuntimes *JavaRuntimeManager, jdkCatalog port.JDKCatalog, catalogs ServerCatalogRegistry, runner *InstallationRunner, firewall *FirewallManager) (*InstallationManager, error) {
	if clock == nil || store == nil || settings == nil || clients == nil || javaRuntimes == nil || jdkCatalog == nil || catalogs == nil || runner == nil || firewall == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "InstallationManager 依赖不能为空")
	}
	manager := &InstallationManager{
		clock: clock, store: store, settings: settings, clients: clients,
		javaRuntimes: javaRuntimes, jdkCatalog: jdkCatalog, catalogs: catalogs, runner: runner, firewall: firewall,
	}
	handlers := map[string]InstallationStepHandler{
		"connect_ssh": manager.connectSSH, "initialize_directories": manager.initializeDirectories,
		"create_server_directory": manager.createServerDirectory, "resolve_java": manager.resolveJava,
		"install_java": manager.installJava, "download_server": manager.downloadServer,
		"install_server": manager.installServer, "write_eula": manager.writeEULA,
		"configure_firewall": manager.configureFirewall, "first_start": manager.firstStart, "register_server": manager.registerServer,
	}
	for name, handler := range handlers {
		if err := runner.RegisterStep(name, handler); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

// ListDistributions returns the dynamic first-release server type registry.
func (m *InstallationManager) ListDistributions() []port.ServerDistribution {
	return m.catalogs.List()
}

// ResolveVersions returns provider-neutral versions for one registered distribution.
func (m *InstallationManager) ResolveVersions(ctx context.Context, distribution enums.MinecraftServerType) ([]port.ServerVersion, error) {
	if !distribution.Valid() {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "服务端类型无效")
	}
	return m.catalogs.ResolveVersions(ctx, distribution)
}

// Start performs the installation preflight before creating a durable asynchronous task.
func (m *InstallationManager) Start(ctx context.Context, serverID model.ID) (InstallationStartResult, error) {
	server, session, client, err := m.connectServer(ctx, serverID)
	if err != nil {
		return InstallationStartResult{}, err
	}
	defer func() { _ = client.Close() }()
	if server.State != enums.LifecycleCreating && server.State != enums.LifecycleFailed {
		return InstallationStartResult{}, apperror.New(apperror.CodeValidationConflict, "只有 Creating 或 Failed Server 可以开始安装")
	}
	preflightScript := `set -eu
home=$(printenv HOME)
test -n "$home"
test -w "$home"
for command in sh mkdir mv cp find chmod sha256sum curl tar awk df wc head dirname grep tail mkfifo seq sleep kill setsid; do command -v "$command" >/dev/null 2>&1 || { printf 'missing:%s\n' "$command" >&2; exit 21; }; done
available=$(df -Pk "$home" | awk 'NR==2 {print $4}')
test "${available:-0}" -ge 524288
printf '%s\n' "$home"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", preflightScript}, Timeout: 20 * time.Second, MaximumOutput: 32 * 1024,
	})
	if err != nil {
		return InstallationStartResult{}, apperror.Wrap(apperror.CodeInstallationPreflightFailed, "安装前能力探测失败", err).WithDetails(map[string]any{
			"sshSessionID": session.ID, "stderr": result.Stderr,
		})
	}
	if server.Type == enums.ServerSpigot {
		if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "git", Arguments: []string{"--version"}, Timeout: 5 * time.Second, MaximumOutput: 4096}); err != nil {
			return InstallationStartResult{}, apperror.Wrap(apperror.CodeProcessStartFailed, "Spigot BuildTools 需要远程 Git", err)
		}
	}
	server.State = enums.LifecycleInstalling
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return InstallationStartResult{}, err
	}
	started, err := m.runner.Start(ctx, serverID)
	if err != nil {
		server.State = enums.LifecycleFailed
		server.UpdatedAt = m.clock.Now().UTC()
		_ = m.store.MinecraftServers().Update(context.WithoutCancel(ctx), server)
		return InstallationStartResult{}, err
	}
	return started, nil
}

// Cancel propagates installation cancellation to the active Operation.
func (m *InstallationManager) Cancel(ctx context.Context, operationID model.ID) error {
	return m.runner.Cancel(ctx, operationID)
}

// Get returns one durable InstallationTask aggregate and its ordered steps.
func (m *InstallationManager) Get(ctx context.Context, taskID model.ID) (*model.InstallationTask, []model.InstallationStep, error) {
	return m.store.Installations().Get(ctx, taskID)
}

// ListByServer returns recent durable installation tasks for one Minecraft Server.
func (m *InstallationManager) ListByServer(ctx context.Context, serverID model.ID, limit, offset int) ([]model.InstallationTask, error) {
	if !serverID.Valid() {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Minecraft Server ID 无效")
	}
	return m.store.Installations().ListByServer(ctx, serverID, limit, offset)
}

// Retry performs preflight again and resumes only failed, cancelled, or waiting steps.
func (m *InstallationManager) Retry(ctx context.Context, taskID model.ID) (InstallationStartResult, error) {
	task, _, err := m.store.Installations().Get(ctx, taskID)
	if err != nil {
		return InstallationStartResult{}, err
	}
	server, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return InstallationStartResult{}, err
	}
	_ = client.Close()
	server.State = enums.LifecycleInstalling
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return InstallationStartResult{}, err
	}
	return m.runner.Retry(ctx, taskID)
}

// ResolveDirectoryConflict applies backup, rename, or cancel without ever deleting the existing directory.
func (m *InstallationManager) ResolveDirectoryConflict(ctx context.Context, taskID model.ID, action, newName string) (InstallationStartResult, error) {
	task, steps, err := m.store.Installations().Get(ctx, taskID)
	if err != nil {
		return InstallationStartResult{}, err
	}
	if task.State != enums.InstallationFailed {
		return InstallationStartResult{}, apperror.New(apperror.CodeValidationConflict, "只有 Failed Installation Task 可以处理目录冲突")
	}
	var conflictStep *model.InstallationStep
	for index := range steps {
		if steps[index].Name == "create_server_directory" && steps[index].State == enums.InstallationStepFailed {
			conflictStep = &steps[index]
			break
		}
	}
	if conflictStep == nil {
		return InstallationStartResult{}, apperror.New(apperror.CodeValidationConflict, "Installation Task 当前没有目录冲突")
	}
	server, err := m.store.MinecraftServers().Get(ctx, task.ServerID, false)
	if err != nil {
		return InstallationStartResult{}, err
	}
	switch action {
	case "backup":
		if conflictStep.Checkpoint == nil {
			conflictStep.Checkpoint = make(map[string]any)
		}
		conflictStep.Checkpoint["resolution"] = "backup"
	case "rename":
		directoryName := model.SafeServerDirectoryName(newName)
		if strings.TrimSpace(newName) == "" || directoryName == "server" && strings.TrimSpace(newName) != "server" {
			return InstallationStartResult{}, apperror.New(apperror.CodeValidationInvalidArgument, "新的 Server 名称无效")
		}
		server.Name = strings.TrimSpace(newName)
		server.DirectoryName = directoryName
		server.RemotePath = path.Join(path.Dir(server.RemotePath), directoryName)
		server.LaunchProfile.WorkingDirectory = server.RemotePath
		server.State = enums.LifecycleFailed
		server.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
			return InstallationStartResult{}, err
		}
		if conflictStep.Checkpoint == nil {
			conflictStep.Checkpoint = make(map[string]any)
		}
		conflictStep.Checkpoint["resolution"] = "rename"
		conflictStep.Checkpoint["newRemotePath"] = server.RemotePath
	case "cancel":
		if err := conflictStep.CancelAfterFailure(m.clock); err != nil {
			return InstallationStartResult{}, err
		}
		if err := task.CancelAfterFailure(m.clock); err != nil {
			return InstallationStartResult{}, err
		}
		if err := m.store.Installations().UpdateStep(ctx, conflictStep); err != nil {
			return InstallationStartResult{}, err
		}
		if err := m.store.Installations().UpdateTask(ctx, task); err != nil {
			return InstallationStartResult{}, err
		}
		return InstallationStartResult{TaskID: task.ID}, nil
	default:
		return InstallationStartResult{}, apperror.New(apperror.CodeValidationInvalidArgument, "目录冲突处理动作无效")
	}
	if err := m.store.Installations().UpdateStep(ctx, conflictStep); err != nil {
		return InstallationStartResult{}, err
	}
	return m.Retry(ctx, taskID)
}

func (m *InstallationManager) connectSSH(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	_, session, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	if err := reporter.SetProgress(1, "SSH 连接、认证、主机指纹与命令通道验证完成", 1); err != nil {
		return nil, err
	}
	return map[string]any{"sshSessionID": session.ID.String(), "host": session.Host, "port": session.Port}, nil
}

func (m *InstallationManager) initializeDirectories(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	_, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	homeResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "printenv", Arguments: []string{"HOME"}, Timeout: 5 * time.Second, MaximumOutput: 4096})
	if err != nil || strings.TrimSpace(homeResult.Stdout) == "" {
		return nil, apperror.Wrap(apperror.CodeIOPermissionDenied, "无法解析远程 Home", err)
	}
	home := path.Clean(strings.TrimSpace(homeResult.Stdout))
	directories := []string{"Servers", "Runtime", "Downloads", "Logs", "Backup", "Temp", "Agents"}
	arguments := []string{"-p"}
	for _, directory := range directories {
		arguments = append(arguments, path.Join(home, "MineOps", directory))
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "mkdir", Arguments: arguments, Timeout: 15 * time.Second}); err != nil {
		return nil, apperror.Wrap(apperror.CodeIOPermissionDenied, "初始化 MineOps 远程目录失败", err)
	}
	if err := reporter.SetProgress(1, "MineOps 远程目录已幂等初始化", 2); err != nil {
		return nil, err
	}
	return map[string]any{"home": home, "root": path.Join(home, "MineOps")}, nil
}

func (m *InstallationManager) createServerDirectory(ctx context.Context, task model.InstallationTask, step model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	marker := path.Join(server.RemotePath, ".mineops-installation")
	resolution := checkpointString(step.Checkpoint, "resolution")
	initializeCheckpoint, checkpointErr := m.stepCheckpoint(ctx, task.ID, "initialize_directories")
	if checkpointErr != nil {
		return nil, checkpointErr
	}
	backupPath := path.Join(checkpointString(initializeCheckpoint, "root"), "Backup", server.DirectoryName+"-"+task.ID.String())
	script := `set -eu
directory=$1
marker=$2
task=$3
	resolution=$4
	backup=$5
if [ -e "$directory" ]; then
  if [ -f "$marker" ] && [ "$(cat "$marker")" = "$task" ]; then exit 0; fi
	  if [ "$resolution" = "backup" ]; then mkdir -p -- "$(dirname "$backup")"; mv -- "$directory" "$backup"; else
  printf 'directory-conflict:%s\n' "$directory" >&2
  exit 22
	  fi
fi
mkdir -p -- "$directory"
printf '%s' "$task" > "$marker"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", server.RemotePath, marker, task.ID.String(), resolution, backupPath},
		Timeout: 15 * time.Second, MaximumOutput: 16 * 1024,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInstallationDirectoryConflict, "服务器目录已存在或无法创建", err).WithDetails(map[string]any{
			"remotePath": server.RemotePath, "stderr": result.Stderr,
		})
	}
	if err := reporter.SetProgress(1, "服务器目录已创建并写入安装标记", 3); err != nil {
		return nil, err
	}
	return map[string]any{"remotePath": server.RemotePath, "marker": marker, "resolution": resolution, "backupPath": backupPath}, nil
}

func (m *InstallationManager) resolveJava(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, err := m.store.MinecraftServers().Get(ctx, task.ServerID, false)
	if err != nil {
		return nil, err
	}
	requirement, err := model.JavaRequirementForMinecraft(server.Type.String(), server.Version)
	if err != nil {
		return nil, err
	}
	var runtime *model.JavaRuntime
	if server.JavaRuntimeID != nil {
		runtime, err = m.store.JavaRuntimes().Get(ctx, *server.JavaRuntimeID)
		if err == nil && !requirement.Compatible(runtime.MajorVersion) {
			err = apperror.New(apperror.CodeValidationConflict, "已选择的 Java Runtime 与服务端版本不兼容")
		}
	} else {
		runtime, err = m.javaRuntimes.Recommend(ctx, server.SSHSessionID, server.Type.String(), server.Version)
		if err != nil && apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
			candidates, discoverErr := m.javaRuntimes.Discover(ctx, server.SSHSessionID)
			if discoverErr != nil {
				return nil, discoverErr
			}
			for _, candidate := range candidates {
				if !requirement.Compatible(candidate.Info.MajorVersion) {
					continue
				}
				runtime, err = m.javaRuntimes.Import(ctx, server.SSHSessionID, candidate.ExecutablePath, candidate.Source)
				if err == nil {
					break
				}
			}
		}
	}
	checkpoint := map[string]any{"requiredMajor": requirement.PreferredMajor}
	if err == nil && runtime != nil {
		server.JavaRuntimeID = &runtime.ID
		server.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
			return nil, err
		}
		checkpoint["javaRuntimeID"] = runtime.ID.String()
		checkpoint["executablePath"] = path.Join(runtime.JavaHome, "bin/java")
		checkpoint["installRequired"] = false
	} else if apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
		checkpoint["installRequired"] = true
	} else if err != nil {
		return nil, err
	}
	if err := reporter.SetProgress(1, "Java Runtime 需求与复用候选已解析", 4); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func (m *InstallationManager) installJava(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	resolveCheckpoint, err := m.stepCheckpoint(ctx, task.ID, "resolve_java")
	if err != nil {
		return nil, err
	}
	if !checkpointBool(resolveCheckpoint, "installRequired") {
		if err := reporter.SetProgress(1, "已复用兼容 Java Runtime", 5); err != nil {
			return nil, err
		}
		return resolveCheckpoint, nil
	}
	major := checkpointInt(resolveCheckpoint, "requiredMajor")
	architectureResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "uname", Arguments: []string{"-m"}, Timeout: 5 * time.Second, MaximumOutput: 4096})
	if err != nil {
		return nil, err
	}
	architecture := strings.TrimSpace(architectureResult.Stdout)
	artifact, err := m.jdkCatalog.Resolve(ctx, major, architecture, "linux")
	if err != nil {
		return nil, err
	}
	homeResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "printenv", Arguments: []string{"HOME"}, Timeout: 5 * time.Second, MaximumOutput: 4096})
	if err != nil {
		return nil, err
	}
	home := path.Clean(strings.TrimSpace(homeResult.Stdout))
	archive := path.Join(home, "MineOps", "Downloads", fmt.Sprintf("temurin-%d-%s.%s", major, architecture, artifact.ArchiveType))
	target := path.Join(home, "MineOps", "Runtime", fmt.Sprintf("java-%d-%s", major, architecture))
	script := `set -eu
url=$1
archive=$2
expected=$3
size=$4
target=$5
temporary="${archive}.part"
extract="${target}.extract"
rm -f -- "$temporary"
curl --fail --location --retry 2 --connect-timeout 15 --output "$temporary" "$url" &
download_pid=$!
while kill -0 "$download_pid" 2>/dev/null; do bytes=$(wc -c < "$temporary" 2>/dev/null | tr -d ' ' || printf '0'); printf 'MINEOPS_PROGRESS %s\n' "${bytes:-0}" >&2; sleep 1; done
wait "$download_pid"
actual_size=$(wc -c < "$temporary" | tr -d ' ')
[ "$size" = "0" ] || [ "$actual_size" = "$size" ]
actual_hash=$(sha256sum "$temporary" | awk '{print $1}')
[ "$actual_hash" = "$expected" ]
mv -f -- "$temporary" "$archive"
rm -rf -- "$extract"
mkdir -p -- "$extract"
tar -xzf "$archive" -C "$extract"
java_path=$(find "$extract" -type f -path '*/bin/java' | head -n 1)
[ -n "$java_path" ]
java_root=$(dirname "$(dirname "$java_path")")
rm -rf -- "$target"
mv -- "$java_root" "$target"
rm -rf -- "$extract"
printf '%s\n' "$actual_hash"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", artifact.URL, archive, artifact.SHA256, strconv.FormatInt(artifact.Size, 10), target},
		Timeout: 20 * time.Minute, MaximumOutput: 128 * 1024,
		OnOutput: downloadOutputReporter(reporter, artifact.Size, "OpenJDK"),
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInstallationJavaFailed, "下载或安装 OpenJDK 失败", err).WithDetails(map[string]any{"stderr": result.Stderr})
	}
	runtime, err := m.javaRuntimes.Import(ctx, server.SSHSessionID, path.Join(target, "bin/java"), model.JavaSourceManaged)
	if err != nil {
		return nil, err
	}
	server.JavaRuntimeID = &runtime.ID
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return nil, err
	}
	if err := reporter.SetProgress(1, "OpenJDK 已校验、安装并注册", 5); err != nil {
		return nil, err
	}
	return map[string]any{
		"javaRuntimeID": runtime.ID.String(), "executablePath": path.Join(target, "bin/java"),
		"artifactSHA256": strings.TrimSpace(result.Stdout), "installRequired": true,
	}, nil
}

func (m *InstallationManager) downloadServer(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	artifact, err := m.catalogs.ResolveArtifact(ctx, server.Type, server.Version, "")
	if err != nil {
		return nil, err
	}
	homeResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "printenv", Arguments: []string{"HOME"}, Timeout: 5 * time.Second, MaximumOutput: 4096})
	if err != nil {
		return nil, err
	}
	fileName := path.Base(artifact.FileName)
	if fileName == "." || fileName == "/" || fileName == "" {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Server Artifact 文件名无效")
	}
	destination := path.Join(strings.TrimSpace(homeResult.Stdout), "MineOps", "Downloads", fileName)
	script := `set -eu
url=$1
destination=$2
expected=$3
expected_size=$4
maximum=$5
temporary="${destination}.part"
if [ -f "$destination" ]; then
  size=$(wc -c < "$destination" | tr -d ' ')
  hash=$(sha256sum "$destination" | awk '{print $1}')
  if { [ "$expected_size" = "0" ] || [ "$size" = "$expected_size" ]; } && { [ -z "$expected" ] || [ "$hash" = "$expected" ]; }; then
    printf '%s %s cache\n' "$hash" "$size"
    exit 0
  fi
fi
rm -f -- "$temporary"
curl --fail --location --retry 2 --connect-timeout 15 --output "$temporary" "$url" &
download_pid=$!
while kill -0 "$download_pid" 2>/dev/null; do bytes=$(wc -c < "$temporary" 2>/dev/null | tr -d ' ' || printf '0'); printf 'MINEOPS_PROGRESS %s\n' "${bytes:-0}" >&2; sleep 1; done
wait "$download_pid"
size=$(wc -c < "$temporary" | tr -d ' ')
[ "$size" -le "$maximum" ]
[ "$expected_size" = "0" ] || [ "$size" = "$expected_size" ]
hash=$(sha256sum "$temporary" | awk '{print $1}')
[ -z "$expected" ] || [ "$hash" = "$expected" ]
mv -f -- "$temporary" "$destination"
printf '%s %s downloaded\n' "$hash" "$size"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", artifact.URL, destination, artifact.SHA256, strconv.FormatInt(artifact.Size, 10), strconv.FormatInt(2*1024*1024*1024, 10)},
		Timeout: 20 * time.Minute, MaximumOutput: 128 * 1024,
		OnOutput: downloadOutputReporter(reporter, artifact.Size, "Server 下载"),
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInstallationArtifactFailed, "下载或校验 Server Artifact 失败", err).WithDetails(map[string]any{"stderr": result.Stderr})
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) < 2 {
		return nil, apperror.New(apperror.CodeArtifactChecksumMismatch, "Server Artifact 校验结果缺失")
	}
	if err := reporter.SetProgress(1, "Server Artifact 已下载并完成 SHA-256 校验", 6); err != nil {
		return nil, err
	}
	return map[string]any{
		"distribution": artifact.Distribution.String(), "gameVersion": artifact.GameVersion,
		"build": artifact.Build, "fileName": fileName, "downloadPath": destination,
		"artifactSHA256": fields[0], "size": fields[1], "sourceRisk": artifact.SourceRisk,
	}, nil
}

func (m *InstallationManager) installServer(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	download, err := m.stepCheckpoint(ctx, task.ID, "download_server")
	if err != nil {
		return nil, err
	}
	source := checkpointString(download, "downloadPath")
	destination := path.Join(server.RemotePath, server.LaunchProfile.JarPath)
	if server.Type == enums.ServerForge || server.Type == enums.ServerNeoForge {
		javaCheckpoint, checkpointErr := m.stepCheckpoint(ctx, task.ID, "install_java")
		if checkpointErr != nil {
			return nil, checkpointErr
		}
		javaExecutable := checkpointString(javaCheckpoint, "executablePath")
		if javaExecutable == "" {
			resolveCheckpoint, resolveErr := m.stepCheckpoint(ctx, task.ID, "resolve_java")
			if resolveErr != nil {
				return nil, resolveErr
			}
			javaExecutable = checkpointString(resolveCheckpoint, "executablePath")
		}
		jvmArguments := []string{fmt.Sprintf("-Xms%dM", server.LaunchProfile.XmsMiB), fmt.Sprintf("-Xmx%dM", server.LaunchProfile.XmxMiB)}
		jvmArguments = append(jvmArguments, server.LaunchProfile.JVMArguments...)
		installerScript := `set -eu
java=$1
installer=$2
jvm_arguments=$3
if ! "$java" -jar "$installer" --installServer > .mineops-installer.log 2>&1; then cat .mineops-installer.log >&2; exit 42; fi
test -f run.sh
printf '%s\n' "$jvm_arguments" > user_jvm_args.txt
chmod u+x run.sh
hash=$(sha256sum "$installer" | awk '{print $1}')
printf '%s run.sh\n' "$hash"`
		result, installErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh", Arguments: []string{"-c", installerScript, "mineops", javaExecutable, source, strings.Join(jvmArguments, "\n")},
			WorkingDirectory: server.RemotePath, Timeout: 30 * time.Minute, MaximumOutput: 2 * 1024 * 1024,
			OnOutput: installationOutputReporter(reporter, 0.6, "Forge Installer"),
		})
		if installErr != nil {
			return nil, apperror.Wrap(apperror.CodeProcessExitFailed, "Forge Installer 执行失败", installErr).WithDetails(map[string]any{
				"stdout": result.Stdout, "stderr": result.Stderr,
			})
		}
		fields := strings.Fields(result.Stdout)
		if len(fields) == 0 {
			return nil, apperror.New(apperror.CodeArtifactChecksumMismatch, "Forge Installer 摘要缺失")
		}
		if err := reporter.SetProgress(1, "Forge Installer 已执行并生成结构化启动文件", 7); err != nil {
			return nil, err
		}
		return map[string]any{"launcher": "run.sh", "artifactSHA256": fields[0], "installerLog": ".mineops-installer.log"}, nil
	}
	if server.Type == enums.ServerSpigot {
		javaCheckpoint, checkpointErr := m.stepCheckpoint(ctx, task.ID, "install_java")
		if checkpointErr != nil {
			return nil, checkpointErr
		}
		javaExecutable := checkpointString(javaCheckpoint, "executablePath")
		if javaExecutable == "" {
			resolveCheckpoint, resolveErr := m.stepCheckpoint(ctx, task.ID, "resolve_java")
			if resolveErr != nil {
				return nil, resolveErr
			}
			javaExecutable = checkpointString(resolveCheckpoint, "executablePath")
		}
		buildScript := `set -eu
java=$1
buildtools=$2
revision=$3
destination=$4
if ! "$java" -jar "$buildtools" --rev "$revision" > .mineops-buildtools.log 2>&1; then cat .mineops-buildtools.log >&2; exit 41; fi
artifact=$(find . -maxdepth 1 -type f -name 'spigot-*.jar' ! -name '*-remapped.jar' | head -n 1)
[ -n "$artifact" ]
temporary="${destination}.part"
cp -- "$artifact" "$temporary"
hash=$(sha256sum "$temporary" | awk '{print $1}')
mv -f -- "$temporary" "$destination"
printf '%s %s\n' "$hash" "$artifact"`
		result, buildErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh", Arguments: []string{"-c", buildScript, "mineops", javaExecutable, source, server.Version, destination},
			WorkingDirectory: server.RemotePath, Timeout: 60 * time.Minute, MaximumOutput: 2 * 1024 * 1024,
			OnOutput: installationOutputReporter(reporter, 0.6, "BuildTools"),
		})
		if buildErr != nil {
			return nil, apperror.Wrap(apperror.CodeProcessExitFailed, "Spigot BuildTools 构建失败", buildErr).WithDetails(map[string]any{
				"stdout": result.Stdout, "stderr": result.Stderr,
			})
		}
		fields := strings.Fields(result.Stdout)
		if len(fields) == 0 {
			return nil, apperror.New(apperror.CodeArtifactChecksumMismatch, "Spigot 构建产物摘要缺失")
		}
		if err := reporter.SetProgress(1, "Spigot BuildTools 已完成构建并安装实际 Jar", 7); err != nil {
			return nil, err
		}
		return map[string]any{"jarPath": destination, "artifactSHA256": fields[0], "buildOutput": result.Stdout}, nil
	}
	script := `set -eu
source=$1
destination=$2
expected=$3
temporary="${destination}.part"
test -f "$source"
mkdir -p -- "$(dirname "$destination")"
cp -- "$source" "$temporary"
actual=$(sha256sum "$temporary" | awk '{print $1}')
[ "$actual" = "$expected" ]
mv -f -- "$temporary" "$destination"
printf '%s\n' "$actual"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", source, destination, checkpointString(download, "artifactSHA256")},
		Timeout: 2 * time.Minute, MaximumOutput: 32 * 1024,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOWriteFailed, "安装 Server Artifact 失败", err).WithDetails(map[string]any{"stderr": result.Stderr})
	}
	if err := reporter.SetProgress(1, "Server Artifact 已原子安装", 7); err != nil {
		return nil, err
	}
	return map[string]any{"jarPath": destination, "artifactSHA256": strings.TrimSpace(result.Stdout)}, nil
}

func (m *InstallationManager) writeEULA(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	profile := runtimeProfileForServerType(server.Type)
	if !profile.requiresEULA {
		if err := reporter.SetProgress(1, "当前服务端类型不需要 Minecraft EULA 文件", 8); err != nil {
			return nil, err
		}
		return map[string]any{"accepted": true, "skipped": true}, nil
	}
	eulaPath := path.Join(server.RemotePath, "eula.txt")
	script := `set -eu
temporary="${1}.part"
printf 'eula=true\n' > "$temporary"
mv -f -- "$temporary" "$1"`
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", script, "mineops", eulaPath}, Timeout: 10 * time.Second}); err != nil {
		return nil, apperror.Wrap(apperror.CodeInstallationEULAFailed, "写入 Minecraft EULA 失败", err)
	}
	server.EULAAccepted = true
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return nil, err
	}
	if err := reporter.SetProgress(1, "已幂等写入 eula=true", 8); err != nil {
		return nil, err
	}
	return map[string]any{"eulaPath": eulaPath, "accepted": true}, nil
}

func (m *InstallationManager) configureFirewall(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, err := m.store.MinecraftServers().Get(ctx, task.ServerID, false)
	if err != nil {
		return nil, err
	}
	settings := m.settings.Snapshot().Firewall
	serverPort := model.DefaultMinecraftPort(server.Type)
	if !settings.AutoOpenOnInstall || settings.Provider == "disabled" || server.FirewallPolicy == enums.FirewallDisabled {
		if err := reporter.SetProgress(1, "安装阶段防火墙自动放行已禁用", 9); err != nil {
			return nil, err
		}
		return map[string]any{"port": serverPort, "skipped": true}, nil
	}
	status, err := m.firewall.AcquirePort(ctx, server.ID, serverPort)
	if err != nil {
		return nil, err
	}
	if err := reporter.SetProgress(1, "首次启动前已协调防火墙端口", 9); err != nil {
		return nil, err
	}
	return map[string]any{
		"port": status.Port, "backend": status.Backend.String(), "active": status.Active,
		"alreadyAllowed": status.AlreadyAllowed,
	}, nil
}

func (m *InstallationManager) firstStart(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, _, client, err := m.connectServer(ctx, task.ServerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	javaCheckpoint, err := m.stepCheckpoint(ctx, task.ID, "install_java")
	if err != nil {
		return nil, err
	}
	javaExecutable := checkpointString(javaCheckpoint, "executablePath")
	if javaExecutable == "" {
		resolveCheckpoint, resolveErr := m.stepCheckpoint(ctx, task.ID, "resolve_java")
		if resolveErr != nil {
			return nil, resolveErr
		}
		javaExecutable = checkpointString(resolveCheckpoint, "executablePath")
	}
	launch := launchCommandForServer(*server, javaExecutable)
	profile := runtimeProfileForServerType(server.Type)
	encodedArguments := strings.Join(launch.arguments, "\n")
	readyPatterns := strings.Join(profile.readyPatterns, "\n")
	configurationPath := path.Join(server.RemotePath, profile.configurationFile)
	script := `set -eu
executable=$1
work=$2
arguments=$3
ready_patterns=$4
stop_command=$5
configuration=$6
output="$work/.mineops-first-start.log"
fifo="$work/.mineops-console"
rm -f -- "$fifo"
mkfifo "$fifo"
exec 3<> "$fifo"
pid=''
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    printf '%s\n' "$stop_command" >&3 2>/dev/null || true
    for attempt in $(seq 1 10); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
    if kill -0 "$pid" 2>/dev/null; then kill -TERM -- "-$pid" 2>/dev/null || true; fi
    for attempt in $(seq 1 5); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
    if kill -0 "$pid" 2>/dev/null; then kill -KILL -- "-$pid" 2>/dev/null || true; fi
    wait "$pid" 2>/dev/null || true
  fi
  exec 3>&- 3<&-
  rm -f -- "$fifo"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
set --
while IFS= read -r argument; do set -- "$@" "$argument"; done <<EOF
$arguments
EOF
setsid "$executable" "$@" <&3 > "$output" 2>&1 &
pid=$!
ready=0
for attempt in $(seq 1 180); do
  if ! kill -0 "$pid" 2>/dev/null; then wait "$pid" || true; tail -c 65536 "$output" >&2 || true; exit 31; fi
  while IFS= read -r pattern; do
    [ -n "$pattern" ] || continue
    if grep -Fq -- "$pattern" "$output"; then ready=1; break; fi
  done <<EOF
$ready_patterns
EOF
  [ "$ready" = "0" ] || break
  sleep 1
done
[ "$ready" = "1" ] || { tail -c 65536 "$output" >&2 || true; exit 33; }
printf '%s\n' "$stop_command" >&3
for attempt in $(seq 1 60); do
  if ! kill -0 "$pid" 2>/dev/null; then
    wait "$pid" || { tail -c 65536 "$output" >&2 || true; exit 34; }
    test -f "$configuration" && test ! -L "$configuration" || { printf 'missing-configuration:%s\n' "$configuration" >&2; exit 35; }
    printf 'ready-and-stopped configuration=%s\n' "$configuration"
    exit 0
  fi
  sleep 1
done
tail -c 65536 "$output" >&2 || true
exit 32`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops", launch.executable, server.RemotePath, encodedArguments, readyPatterns, profile.stopCommand, configurationPath},
		WorkingDirectory: server.RemotePath, Timeout: 5 * time.Minute, MaximumOutput: 512 * 1024,
		OnOutput: installationOutputReporter(reporter, 0.6, "首次启动"),
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInstallationFirstStartFailed, "Minecraft 首次启动未就绪或未正常停止", err).WithDetails(map[string]any{
			"stderr": result.Stderr, "stdout": result.Stdout,
		})
	}
	if err := reporter.SetProgress(1, "首次启动已就绪并正常停止", 10); err != nil {
		return nil, err
	}
	return map[string]any{"ready": true, "stopped": true, "evidence": strings.TrimSpace(result.Stdout)}, nil
}

func (m *InstallationManager) registerServer(ctx context.Context, task model.InstallationTask, _ model.InstallationStep, reporter InstallationStepReporter) (map[string]any, error) {
	server, err := m.store.MinecraftServers().Get(ctx, task.ServerID, false)
	if err != nil {
		return nil, err
	}
	installCheckpoint, err := m.stepCheckpoint(ctx, task.ID, "install_server")
	if err != nil {
		return nil, err
	}
	firstStartCheckpoint, err := m.stepCheckpoint(ctx, task.ID, "first_start")
	if err != nil {
		return nil, err
	}
	profile := runtimeProfileForServerType(server.Type)
	if checkpointString(installCheckpoint, "artifactSHA256") == "" || !checkpointBool(firstStartCheckpoint, "ready") || !checkpointBool(firstStartCheckpoint, "stopped") || (profile.requiresEULA && !server.EULAAccepted) || server.JavaRuntimeID == nil {
		return nil, apperror.New(apperror.CodeInstallationRegistrationFailed, "安装结果证据不完整，拒绝注册 Server")
	}
	if err := reporter.SetProgress(1, "Server 已注册并进入 Stopped 状态", 11); err != nil {
		return nil, err
	}
	return map[string]any{"serverID": server.ID.String(), "state": server.State.String()}, nil
}

func (m *InstallationManager) connectServer(ctx context.Context, serverID model.ID) (*model.MinecraftServer, *model.SSHSession, *SSHClient, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, nil, nil, err
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return nil, nil, nil, err
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return nil, nil, nil, err
	}
	return server, session, client, nil
}

func (m *InstallationManager) stepCheckpoint(ctx context.Context, taskID model.ID, stepName string) (map[string]any, error) {
	_, steps, err := m.store.Installations().Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step.Name == stepName && step.State == enums.InstallationStepSuccess {
			return step.Checkpoint, nil
		}
	}
	return nil, apperror.New(apperror.CodeValidationConflict, "Installation 前置步骤 checkpoint 不可用").WithDetails(map[string]any{"step": stepName})
}

func checkpointString(checkpoint map[string]any, key string) string {
	value, _ := checkpoint[key].(string)
	return value
}

func checkpointBool(checkpoint map[string]any, key string) bool {
	value, _ := checkpoint[key].(bool)
	return value
}

func checkpointInt(checkpoint map[string]any, key string) int {
	switch value := checkpoint[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case string:
		parsed, _ := strconv.Atoi(value)
		return parsed
	default:
		return 0
	}
}

func installationOutputReporter(reporter InstallationStepReporter, progress float64, prefix string) func(RemoteCommandOutput) {
	return func(output RemoteCommandOutput) {
		message := strings.TrimSpace(strings.ReplaceAll(output.Data, "\r", "\n"))
		if message == "" {
			return
		}
		if len(message) > 800 {
			message = message[len(message)-800:]
		}
		_ = reporter.SetProgress(progress, prefix+" ["+output.Stream+"]: "+message, 0)
	}
}

func downloadOutputReporter(reporter InstallationStepReporter, total int64, prefix string) func(RemoteCommandOutput) {
	lastAt := time.Now()
	var lastBytes int64
	return func(output RemoteCommandOutput) {
		for _, line := range strings.Split(strings.ReplaceAll(output.Data, "\r", "\n"), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "MINEOPS_PROGRESS" {
				downloaded, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					continue
				}
				now := time.Now()
				elapsed := now.Sub(lastAt).Seconds()
				speed := float64(0)
				if elapsed > 0 {
					speed = float64(downloaded-lastBytes) / elapsed
				}
				progress := float64(0.5)
				message := fmt.Sprintf("%s: %.1f MiB · %.1f MiB/s", prefix, float64(downloaded)/1024/1024, speed/1024/1024)
				if total > 0 {
					progress = min(float64(downloaded)/float64(total), 0.99)
					remaining := time.Duration(0)
					if speed > 0 && downloaded < total {
						remaining = time.Duration(float64(total-downloaded) / speed * float64(time.Second))
					}
					message = fmt.Sprintf("%s: %.1f%% · %.1f MiB/s · 剩余 %s", prefix, progress*100, speed/1024/1024, remaining.Round(time.Second))
				}
				_ = reporter.SetProgress(progress, message, 0)
				lastAt, lastBytes = now, downloaded
				continue
			}
			if strings.TrimSpace(line) != "" {
				installationOutputReporter(reporter, 0.5, prefix)(RemoteCommandOutput{Stream: output.Stream, Data: line})
			}
		}
	}
}
