package service

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ServerInstallationStatus 把远端文件与持久化的 Server 及安装元数据比对后给出汇总。
type ServerInstallationStatus struct {
	State                string                  `json:"state"`
	DirectoryFound       bool                    `json:"directoryFound"`
	ArtifactFound        bool                    `json:"artifactFound"`
	PropertiesFound      bool                    `json:"propertiesFound"`
	EULAAccepted         bool                    `json:"eulaAccepted"`
	JavaRuntimeBound     bool                    `json:"javaRuntimeBound"`
	ArtifactPath         string                  `json:"artifactPath"`
	ArtifactSize         int64                   `json:"artifactSize"`
	ActualArtifactHash   string                  `json:"actualArtifactHash,omitempty"`
	ExpectedArtifactHash string                  `json:"expectedArtifactHash,omitempty"`
	LatestTaskID         string                  `json:"latestTaskID,omitempty"`
	LatestTaskState      enums.InstallationState `json:"latestTaskState,omitempty"`
	Issues               []string                `json:"issues"`
	Warnings             []string                `json:"warnings"`
	InspectedAt          time.Time               `json:"inspectedAt"`
}

// InspectInstallationStatus 比对远端构件、EULA、Properties 与最近的安装检查点。
func (m *MinecraftServerManager) InspectInstallationStatus(ctx context.Context, serverID model.ID) (ServerInstallationStatus, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return ServerInstallationStatus{}, err
	}
	client, _, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return ServerInstallationStatus{}, err
	}
	defer func() { _ = client.Close() }()
	artifactPath := path.Join(server.RemotePath, server.LaunchProfile.JarPath)
	if server.Type == enums.ServerForge || server.Type == enums.ServerNeoForge {
		artifactPath = path.Join(server.RemotePath, "run.sh")
	}
	profile := runtimeProfileForServerType(server.Type)
	propertiesPath := path.Join(server.RemotePath, profile.configurationFile)
	eulaPath := path.Join(server.RemotePath, "eula.txt")
	script := `set -eu
test -d "$1"
test ! -L "$1"
artifact=0
artifact_size=0
artifact_hash=''
if [ -f "$2" ] && [ ! -L "$2" ]; then artifact=1; artifact_size=$(stat --format=%s -- "$2"); artifact_hash=$(sha256sum -- "$2" | awk '{print $1}'); fi
properties=0
if [ -f "$3" ] && [ ! -L "$3" ]; then properties=1; fi
eula=0
if [ -f "$4" ] && [ ! -L "$4" ] && grep -Eiq '^[[:space:]]*eula[[:space:]]*=[[:space:]]*true[[:space:]]*$' "$4"; then eula=1; fi
printf '1\000%s\000%s\000%s\000%s\000%s\000' "$artifact" "$artifact_size" "$artifact_hash" "$properties" "$eula"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops-server-status", server.RemotePath, artifactPath, propertiesPath, eulaPath}, Timeout: 30 * time.Second, MaximumOutput: 4096,
	})
	if err != nil {
		return ServerInstallationStatus{}, apperror.Wrap(apperror.CodeSFTPPathRejected, "Server 安装目录不存在、是符号链接或无法检查", err)
	}
	fields := strings.Split(result.Stdout, "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields) != 6 {
		return ServerInstallationStatus{}, apperror.New(apperror.CodeIOReadFailed, "Server 安装状态响应格式无效")
	}
	artifactSize, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || artifactSize < 0 {
		return ServerInstallationStatus{}, apperror.New(apperror.CodeIOReadFailed, "Server Artifact 大小无效")
	}
	status := ServerInstallationStatus{
		State: "partial", DirectoryFound: fields[0] == "1", ArtifactFound: fields[1] == "1",
		PropertiesFound: fields[4] == "1", EULAAccepted: !profile.requiresEULA || fields[5] == "1", JavaRuntimeBound: server.JavaRuntimeID != nil,
		ArtifactPath: artifactPath, ArtifactSize: artifactSize, ActualArtifactHash: strings.ToLower(fields[3]),
		Issues: make([]string, 0), Warnings: make([]string, 0), InspectedAt: m.clock.Now().UTC(),
	}
	if !status.ArtifactFound {
		status.Issues = append(status.Issues, "启动 Artifact 不存在或不是普通文件")
	}
	if !status.PropertiesFound {
		status.Issues = append(status.Issues, profile.configurationFile+" 不存在或不是普通文件")
	}
	if profile.requiresEULA && (!status.EULAAccepted || !server.EULAAccepted) {
		status.Issues = append(status.Issues, "远端 EULA 与 Server 元数据未同时确认")
	}
	if !status.JavaRuntimeBound {
		status.Issues = append(status.Issues, "Server 尚未绑定 Java Runtime")
	}

	tasks, err := m.store.Installations().ListByServer(ctx, server.ID, 1, 0)
	if err != nil {
		return ServerInstallationStatus{}, err
	}
	mismatch := false
	if len(tasks) == 0 {
		status.Warnings = append(status.Warnings, "没有 MineOps Installation 记录；该 Server 可能由远程导入注册")
		if len(status.Issues) == 0 {
			status.State = "untracked"
		}
		return status, nil
	}
	latest, steps, err := m.store.Installations().Get(ctx, tasks[0].ID)
	if err != nil {
		return ServerInstallationStatus{}, err
	}
	status.LatestTaskID = latest.ID.String()
	status.LatestTaskState = latest.State
	if latest.State != enums.InstallationSucceeded {
		status.Issues = append(status.Issues, "最近一次 Installation 未成功完成")
	}
	var downloadCheckpoint map[string]any
	var installCheckpoint map[string]any
	for _, step := range steps {
		switch step.Name {
		case "download_server":
			downloadCheckpoint = step.Checkpoint
		case "install_server":
			installCheckpoint = step.Checkpoint
		}
	}
	if distribution := checkpointString(downloadCheckpoint, "distribution"); distribution != "" && !strings.EqualFold(distribution, server.Type.String()) {
		status.Issues = append(status.Issues, "Installation 发行版与 Server 元数据不一致")
		mismatch = true
	}
	if version := checkpointString(downloadCheckpoint, "gameVersion"); version != "" && version != server.Version {
		status.Issues = append(status.Issues, "Installation Minecraft 版本与 Server 元数据不一致")
		mismatch = true
	}
	status.ExpectedArtifactHash = strings.ToLower(checkpointString(installCheckpoint, "artifactSHA256"))
	if server.Type != enums.ServerForge && server.Type != enums.ServerNeoForge && status.ExpectedArtifactHash != "" && status.ArtifactFound && !strings.EqualFold(status.ExpectedArtifactHash, status.ActualArtifactHash) {
		status.Issues = append(status.Issues, "远端启动 Artifact SHA-256 与 Installation checkpoint 不一致")
		mismatch = true
	}
	if mismatch {
		status.State = "mismatch"
	} else if len(status.Issues) == 0 && latest.State == enums.InstallationSucceeded {
		status.State = "consistent"
	}
	return status, nil
}
