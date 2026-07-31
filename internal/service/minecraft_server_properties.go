package service

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const maximumServerPropertiesBackups = 10

// ServerPropertyValue 是某个 server.properties 键的最终生效值。
type ServerPropertyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ServerPropertyUpdate 修改一个键,同时完整保留所有无关的物理行。
type ServerPropertyUpdate struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ServerPropertiesSnapshot 承载原文、生效值与校验信息。
type ServerPropertiesSnapshot struct {
	Document         *model.RemoteTextDocument `json:"document"`
	Values           []ServerPropertyValue     `json:"values"`
	ServerPort       uint16                    `json:"serverPort"`
	ValidationNotice string                    `json:"validationNotice,omitempty"`
}

// ServerPropertiesBackup 是一份保存前保留的 server.properties 历史版本。
type ServerPropertiesBackup struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

// ReadProperties 读取并解析某台 Server 有界的普通 server.properties 文件。
func (m *MinecraftServerManager) ReadProperties(ctx context.Context, serverID model.ID) (*ServerPropertiesSnapshot, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, err
	}
	client, _, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	propertiesPath := path.Join(server.RemotePath, "server.properties")
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test -f "$1" && test ! -L "$1"`, "mineops-server-properties", propertiesPath}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return nil, apperror.Wrap(apperror.CodeSFTPPathRejected, "server.properties 必须是普通文件，拒绝符号链接", err)
	}
	document, err := readRemoteText(ctx, client, propertiesPath)
	if err != nil {
		return nil, err
	}
	return serverPropertiesSnapshot(document), nil
}

// SaveProperties 按原文或结构化键更新保存,带冲突检测并保留备份。
func (m *MinecraftServerManager) SaveProperties(ctx context.Context, serverID model.ID, mode, rawContent, expectedVersion string, updates []ServerPropertyUpdate, firewallConfirmed bool) (*ServerPropertiesSnapshot, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, err
	}
	client, _, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	propertiesPath := path.Join(server.RemotePath, "server.properties")
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test -f "$1" && test ! -L "$1"`, "mineops-server-properties", propertiesPath}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return nil, apperror.Wrap(apperror.CodeSFTPPathRejected, "server.properties 必须是普通文件，拒绝符号链接", err)
	}
	current, err := readRemoteText(ctx, client, propertiesPath)
	if err != nil {
		return nil, err
	}
	if expectedVersion == "" || current.VersionToken != expectedVersion {
		return nil, apperror.New(apperror.CodeValidationConflict, "远端 server.properties 已变化，拒绝覆盖").WithDetails(map[string]any{
			"expectedVersion": expectedVersion, "actualVersion": current.VersionToken,
		})
	}
	content := rawContent
	switch mode {
	case "raw":
	case "structured":
		properties := model.ParseProperties(current.Content)
		for _, update := range updates {
			if err := properties.Set(update.Key, update.Value); err != nil {
				return nil, err
			}
		}
		content = properties.Render()
	default:
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "server.properties 保存模式无效")
	}
	oldPort, err := model.ParseProperties(current.Content).ServerPort()
	if err != nil {
		return nil, err
	}
	properties := model.ParseProperties(content)
	newPort, err := properties.ServerPort()
	if err != nil {
		return nil, err
	}
	if _, err := model.NewRemoteTextDocument(propertiesPath, []byte(content), current.ModifiedAt, maximumRemoteTextBytes); err != nil {
		return nil, err
	}
	plan, err := m.firewall.PreparePortChange(ctx, serverID, oldPort, newPort, firewallConfirmed)
	if err != nil {
		return nil, err
	}
	document, err := m.saveServerProperties(ctx, client, server.RemotePath, current, content)
	if err != nil {
		m.firewall.AbortPortChange(context.WithoutCancel(ctx), plan)
		return nil, err
	}
	snapshot := serverPropertiesSnapshot(document)
	if err := m.firewall.CommitPortChange(ctx, plan); err != nil {
		snapshot.ValidationNotice = "配置已保存，但防火墙旧端口清理未完成：" + err.Error()
	}
	return snapshot, nil
}

// ListPropertyBackups 列出某台 Server 保存前保留的历史版本。
func (m *MinecraftServerManager) ListPropertyBackups(ctx context.Context, serverID model.ID) ([]ServerPropertiesBackup, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, err
	}
	client, _, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	managedDirectory := path.Join(server.RemotePath, ".mineops")
	backupDirectory := path.Join(server.RemotePath, ".mineops", "config-backups")
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `set -eu
if [ ! -e "$1" ]; then exit 0; fi
test -d "$1"
test ! -L "$1"
if [ ! -e "$2" ]; then exit 0; fi
test -d "$2"
test ! -L "$2"
find "$2" -mindepth 1 -maxdepth 1 -type f -name 'server.properties-*.bak' -printf '%f\000%p\000%s\000%T@\000'`, "mineops-properties-backups", managedDirectory, backupDirectory},
		Timeout: 15 * time.Second, MaximumOutput: 256 * 1024,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "读取 server.properties 备份列表失败", err)
	}
	fields := strings.Split(result.Stdout, "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%4 != 0 {
		return nil, apperror.New(apperror.CodeIOReadFailed, "server.properties 备份列表格式无效")
	}
	backups := make([]ServerPropertiesBackup, 0, len(fields)/4)
	for index := 0; index < len(fields); index += 4 {
		size, sizeErr := strconv.ParseInt(fields[index+2], 10, 64)
		modified, modifiedErr := strconv.ParseFloat(fields[index+3], 64)
		if sizeErr != nil || modifiedErr != nil || size < 0 || size > maximumRemoteTextBytes {
			return nil, apperror.New(apperror.CodeIOReadFailed, "server.properties 备份元数据无效")
		}
		seconds := int64(modified)
		nanoseconds := int64((modified - float64(seconds)) * float64(time.Second))
		backups = append(backups, ServerPropertiesBackup{
			Name: fields[index], Path: fields[index+1], Size: size,
			ModifiedAt: time.Unix(seconds, nanoseconds).UTC(),
		})
	}
	sort.Slice(backups, func(left, right int) bool { return backups[left].ModifiedAt.After(backups[right].ModifiedAt) })
	return backups, nil
}

// RestorePropertyBackup 在校验当前版本标识后恢复一个保留的历史版本。
func (m *MinecraftServerManager) RestorePropertyBackup(ctx context.Context, serverID model.ID, backupPath, expectedVersion string, firewallConfirmed bool) (*ServerPropertiesSnapshot, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, err
	}
	client, home, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	managedDirectory := path.Join(server.RemotePath, ".mineops")
	backupDirectory := path.Join(server.RemotePath, ".mineops", "config-backups")
	normalizedBackup, err := model.NormalizeRemotePath(backupPath, backupDirectory, home)
	if err != nil {
		return nil, err
	}
	if path.Dir(normalizedBackup) != backupDirectory || !strings.HasPrefix(path.Base(normalizedBackup), "server.properties-") || !strings.HasSuffix(normalizedBackup, ".bak") {
		return nil, apperror.New(apperror.CodeSFTPPathRejected, "配置备份不属于当前 Server")
	}
	propertiesPath := path.Join(server.RemotePath, "server.properties")
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test -d "$1" && test ! -L "$1" && test -d "$2" && test ! -L "$2" && test -f "$3" && test ! -L "$3" && test -f "$4" && test ! -L "$4"`, "mineops-properties-restore", managedDirectory, backupDirectory, normalizedBackup, propertiesPath}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return nil, apperror.Wrap(apperror.CodeSFTPPathRejected, "配置备份必须是普通文件", err)
	}
	backup, err := readRemoteText(ctx, client, normalizedBackup)
	if err != nil {
		return nil, err
	}
	properties := model.ParseProperties(backup.Content)
	newPort, err := properties.ServerPort()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeValidationInvalidArgument, "配置备份中的 server-port 无效", err)
	}
	current, err := readRemoteText(ctx, client, propertiesPath)
	if err != nil {
		return nil, err
	}
	if expectedVersion == "" || current.VersionToken != expectedVersion {
		return nil, apperror.New(apperror.CodeValidationConflict, "远端 server.properties 已变化，拒绝恢复备份").WithDetails(map[string]any{
			"expectedVersion": expectedVersion, "actualVersion": current.VersionToken,
		})
	}
	oldPort, err := model.ParseProperties(current.Content).ServerPort()
	if err != nil {
		return nil, err
	}
	plan, err := m.firewall.PreparePortChange(ctx, serverID, oldPort, newPort, firewallConfirmed)
	if err != nil {
		return nil, err
	}
	document, err := m.saveServerProperties(ctx, client, server.RemotePath, current, backup.Content)
	if err != nil {
		m.firewall.AbortPortChange(context.WithoutCancel(ctx), plan)
		return nil, err
	}
	snapshot := serverPropertiesSnapshot(document)
	if err := m.firewall.CommitPortChange(ctx, plan); err != nil {
		snapshot.ValidationNotice = "配置已恢复，但防火墙旧端口清理未完成：" + err.Error()
	}
	return snapshot, nil
}

func (m *MinecraftServerManager) saveServerProperties(ctx context.Context, client *SSHClient, serverPath string, current *model.RemoteTextDocument, content string) (*model.RemoteTextDocument, error) {
	modeResult, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "stat", Arguments: []string{"--format=%a", "--", current.Path}, Timeout: 5 * time.Second, MaximumOutput: 128,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "读取 server.properties 权限失败", err)
	}
	backupID, err := model.NewID(m.clock.Now())
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成配置备份 ID 失败", err)
	}
	backupDirectory := path.Join(serverPath, ".mineops", "config-backups")
	managedDirectory := path.Join(serverPath, ".mineops")
	backupName := fmt.Sprintf("server.properties-%s-%s.bak", m.clock.Now().UTC().Format("20060102T150405Z"), backupID.String())
	backupPath := path.Join(backupDirectory, backupName)
	temporary := current.Path + ".mineops-tmp-" + backupID.String()
	script := `set -eu
test -f "$1"
test ! -L "$1"
test ! -L "$2"
test ! -e "$2"
test ! -L "$4"
test ! -e "$4"
test ! -L "$7"
if [ -e "$7" ]; then test -d "$7" && test ! -L "$7"; else mkdir --mode=0700 -- "$7"; fi
test ! -L "$3"
if [ -e "$3" ]; then test -d "$3" && test ! -L "$3"; else mkdir --mode=0700 -- "$3"; fi
chmod 0700 -- "$3"
cp -p -- "$1" "$4"
trap 'rm -f -- "$2"' EXIT
umask 077
cat > "$2"
chmod -- "$5" "$2"
mv -f -- "$2" "$1"
trap - EXIT
find "$3" -mindepth 1 -maxdepth 1 -type f -name 'server.properties-*.bak' -printf '%T@ %f\n' | sort -nr | awk -v keep="$6" 'NR > keep { print $2 }' | while IFS= read -r old; do rm -f -- "$3/$old"; done || true`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops-properties-save", current.Path, temporary, backupDirectory, backupPath, strings.TrimSpace(modeResult.Stdout), strconv.Itoa(maximumServerPropertiesBackups), managedDirectory},
		InputReader: strings.NewReader(content), Timeout: 30 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{
			Executable: "rm", Arguments: []string{"-f", "--", temporary}, Timeout: 5 * time.Second, MaximumOutput: 4096,
		})
		return nil, apperror.Wrap(apperror.CodeIOWriteFailed, "原子保存 server.properties 失败，原文件保持不变", err)
	}
	return readRemoteText(ctx, client, current.Path)
}

func serverPropertiesSnapshot(document *model.RemoteTextDocument) *ServerPropertiesSnapshot {
	properties := model.ParseProperties(document.Content)
	lastIndexes := make(map[string]int)
	for index, line := range properties.Lines {
		if line.Kind == model.PropertyEntry {
			lastIndexes[line.Key] = index
		}
	}
	values := make([]ServerPropertyValue, 0, len(lastIndexes))
	for index, line := range properties.Lines {
		if line.Kind == model.PropertyEntry && lastIndexes[line.Key] == index {
			values = append(values, ServerPropertyValue{Key: line.Key, Value: line.Value})
		}
	}
	serverPort, err := properties.ServerPort()
	notice := ""
	if err != nil {
		notice = err.Error()
	}
	return &ServerPropertiesSnapshot{Document: document, Values: values, ServerPort: serverPort, ValidationNotice: notice}
}
