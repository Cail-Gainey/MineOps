package service

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/appthread"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// JavaCandidate 承载持久化之前已校验的远端 Java 可执行文件。
type JavaCandidate struct {
	ExecutablePath string                  `json:"executablePath"`
	Source         model.JavaRuntimeSource `json:"source"`
	Info           model.JavaVersionInfo   `json:"info"`
}

// JavaRuntimeManager 负责远端 Java 安装的探测、校验、导入、推荐与删除。
type JavaRuntimeManager struct {
	clock    model.Clock
	store    repository.Store
	clients  *SSHClientFactory
	settings *appsettings.Manager
	catalog  port.JDKCatalog
	runner   *OperationRunner
	pool     *appthread.Pool
}

// NewJavaRuntimeManager 创建远端 Java 应用服务。
//
// pool 限制 Discover 发起的并发远端探测数;池为 nil 时回落到 appthread.Default()。
func NewJavaRuntimeManager(clock model.Clock, store repository.Store, clients *SSHClientFactory, settings *appsettings.Manager, catalog port.JDKCatalog, runner *OperationRunner, pool *appthread.Pool) (*JavaRuntimeManager, error) {
	if clock == nil || store == nil || clients == nil || settings == nil || catalog == nil || runner == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Java Runtime Manager 依赖不能为空")
	}
	if pool == nil {
		pool = appthread.Default()
	}
	return &JavaRuntimeManager{clock: clock, store: store, clients: clients, settings: settings, catalog: catalog, runner: runner, pool: pool}, nil
}

// Discover 按优先级依次校验受管目录、JAVA_HOME、PATH 与常见 Linux 路径下的 Java 候选。
func (m *JavaRuntimeManager) Discover(ctx context.Context, sshSessionID model.ID) ([]JavaCandidate, error) {
	_, client, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	return m.DiscoverWithClient(ctx, client)
}

// DiscoverWithClient 在已认证的客户端上执行探测,便于调用方复用同一条 SSH 连接。
//
// 远程探测通过线程池并发执行:环境变量合并成一次往返,系统目录 find 与候选校验按输入顺序扇出,
// 因此 Managed→JAVA_HOME→PATH→System 的优先级顺序与串行实现完全一致。
func (m *JavaRuntimeManager) DiscoverWithClient(ctx context.Context, client *SSHClient) ([]JavaCandidate, error) {
	if client == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Java 发现的 SSH Client 不能为空")
	}
	environment := m.discoverEnvironment(ctx, client)

	type candidatePath struct {
		value  string
		source model.JavaRuntimeSource
	}
	paths := make([]candidatePath, 0, 32)
	systemRoots := []string{"/usr/lib/jvm", "/usr/local/lib/jvm", "/opt/java", "/opt/jdk"}
	roots := make([]string, 0, len(systemRoots)+1)
	managedRoot := ""
	if environment.home != "" {
		managedRoot = path.Join(environment.home, "MineOps/Runtime")
		roots = append(roots, managedRoot)
	}
	roots = append(roots, systemRoots...)
	found := appthread.Map(ctx, m.pool, roots, func(taskCtx context.Context, root string) []string {
		return m.findJavaExecutables(taskCtx, client, root)
	})

	if managedRoot != "" {
		for _, value := range found[0] {
			paths = append(paths, candidatePath{value: value, source: model.JavaSourceManaged})
		}
		found = found[1:]
	}
	if environment.javaHome != "" {
		paths = append(paths, candidatePath{value: path.Join(environment.javaHome, "bin/java"), source: model.JavaSourceJavaHome})
	}
	if environment.pathJava != "" {
		paths = append(paths, candidatePath{value: environment.pathJava, source: model.JavaSourcePath})
	}
	for _, values := range found {
		for _, value := range values {
			paths = append(paths, candidatePath{value: value, source: model.JavaSourceSystem})
		}
	}

	seen := make(map[string]bool, len(paths))
	unique := make([]candidatePath, 0, len(paths))
	for _, candidate := range paths {
		candidate.value = path.Clean(strings.TrimSpace(candidate.value))
		if candidate.value == "." || seen[candidate.value] {
			continue
		}
		seen[candidate.value] = true
		unique = append(unique, candidate)
	}

	type verification struct {
		candidate JavaCandidate
		err       error
	}
	verified := appthread.Map(ctx, m.pool, unique, func(taskCtx context.Context, candidate candidatePath) verification {
		value, verifyErr := m.validateWithClient(taskCtx, client, candidate.value, candidate.source)
		return verification{candidate: value, err: verifyErr}
	})
	result := make([]JavaCandidate, 0, len(verified))
	for _, item := range verified {
		if item.err == nil {
			result = append(result, item.candidate)
		}
	}
	// appthread.Map 会吞掉任务错误。上下文已取消时结果必然不完整,
	// 必须把取消传出去,否则调用方会把"发现被打断"误判成"这台机器没有 Java"而去装一份新的 JDK。
	if ctx.Err() != nil {
		return nil, apperror.Wrap(apperror.CodeProcessCancelled, "Java 发现已取消", ctx.Err())
	}
	return result, nil
}

type javaEnvironment struct {
	home     string
	javaHome string
	pathJava string
}

// 固定输出三行(HOME、JAVA_HOME、PATH 中的 java),未设置的变量输出空行,便于按行位取值。
const javaEnvironmentProbeScript = `printf '%s\n' "${HOME:-}"
printf '%s\n' "${JAVA_HOME:-}"
printf '%s\n' "$(command -v java 2>/dev/null || true)"`

// discoverEnvironment 用一次往返读取 HOME、JAVA_HOME 与 PATH 中的 java,而不是三次。
func (m *JavaRuntimeManager) discoverEnvironment(ctx context.Context, client *SSHClient) javaEnvironment {
	output := m.commandOutput(ctx, client, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", javaEnvironmentProbeScript},
		Timeout: 10 * time.Second, MaximumOutput: 16 * 1024,
	})
	lines := strings.Split(output, "\n")
	value := func(index int) string {
		if index >= len(lines) {
			return ""
		}
		return strings.TrimSpace(lines[index])
	}
	return javaEnvironment{home: value(0), javaHome: value(1), pathJava: value(2)}
}

// List 按仓储过滤条件返回已持久化的 Java 运行时。
func (m *JavaRuntimeManager) List(ctx context.Context, query repository.JavaRuntimeQuery) ([]model.JavaRuntime, error) {
	return m.store.JavaRuntimes().List(ctx, query)
}

// ListArtifacts 返回已核准、与供应方无关的 JDK 下载构件。
func (m *JavaRuntimeManager) ListArtifacts(ctx context.Context, majorVersion int, architecture string) ([]port.JDKArtifact, error) {
	return m.catalog.List(ctx, majorVersion, architecture, "linux")
}

// Validate 通过 SSH 校验一个手工指定的 Java 可执行文件。
func (m *JavaRuntimeManager) Validate(ctx context.Context, sshSessionID model.ID, executablePath string, source model.JavaRuntimeSource) (JavaCandidate, error) {
	_, client, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return JavaCandidate{}, err
	}
	defer func() { _ = client.Close() }()
	return m.validateWithClient(ctx, client, executablePath, source)
}

// ValidateWithClient 在已认证的客户端上校验一个 Java 可执行文件。
func (m *JavaRuntimeManager) ValidateWithClient(ctx context.Context, client *SSHClient, executablePath string, source model.JavaRuntimeSource) (JavaCandidate, error) {
	if client == nil {
		return JavaCandidate{}, apperror.New(apperror.CodeValidationRequired, "Java 校验的 SSH Client 不能为空")
	}
	return m.validateWithClient(ctx, client, executablePath, source)
}

// Import 校验并在事务内登记一个远端 Java 安装,按路径去重。
func (m *JavaRuntimeManager) Import(ctx context.Context, sshSessionID model.ID, executablePath string, source model.JavaRuntimeSource) (*model.JavaRuntime, error) {
	candidate, err := m.Validate(ctx, sshSessionID, executablePath, source)
	if err != nil {
		return nil, err
	}
	return m.registerCandidate(ctx, sshSessionID, candidate)
}

// ImportCandidate 登记一个已校验的候选,不再新建 SSH 连接。
//
// Discover 返回的候选已经完成远程校验,重复 Import 会为每个候选再连一次并重跑 java -version;
// 该入口让调用方直接复用校验结果,只做数据库写入。
func (m *JavaRuntimeManager) ImportCandidate(ctx context.Context, sshSessionID model.ID, candidate JavaCandidate) (*model.JavaRuntime, error) {
	if candidate.ExecutablePath == "" {
		return nil, apperror.New(apperror.CodeValidationRequired, "Java 候选不能为空")
	}
	return m.registerCandidate(ctx, sshSessionID, candidate)
}

func (m *JavaRuntimeManager) registerCandidate(ctx context.Context, sshSessionID model.ID, candidate JavaCandidate) (*model.JavaRuntime, error) {
	installPath := candidate.Info.JavaHome
	if installPath == "" {
		installPath = path.Dir(path.Dir(candidate.ExecutablePath))
	}
	if existing, findErr := m.store.JavaRuntimes().GetByPath(ctx, sshSessionID, installPath); findErr == nil {
		existing.Version = candidate.Info.Version
		existing.MajorVersion = candidate.Info.MajorVersion
		existing.Vendor = candidate.Info.Vendor
		existing.Architecture = candidate.Info.Architecture
		existing.Source = candidate.Source
		existing.JavaHome = installPath
		existing.Managed = candidate.Source == model.JavaSourceManaged
		existing.Reusable = true
		existing.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.JavaRuntimes().Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	} else if apperror.ToDTO(findErr).Code != apperror.CodeIONotFound.String() {
		return nil, findErr
	}
	javaRuntime, err := model.NewJavaRuntime(m.clock, model.JavaRuntime{
		SSHSessionID: sshSessionID, Version: candidate.Info.Version, MajorVersion: candidate.Info.MajorVersion,
		Vendor: candidate.Info.Vendor, Architecture: candidate.Info.Architecture, Source: candidate.Source,
		InstallPath: installPath, JavaHome: installPath, Managed: candidate.Source == model.JavaSourceManaged,
		Reusable: true,
	})
	if err != nil {
		return nil, err
	}
	if err := m.store.JavaRuntimes().Create(ctx, javaRuntime); err != nil {
		return nil, err
	}
	return javaRuntime, nil
}

// Recommend 为某个 Minecraft 版本选出默认的或最接近的可复用兼容 Java 运行时。
func (m *JavaRuntimeManager) Recommend(ctx context.Context, sshSessionID model.ID, serverType, minecraftVersion string) (*model.JavaRuntime, error) {
	requirement, err := model.JavaRequirementForMinecraft(serverType, minecraftVersion)
	if err != nil {
		return nil, err
	}
	runtimes, err := m.store.JavaRuntimes().List(ctx, repository.JavaRuntimeQuery{SSHSessionID: sshSessionID, Limit: 500})
	if err != nil {
		return nil, err
	}
	compatible := make([]model.JavaRuntime, 0, len(runtimes))
	for _, javaRuntime := range runtimes {
		if javaRuntime.Reusable && requirement.Compatible(javaRuntime.MajorVersion) {
			compatible = append(compatible, javaRuntime)
		}
	}
	if len(compatible) == 0 {
		return nil, apperror.New(apperror.CodeIONotFound, "没有兼容的 Java Runtime").WithDetails(map[string]any{
			"minimumMajor": requirement.MinimumMajor, "maximumMajor": requirement.MaximumMajor,
		})
	}
	sort.SliceStable(compatible, func(left, right int) bool {
		if compatible[left].Default != compatible[right].Default {
			return compatible[left].Default
		}
		leftDistance := absolute(compatible[left].MajorVersion - requirement.PreferredMajor)
		rightDistance := absolute(compatible[right].MajorVersion - requirement.PreferredMajor)
		return leftDistance < rightDistance
	})
	selected := compatible[0]
	return &selected, nil
}

// SetDefault 把一条 Java 运行时标记为其 SSH Session 的默认项。
func (m *JavaRuntimeManager) SetDefault(ctx context.Context, sshSessionID, id model.ID) error {
	return m.store.JavaRuntimes().SetDefault(ctx, sshSessionID, id)
}

// Delete 删除一条无引用的 Java 运行时登记,不删除其远端文件。
func (m *JavaRuntimeManager) Delete(ctx context.Context, id model.ID) error {
	references, err := m.store.JavaRuntimes().CountServerReferences(ctx, id)
	if err != nil {
		return err
	}
	if references > 0 {
		return apperror.New(apperror.CodeValidationConflict, "Java Runtime 已被 Minecraft Server 引用").WithDetails(map[string]any{"serverCount": references})
	}
	return m.store.JavaRuntimes().Delete(ctx, id)
}

func (m *JavaRuntimeManager) connect(ctx context.Context, sshSessionID model.ID) (*model.SSHSession, *SSHClient, error) {
	sshSession, err := m.store.SSHSessions().Get(ctx, sshSessionID)
	if err != nil {
		return nil, nil, err
	}
	client, err := m.clients.Connect(ctx, sshSession, m.settings.Snapshot().SSH)
	if err != nil {
		return nil, nil, err
	}
	return sshSession, client, nil
}

func (m *JavaRuntimeManager) validateWithClient(ctx context.Context, client *SSHClient, executablePath string, source model.JavaRuntimeSource) (JavaCandidate, error) {
	executablePath = path.Clean(strings.TrimSpace(executablePath))
	if !path.IsAbs(executablePath) {
		return JavaCandidate{}, apperror.New(apperror.CodeValidationInvalidArgument, "Java 可执行文件必须是远程绝对路径")
	}
	resolveScript := `set -eu
candidate=$1
if [ -d "$candidate" ]; then candidate=$candidate/bin/java; fi
if [ ! -f "$candidate" ] || [ ! -x "$candidate" ]; then
  printf 'Java executable is missing or not executable: %s\n' "$candidate" >&2
  exit 2
fi
if command -v readlink >/dev/null 2>&1; then
  resolved=$(readlink -f "$candidate" 2>/dev/null || true)
  if [ -n "$resolved" ]; then candidate=$resolved; fi
fi
printf '%s' "$candidate"`
	resolved, resolveErr := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", resolveScript, "mineops-java-validate", executablePath},
		Timeout: 10 * time.Second, MaximumOutput: 16 * 1024,
	})
	if resolveErr != nil || strings.TrimSpace(resolved.Stdout) == "" {
		return JavaCandidate{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "Java 可执行文件不存在或不可执行", resolveErr).WithDetails(map[string]any{
			"requestedPath": executablePath,
			"exitCode":      resolved.ExitCode,
			"stderr":        strings.TrimSpace(resolved.Stderr),
		})
	}
	executablePath = path.Clean(strings.TrimSpace(resolved.Stdout))
	probes := [][]string{{"-XshowSettings:properties", "-version"}, {"-version"}, {"--version"}}
	combined := strings.Builder{}
	var lastResult RemoteCommandResult
	var lastError error
	var parseErr error
	for _, arguments := range probes {
		lastResult, lastError = client.RunCommand(ctx, RemoteCommand{
			Executable: executablePath, Arguments: arguments,
			Timeout: 15 * time.Second, MaximumOutput: 256 * 1024,
		})
		output := strings.TrimSpace(lastResult.Stdout + "\n" + lastResult.Stderr)
		if output != "" {
			if combined.Len() > 0 {
				combined.WriteByte('\n')
			}
			combined.WriteString(output)
		}
		var info model.JavaVersionInfo
		info, parseErr = model.ParseJavaVersionOutput(combined.String())
		if parseErr == nil {
			return JavaCandidate{ExecutablePath: executablePath, Source: source, Info: info}, nil
		}
	}
	return JavaCandidate{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "无法读取 Java 版本信息", parseErr).WithDetails(map[string]any{
		"executablePath": executablePath,
		"exitCode":       lastResult.ExitCode,
		"stderr":         strings.TrimSpace(lastResult.Stderr),
		"commandFailed":  lastError != nil,
	})
}

func (m *JavaRuntimeManager) findJavaExecutables(ctx context.Context, client *SSHClient, root string) []string {
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "find", Arguments: []string{root, "-maxdepth", "5", "-type", "f", "-name", "java", "-perm", "-u+x"},
		Timeout: 10 * time.Second, MaximumOutput: 64 * 1024,
	})
	if err != nil && strings.TrimSpace(result.Stdout) == "" {
		return nil
	}
	return strings.Fields(result.Stdout)
}

func (m *JavaRuntimeManager) commandOutput(ctx context.Context, client *SSHClient, command RemoteCommand) string {
	result, err := client.RunCommand(ctx, command)
	if err != nil {
		var applicationError *apperror.Error
		if !errors.As(err, &applicationError) {
			return ""
		}
	}
	return result.Stdout
}

func absolute(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
