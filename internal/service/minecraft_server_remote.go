package service

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const minecraftVersionPattern = `[0-9]+(?:\.[0-9]+){1,2}`

var minecraftVersionClue = regexp.MustCompile(`(?:^|[^0-9])(` + minecraftVersionPattern + `)(?:[^0-9]|$)`)
var forgeLibraryVersionEvidence = regexp.MustCompile(`(?i)net/minecraftforge/forge/(` + minecraftVersionPattern + `)-[^/\s]+/(?:unix|win)_args\.txt`)
var neoForgeBuildVersionEvidence = regexp.MustCompile(`(?im)(?:net/neoforged/neoforge/|(?:^|[/_-])neoforge[-_])([0-9]+)\.([0-9]+)(?:\.[^/\s]+)?`)
var minecraftVersionEvidence = []*regexp.Regexp{
	regexp.MustCompile(`(?i)"(?:id|name)"\s*:\s*"(` + minecraftVersionPattern + `)"`),
	regexp.MustCompile(`(?i)"minecraft"\s*:\s*"(` + minecraftVersionPattern + `)"`),
	regexp.MustCompile(`(?im)^[0-9a-f]{32,128}[ \t]+(` + minecraftVersionPattern + `)[ \t]+[^\r\n]*server-`),
	regexp.MustCompile(`(?i)\b(?:minecraft|mc|game)[ ._-]*version\b"?\s*(?:[:=]|,\s*)\s*"?v?(` + minecraftVersionPattern + `)`),
	regexp.MustCompile(`(?i)starting\s+minecraft\s+server\s+version\s+v?(` + minecraftVersionPattern + `)`),
	regexp.MustCompile(`(?i)loading\s+minecraft\s+v?(` + minecraftVersionPattern + `)`),
	regexp.MustCompile(`(?i)implementing\s+api\s+version\s+v?(` + minecraftVersionPattern + `)`),
	regexp.MustCompile(`(?i)\bfor\s+(?:minecraft|mc)\s+v?(` + minecraftVersionPattern + `)`),
	regexp.MustCompile(`(?i)\(\s*mc\s*:\s*v?(` + minecraftVersionPattern + `)\s*\)`),
	regexp.MustCompile(`(?i)\b(?:paper|purpur|folia|spigot)\s+(?:server\s+)?version\s+v?(` + minecraftVersionPattern + `)`),
}

const remoteServerInventoryScript = `set -eu
for file in "$1"/* "$1"/.[!.]* "$1"/..?*; do
  if [ ! -e "$file" ] || [ -L "$file" ] || [ ! -f "$file" ]; then continue; fi
  name=${file##*/}
  case "$name" in
    run.sh|server.properties|eula.txt)
      size=$(wc -c < "$file" | tr -d '[:space:]')
      printf '%s\000%s\000' "$name" "$size"
      ;;
    *)
      lower_name=$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')
      case "$lower_name" in
        *.jar)
          size=$(wc -c < "$file" | tr -d '[:space:]')
          printf '%s\000%s\000' "$name" "$size"
          ;;
      esac
      ;;
  esac
done`

const remoteServerMetadataScript = `set -u
root=$1
jar=$2
if [ -f "$root/version.json" ] && [ ! -L "$root/version.json" ]; then
  printf '%s\n' '--- mineops:root/version.json ---'
  head -c 32768 "$root/version.json" 2>/dev/null || true
  printf '\n'
fi
if [ -n "$jar" ] && [ -f "$root/$jar" ] && [ ! -L "$root/$jar" ]; then
  python_command=
  if command -v python3 >/dev/null 2>&1; then
    python_command=python3
  elif command -v python >/dev/null 2>&1; then
    python_command=python
  fi
  for entry in version.json META-INF/MANIFEST.MF META-INF/versions.list fabric-server-launch.properties quilt-server-launch.properties install_profile.json; do
    printf '%s\n' '--- mineops:jar-entry ---'
    if command -v unzip >/dev/null 2>&1; then
      unzip -p "$root/$jar" "$entry" 2>/dev/null | head -c 32768 || true
    elif command -v busybox >/dev/null 2>&1 && busybox unzip -h >/dev/null 2>&1; then
      busybox unzip -p "$root/$jar" "$entry" 2>/dev/null | head -c 32768 || true
    elif command -v bsdtar >/dev/null 2>&1; then
      bsdtar -xOf "$root/$jar" "$entry" 2>/dev/null | head -c 32768 || true
    elif [ -n "$python_command" ]; then
      "$python_command" -c 'import sys, zipfile
try:
    with zipfile.ZipFile(sys.argv[1]) as archive:
        sys.stdout.buffer.write(archive.read(sys.argv[2])[:32768])
except (KeyError, OSError, zipfile.BadZipFile):
    pass' "$root/$jar" "$entry" 2>/dev/null || true
    fi
    printf '\n'
  done
fi
if [ -f "$root/run.sh" ] && [ ! -L "$root/run.sh" ]; then
  printf '%s\n' '--- mineops:file/run.sh ---'
  head -c 32768 "$root/run.sh" 2>/dev/null || true
  printf '\n'
fi
for entry in \
  "$root"/libraries/net/minecraftforge/forge/*/unix_args.txt \
  "$root"/libraries/net/minecraftforge/forge/*/win_args.txt \
  "$root"/libraries/net/neoforged/neoforge/*/unix_args.txt \
  "$root"/libraries/net/neoforged/neoforge/*/win_args.txt; do
  if [ ! -f "$entry" ] || [ -L "$entry" ]; then continue; fi
  relative=${entry#"$root"/}
  printf '%s\n' "--- mineops:file/$relative ---"
  head -c 32768 "$entry" 2>/dev/null || true
  printf '\n'
done
if [ -f "$root/logs/latest.log" ] && [ ! -L "$root/logs/latest.log" ]; then
  printf '%s\n' '--- mineops:logs/latest.log:head ---'
  head -c 131072 "$root/logs/latest.log" 2>/dev/null || true
  printf '\n%s\n' '--- mineops:logs/latest.log:tail ---'
  tail -c 131072 "$root/logs/latest.log" 2>/dev/null || true
  printf '\n'
fi`

// RemoteServerJar is one ordinary root-level Jar candidate discovered without changing the remote directory.
type RemoteServerJar struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// RemoteServerInspection contains read-only evidence used before importing an existing remote Server.
type RemoteServerInspection struct {
	RemotePath       string                    `json:"remotePath"`
	Jars             []RemoteServerJar         `json:"jars"`
	SuggestedJar     string                    `json:"suggestedJar"`
	SuggestedType    enums.MinecraftServerType `json:"suggestedType"`
	SuggestedVersion string                    `json:"suggestedVersion"`
	PropertiesFound  bool                      `json:"propertiesFound"`
	EULAFound        bool                      `json:"eulaFound"`
	EULAAccepted     bool                      `json:"eulaAccepted"`
	Warnings         []string                  `json:"warnings"`
}

// InspectRemote performs read-only launch file, Properties, EULA, and version-clue inspection.
func (m *MinecraftServerManager) InspectRemote(ctx context.Context, sshSessionID model.ID, requestedPath string) (RemoteServerInspection, error) {
	client, home, err := m.connectRemoteServer(ctx, sshSessionID)
	if err != nil {
		return RemoteServerInspection{}, err
	}
	defer func() { _ = client.Close() }()
	requestedRemotePath, err := model.NormalizeRemotePath(requestedPath, home, home)
	if err != nil {
		return RemoteServerInspection{}, err
	}
	remotePath, err := resolveRemoteServerImportPath(ctx, client, requestedRemotePath)
	if err != nil {
		return RemoteServerInspection{}, err
	}
	remotePath, err = model.NormalizeRemotePath(remotePath, home, home)
	if err != nil {
		return RemoteServerInspection{}, err
	}
	if remotePath == "/" || remotePath == home || path.Dir(remotePath) == "/" {
		return RemoteServerInspection{}, apperror.New(apperror.CodeSFTPPathRejected, "远程 Server 目录不能是根目录、Home 或根目录下一级路径")
	}
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", remoteServerInventoryScript, "mineops-server-import", remotePath},
		Timeout: 20 * time.Second, MaximumOutput: 512 * 1024,
	})
	if err != nil {
		return RemoteServerInspection{}, apperror.Wrap(apperror.CodeIOReadFailed, "检查远程 Server 文件失败", err).WithDetails(map[string]any{
			"remotePath": remotePath,
			"exitCode":   result.ExitCode,
			"stderr":     strings.TrimSpace(result.Stderr),
		})
	}
	fields := strings.Split(result.Stdout, "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%2 != 0 {
		return RemoteServerInspection{}, apperror.New(apperror.CodeIOReadFailed, "远程 Server 文件响应格式无效")
	}
	inspection := RemoteServerInspection{
		RemotePath: remotePath, SuggestedType: enums.ServerVanilla,
		Jars: make([]RemoteServerJar, 0), Warnings: make([]string, 0),
	}
	if requestedRemotePath != remotePath {
		inspection.Warnings = append(inspection.Warnings, "远程目录符号链接已解析为实际路径，导入后将固定使用该目录")
	}
	var runScript *RemoteServerJar
	for index := 0; index < len(fields); index += 2 {
		name := fields[index]
		size, parseErr := strconv.ParseInt(fields[index+1], 10, 64)
		if parseErr != nil || size < 0 {
			return RemoteServerInspection{}, apperror.New(apperror.CodeIOReadFailed, "远程 Server 文件大小无效")
		}
		switch name {
		case "server.properties":
			inspection.PropertiesFound = true
		case "eula.txt":
			inspection.EULAFound = true
		case "run.sh":
			candidate := RemoteServerJar{Name: name, Size: size}
			runScript = &candidate
		default:
			if strings.HasSuffix(strings.ToLower(name), ".jar") {
				inspection.Jars = append(inspection.Jars, RemoteServerJar{Name: name, Size: size})
			}
		}
	}
	sort.Slice(inspection.Jars, func(left, right int) bool { return inspection.Jars[left].Name < inspection.Jars[right].Name })
	if inspection.EULAFound {
		eula, readErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "cat", Arguments: []string{"--", path.Join(remotePath, "eula.txt")}, Timeout: 5 * time.Second, MaximumOutput: 16 * 1024,
		})
		if readErr != nil {
			return RemoteServerInspection{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取远程 EULA 状态失败", readErr)
		}
		for _, line := range strings.Split(eula.Stdout, "\n") {
			key, value, found := strings.Cut(strings.TrimSpace(line), "=")
			if found && strings.EqualFold(strings.TrimSpace(key), "eula") {
				inspection.EULAAccepted = strings.EqualFold(strings.TrimSpace(value), "true")
			}
		}
	}
	inspection.SuggestedJar, inspection.SuggestedType, inspection.SuggestedVersion = inferRemoteServerClues(inspection.Jars, "")
	metadataEvidence := ""
	if runScript != nil || inspection.SuggestedVersion == "" || inspection.SuggestedType == enums.ServerVanilla {
		metadata, metadataErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh", Arguments: []string{"-c", remoteServerMetadataScript, "mineops-server-import-metadata", remotePath, inspection.SuggestedJar},
			Timeout: 15 * time.Second, MaximumOutput: 512 * 1024,
		})
		if metadataErr == nil {
			metadataEvidence = metadata.Stdout
		}
	}
	launchCandidates := append([]RemoteServerJar(nil), inspection.Jars...)
	if runScript != nil {
		launchCandidates = append(launchCandidates, *runScript)
	}
	suggestedLauncher, inferredType, inferredVersion := inferRemoteServerClues(launchCandidates, metadataEvidence)
	if inferredType == enums.ServerForge || inferredType == enums.ServerNeoForge {
		inspection.Jars = launchCandidates
		inspection.SuggestedJar = suggestedLauncher
		inspection.SuggestedType = inferredType
		if inferredVersion != "" {
			inspection.SuggestedVersion = inferredVersion
		}
	} else {
		inspection.SuggestedJar, inspection.SuggestedType, inspection.SuggestedVersion = inferRemoteServerClues(inspection.Jars, metadataEvidence)
	}
	sort.Slice(inspection.Jars, func(left, right int) bool { return inspection.Jars[left].Name < inspection.Jars[right].Name })
	if inspection.SuggestedVersion == "" {
		if match := minecraftVersionClue.FindStringSubmatch(strings.ToLower(path.Base(remotePath))); len(match) > 1 {
			inspection.SuggestedVersion = match[1]
		}
	}
	if len(inspection.Jars) == 0 {
		inspection.Warnings = append(inspection.Warnings, "根目录未发现可导入的启动文件")
	}
	if len(inspection.Jars) > 1 {
		inspection.Warnings = append(inspection.Warnings, "发现多个启动文件，导入前必须明确选择")
	}
	if !inspection.PropertiesFound {
		inspection.Warnings = append(inspection.Warnings, "未发现 server.properties，可能尚未完成首次启动")
	}
	if !inspection.EULAFound || !inspection.EULAAccepted {
		inspection.Warnings = append(inspection.Warnings, "EULA 未确认；MineOps 不会在导入时修改 eula.txt")
	}
	if inspection.SuggestedVersion == "" {
		inspection.Warnings = append(inspection.Warnings, "无法从启动文件、版本元数据或最新日志识别 Minecraft 版本，需要手工确认")
	}
	return inspection, nil
}

func resolveRemoteServerImportPath(ctx context.Context, client *SSHClient, remotePath string) (string, error) {
	resolveScript := `set -eu
candidate=$1
resolved=
if command -v readlink >/dev/null 2>&1; then
  resolved=$(readlink -f "$candidate" 2>/dev/null || true)
fi
if [ -z "$resolved" ] && command -v realpath >/dev/null 2>&1; then
  resolved=$(realpath "$candidate" 2>/dev/null || true)
fi
if [ -z "$resolved" ]; then
  if [ -L "$candidate" ]; then
    printf 'remote directory symlink cannot be resolved: %s\n' "$candidate" >&2
    exit 2
  fi
  resolved=$candidate
fi
if [ ! -d "$resolved" ] || [ -L "$resolved" ]; then
  printf 'remote directory is missing or not an ordinary directory: %s\n' "$candidate" >&2
  exit 3
fi
printf '%s' "$resolved"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", resolveScript, "mineops-server-import-resolve", remotePath},
		Timeout: 10 * time.Second, MaximumOutput: 4096,
	})
	resolvedPath := strings.TrimSpace(result.Stdout)
	if err != nil || resolvedPath == "" {
		return "", apperror.Wrap(apperror.CodeSFTPPathRejected, "远程 Server 路径不存在或无法解析为普通目录", err).WithDetails(map[string]any{
			"requestedPath": remotePath,
			"exitCode":      result.ExitCode,
			"stderr":        strings.TrimSpace(result.Stderr),
		})
	}
	return resolvedPath, nil
}

// ImportRemote registers read-only inspected remote files without modifying the original directory.
func (m *MinecraftServerManager) ImportRemote(ctx context.Context, command MinecraftServerCommand) (*model.MinecraftServer, error) {
	if err := m.validateReferences(ctx, command.SSHSessionID, command.JavaRuntimeID); err != nil {
		return nil, err
	}
	inspection, err := m.InspectRemote(ctx, command.SSHSessionID, command.RemotePath)
	if err != nil {
		return nil, err
	}
	selectedLauncher := path.Clean(strings.TrimSpace(command.LaunchProfile.JarPath))
	if selectedLauncher == "." || selectedLauncher == "" || path.IsAbs(selectedLauncher) || strings.Contains(selectedLauncher, "/") {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "导入启动文件必须是远程 Server 根目录下的文件名")
	}
	launcherFound := false
	for _, candidate := range inspection.Jars {
		if candidate.Name == selectedLauncher {
			launcherFound = true
			break
		}
	}
	if !launcherFound {
		return nil, apperror.New(apperror.CodeValidationConflict, "选择的启动文件与最新远程检查结果不一致")
	}
	command.RemotePath = inspection.RemotePath
	command.LaunchProfile.WorkingDirectory = inspection.RemotePath
	command.LaunchProfile.JarPath = selectedLauncher
	server, err := model.NewMinecraftServer(m.clock, model.MinecraftServer{
		SSHSessionID: command.SSHSessionID, JavaRuntimeID: command.JavaRuntimeID,
		Name: command.Name, Type: command.Type, Version: command.Version,
		RemotePath: inspection.RemotePath, Group: command.Group, Tags: normalizeTags(command.Tags),
		Favourite: command.Favourite, State: enums.LifecycleReady,
		LaunchProfile: command.LaunchProfile, FirewallPolicy: command.FirewallPolicy,
		EULAAccepted: inspection.EULAAccepted,
	})
	if err != nil {
		return nil, err
	}
	if inspection.EULAAccepted {
		server.State = enums.LifecycleStopped
	}
	if err := m.store.MinecraftServers().Create(ctx, server); err != nil {
		return nil, err
	}
	return server, nil
}

// StartHardDeleteRemote starts destructive removal only after exact soft-deleted Server confirmation.
func (m *MinecraftServerManager) StartHardDeleteRemote(ctx context.Context, id model.ID, confirmedName, confirmedPath string) (model.ID, error) {
	server, err := m.store.MinecraftServers().Get(ctx, id, true)
	if err != nil {
		return "", err
	}
	if server.DeletedAt == nil || confirmedName != server.Name || path.Clean(confirmedPath) != server.RemotePath {
		return "", apperror.New(apperror.CodeValidationConflict, "远程硬删除确认对象与实际 Server 不一致")
	}
	return m.runner.Start(ctx, OperationRequest{
		Type: enums.OperationDelete, TargetType: enums.OperationTargetServer, TargetID: server.ID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.hardDeleteRemote(operationCtx, reporter, server)
		},
	})
}

func (m *MinecraftServerManager) hardDeleteRemote(ctx context.Context, reporter OperationReporter, server *model.MinecraftServer) error {
	if err := reporter.SetProgress("verify", 0.05, "正在重新核验远程删除边界"); err != nil {
		return err
	}
	client, home, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	remotePath, err := model.NormalizeRemotePath(server.RemotePath, home, home)
	if err != nil {
		return err
	}
	if remotePath != server.RemotePath || remotePath == "/" || remotePath == home || path.Dir(remotePath) == "/" || strings.HasPrefix(home, strings.TrimSuffix(remotePath, "/")+"/") {
		return apperror.New(apperror.CodeSFTPPathRejected, "拒绝删除根目录、Home、Home 上级或根目录下一级路径")
	}
	verifyScript := `set -eu
if [ -L "$1" ]; then exit 2; fi
if [ ! -e "$1" ]; then printf 'missing'; exit 0; fi
test -d "$1"
child_device=$(stat --format=%d -- "$1")
parent_device=$(stat --format=%d -- "$(dirname -- "$1")")
test "$child_device" = "$parent_device"
printf 'present'`
	verification, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", verifyScript, "mineops-server-delete", remotePath}, Timeout: 15 * time.Second, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "远程 Server 路径不存在、是符号链接或位于独立挂载根", err)
	}
	if strings.TrimSpace(verification.Stdout) == "present" {
		if err := reporter.SetProgress("delete", 0.35, fmt.Sprintf("正在永久删除远程目录 %s", remotePath)); err != nil {
			return err
		}
		if _, err := client.RunCommand(ctx, RemoteCommand{
			Executable: "rm", Arguments: []string{"-rf", "--", remotePath}, MaximumOutput: 4096,
		}); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "永久删除远程 Server 目录失败", err)
		}
	} else if strings.TrimSpace(verification.Stdout) != "missing" {
		return apperror.New(apperror.CodeIOReadFailed, "远程 Server 删除边界响应无效")
	}
	if err := reporter.SetProgress("metadata", 0.9, "正在删除加密数据库中的 Server 注册"); err != nil {
		return err
	}
	if err := m.store.MinecraftServers().HardDelete(context.WithoutCancel(ctx), server.ID); err != nil {
		return err
	}
	return reporter.SetProgress("complete", 0.98, "远程目录和 Server 注册已永久删除")
}

func (m *MinecraftServerManager) connectRemoteServer(ctx context.Context, sshSessionID model.ID) (*SSHClient, string, error) {
	if !sshSessionID.Valid() {
		return nil, "", apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
	}
	session, err := m.store.SSHSessions().Get(ctx, sshSessionID)
	if err != nil {
		return nil, "", err
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return nil, "", err
	}
	homeResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "printenv", Arguments: []string{"HOME"}, Timeout: 5 * time.Second, MaximumOutput: 4096})
	if err != nil || strings.TrimSpace(homeResult.Stdout) == "" {
		_ = client.Close()
		return nil, "", apperror.Wrap(apperror.CodeSFTPPathRejected, "读取远程 Home 目录失败", err)
	}
	home, err := model.NormalizeRemotePath(strings.TrimSpace(homeResult.Stdout), "/", "/")
	if err != nil {
		_ = client.Close()
		return nil, "", err
	}
	return client, home, nil
}

func inferRemoteServerClues(jars []RemoteServerJar, evidence string) (string, enums.MinecraftServerType, string) {
	clues := []struct {
		markers []string
		kind    enums.MinecraftServerType
	}{
		{[]string{"neoforge", "neoforged", "neoforge_server"}, enums.ServerNeoForge},
		{[]string{"waterfall"}, enums.ServerWaterfall},
		{[]string{"purpur"}, enums.ServerPurpur},
		{[]string{"folia"}, enums.ServerFolia},
		{[]string{"quilt", "quiltmc"}, enums.ServerQuilt},
		{[]string{"fabric", "fabricmc"}, enums.ServerFabric},
		{[]string{"paper", "papermc", "paperclip"}, enums.ServerPaper},
		{[]string{"velocity", "velocitypowered"}, enums.ServerVelocity},
		{[]string{"spigot", "craftbukkit", "org.bukkit"}, enums.ServerSpigot},
		{[]string{"bungeecord", "net.md_5.bungee", "bungee"}, enums.ServerBungee},
		{[]string{"minecraftforge", "net.minecraftforge", "forge_server", "forge-", "forge_", "/forge/"}, enums.ServerForge},
	}
	allJarNames := strings.Builder{}
	for _, jar := range jars {
		allJarNames.WriteString(jar.Name)
		allJarNames.WriteByte('\n')
	}
	lower := strings.ToLower(allJarNames.String() + evidence)
	typeValue := enums.ServerVanilla
	for _, clue := range clues {
		for _, marker := range clue.markers {
			if strings.Contains(lower, marker) {
				typeValue = clue.kind
				break
			}
		}
		if typeValue != enums.ServerVanilla {
			break
		}
	}
	selected := ""
	if typeValue == enums.ServerForge || typeValue == enums.ServerNeoForge {
		for _, jar := range jars {
			if strings.EqualFold(jar.Name, "run.sh") {
				selected = jar.Name
				break
			}
		}
	}
	if selected == "" && typeValue != enums.ServerVanilla {
		for _, jar := range jars {
			lowerName := strings.ToLower(jar.Name)
			for _, clue := range clues {
				if clue.kind != typeValue {
					continue
				}
				for _, marker := range clue.markers {
					if strings.Contains(lowerName, marker) {
						selected = jar.Name
						break
					}
				}
				if selected != "" {
					break
				}
			}
			if selected != "" {
				break
			}
		}
	}
	if selected == "" {
		for _, jar := range jars {
			if strings.EqualFold(jar.Name, "server.jar") {
				selected = jar.Name
				break
			}
		}
	}
	if selected == "" {
		for _, jar := range jars {
			if strings.HasSuffix(strings.ToLower(jar.Name), ".jar") {
				selected = jar.Name
				break
			}
		}
	}
	version := ""
	if typeValue == enums.ServerNeoForge {
		if match := neoForgeBuildVersionEvidence.FindStringSubmatch(evidence + "\n" + allJarNames.String()); len(match) > 2 {
			minor, minorErr := strconv.Atoi(match[1])
			patch, patchErr := strconv.Atoi(match[2])
			if minorErr == nil && patchErr == nil && minor > 1 {
				if patch == 0 {
					version = fmt.Sprintf("1.%d", minor)
				} else {
					version = fmt.Sprintf("1.%d.%d", minor, patch)
				}
			}
		}
	}
	if version == "" && (typeValue == enums.ServerForge || typeValue == enums.ServerNeoForge) {
		for _, jar := range jars {
			lowerName := strings.ToLower(jar.Name)
			if typeValue == enums.ServerForge && !strings.Contains(lowerName, "forge-") && !strings.Contains(lowerName, "forge_") {
				continue
			}
			if typeValue == enums.ServerNeoForge && !strings.Contains(lowerName, "neoforge-") && !strings.Contains(lowerName, "neoforge_") {
				continue
			}
			if match := minecraftVersionClue.FindStringSubmatch(lowerName); len(match) > 1 {
				version = match[1]
				break
			}
		}
	}
	if selected != "" {
		if match := minecraftVersionClue.FindStringSubmatch(strings.ToLower(selected)); version == "" && len(match) > 1 {
			version = match[1]
		}
	}
	if version == "" {
		for _, pattern := range minecraftVersionEvidence {
			if match := pattern.FindStringSubmatch(evidence); len(match) > 1 {
				version = match[1]
				break
			}
		}
	}
	if version == "" {
		if match := forgeLibraryVersionEvidence.FindStringSubmatch(evidence); len(match) > 1 {
			version = match[1]
		}
	}
	return selected, typeValue, version
}
