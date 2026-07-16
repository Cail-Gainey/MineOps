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
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

const (
	maximumRemoteTextBytes    = 2 * 1024 * 1024
	remoteDirectoryFindFormat = `%y\000%f\000%s\000%m\000%T@\000%l\000`
)

// RemoteDirectory contains one normalized directory, remote Home, and stable-sorted entries.
type RemoteDirectory struct {
	Path    string             `json:"path"`
	Home    string             `json:"home"`
	Entries []model.RemoteFile `json:"entries"`
}

// FileManager provides remote Linux file operations through bounded structured SSH commands.
type FileManager struct {
	clock    model.Clock
	store    repository.Store
	settings *appsettings.Manager
	clients  *SSHClientFactory
	runner   *OperationRunner
}

// NewFileManager creates the remote file application service without adding an SFTP dependency.
func NewFileManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, clients *SSHClientFactory, runner *OperationRunner) (*FileManager, error) {
	if clock == nil || store == nil || settings == nil || clients == nil || runner == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "FileManager 依赖不能为空")
	}
	return &FileManager{clock: clock, store: store, settings: settings, clients: clients, runner: runner}, nil
}

// List returns one remote directory with directories first and names in stable order.
func (m *FileManager) List(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory string) (RemoteDirectory, error) {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return RemoteDirectory{}, err
	}
	defer func() { _ = client.Close() }()
	normalized, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return RemoteDirectory{}, err
	}
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "find", Arguments: []string{normalized, "-mindepth", "1", "-maxdepth", "1", "-printf", remoteDirectoryFindFormat},
		Timeout: 20 * time.Second, MaximumOutput: 4 * 1024 * 1024,
	})
	if err != nil {
		return RemoteDirectory{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取远程目录失败", err)
	}
	if result.Truncated {
		return RemoteDirectory{}, apperror.New(apperror.CodeSFTPTransferFailed, "远程目录条目超过显示上限")
	}
	fields := strings.Split(result.Stdout, "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%6 != 0 {
		return RemoteDirectory{}, apperror.New(apperror.CodeSFTPTransferFailed, "远程目录响应格式无效")
	}
	entries := make([]model.RemoteFile, 0, len(fields)/6)
	for index := 0; index < len(fields); index += 6 {
		size, sizeErr := strconv.ParseInt(fields[index+2], 10, 64)
		mode, modeErr := strconv.ParseUint(fields[index+3], 8, 32)
		modified, modifiedErr := strconv.ParseFloat(fields[index+4], 64)
		if sizeErr != nil || modeErr != nil || modifiedErr != nil {
			return RemoteDirectory{}, apperror.New(apperror.CodeSFTPTransferFailed, "远程目录元数据无效")
		}
		seconds := int64(modified)
		nanoseconds := int64((modified - float64(seconds)) * float64(time.Second))
		entries = append(entries, model.RemoteFile{
			Path: path.Join(normalized, fields[index+1]), Name: fields[index+1], Kind: remoteFileKind(fields[index]),
			Size: size, Mode: uint32(mode), ModifiedAt: time.Unix(seconds, nanoseconds).UTC(), LinkTarget: fields[index+5],
		})
	}
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].Kind == model.RemoteFileDirectory && entries[right].Kind != model.RemoteFileDirectory {
			return true
		}
		if entries[left].Kind != model.RemoteFileDirectory && entries[right].Kind == model.RemoteFileDirectory {
			return false
		}
		return strings.ToLower(entries[left].Name) < strings.ToLower(entries[right].Name)
	})
	return RemoteDirectory{Path: normalized, Home: home, Entries: entries}, nil
}

// ReadText reads one bounded UTF-8 remote document and rejects concurrent modification during the read.
func (m *FileManager) ReadText(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory string) (*model.RemoteTextDocument, error) {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	normalized, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return nil, err
	}
	return readRemoteText(ctx, client, normalized)
}

// SaveText performs conflict detection and atomically replaces one existing remote text file.
func (m *FileManager) SaveText(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory, content, expectedVersion string) (*model.RemoteTextDocument, error) {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	normalized, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return nil, err
	}
	current, err := readRemoteText(ctx, client, normalized)
	if err != nil {
		return nil, err
	}
	if expectedVersion == "" || current.VersionToken != expectedVersion {
		return nil, apperror.New(apperror.CodeValidationConflict, "远端文件已变化，拒绝覆盖").WithDetails(map[string]any{
			"expectedVersion": expectedVersion, "actualVersion": current.VersionToken,
		})
	}
	if _, err := model.NewRemoteTextDocument(normalized, []byte(content), current.ModifiedAt, maximumRemoteTextBytes); err != nil {
		return nil, err
	}
	modeResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "stat", Arguments: []string{"--format=%a", "--", normalized}, Timeout: 5 * time.Second, MaximumOutput: 128})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "读取远程文件权限失败", err)
	}
	temporary := normalized + ".mineops-tmp-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	script := `umask 077; cat > "$1" && chmod -- "$2" "$1" && mv -f -- "$1" "$3"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops-file-save", temporary, strings.TrimSpace(modeResult.Stdout), normalized},
		Input: []byte(content), Timeout: 30 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{Executable: "rm", Arguments: []string{"-f", "--", temporary}, Timeout: 5 * time.Second})
		return nil, apperror.Wrap(apperror.CodeIOWriteFailed, "保存远程文本失败", err)
	}
	return readRemoteText(ctx, client, normalized)
}

// SaveTextAs atomically creates a new remote text file and refuses to replace an existing path.
func (m *FileManager) SaveTextAs(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory, content string) (*model.RemoteTextDocument, error) {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	normalized, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return nil, err
	}
	if _, err := model.NewRemoteTextDocument(normalized, []byte(content), time.Unix(0, 0), maximumRemoteTextBytes); err != nil {
		return nil, err
	}
	temporary := normalized + ".mineops-tmp-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	script := `umask 077; cat > "$1" && chmod 0600 -- "$1" && ln -- "$1" "$2" && rm -f -- "$1"`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops-file-create", temporary, normalized},
		Input: []byte(content), Timeout: 30 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{Executable: "rm", Arguments: []string{"-f", "--", temporary}, Timeout: 5 * time.Second})
		return nil, apperror.Wrap(apperror.CodeIOWriteFailed, "另存远程文本失败，目标可能已存在", err)
	}
	return readRemoteText(ctx, client, normalized)
}

// CreateDirectory creates one remote directory without recursively creating missing parents.
func (m *FileManager) CreateDirectory(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory string) error {
	return m.runPathCommand(ctx, sshSessionID, requestedPath, currentDirectory, RemoteCommand{Executable: "mkdir", Arguments: []string{"--mode=0755", "--"}})
}

// CreateFile creates one empty remote file and refuses to overwrite an existing entry.
func (m *FileManager) CreateFile(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory string) error {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	normalized, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	script := `umask 077; set -C; : > "$1"`
	_, err = client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", script, "mineops-file-create", normalized}, Timeout: 10 * time.Second, MaximumOutput: 4096})
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建远程文件失败，目标可能已存在", err)
	}
	return nil
}

// Rename moves one remote entry after normalizing both source and destination paths.
func (m *FileManager) Rename(ctx context.Context, sshSessionID model.ID, sourcePath, targetPath, currentDirectory string) error {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	source, err := model.NormalizeRemotePath(sourcePath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	target, err := model.NormalizeRemotePath(targetPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	script := `test ! -e "$2" && mv -- "$1" "$2"`
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "sh", Arguments: []string{"-c", script, "mineops-file-rename", source, target}, Timeout: 15 * time.Second, MaximumOutput: 4096}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "重命名远程条目失败，目标可能已存在", err)
	}
	return nil
}

// Delete removes one remote file or directory after applying dangerous-path rejection.
func (m *FileManager) Delete(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory string, recursive bool) error {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	normalized, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	if err := model.ValidateRemoteDelete(normalized, home, recursive); err != nil {
		return err
	}
	arguments := []string{"-f", "--", normalized}
	if recursive {
		arguments = []string{"-rf", "--", normalized}
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "rm", Arguments: arguments, Timeout: 30 * time.Second, MaximumOutput: 4096}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除远程条目失败", err)
	}
	return nil
}

// Chmod changes one remote entry mode using a validated octal value.
func (m *FileManager) Chmod(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory, mode string) error {
	parsed, err := strconv.ParseUint(strings.TrimSpace(mode), 8, 32)
	if err != nil || parsed > 0o7777 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "远程权限必须是 0000 到 7777 的八进制值")
	}
	command := RemoteCommand{Executable: "chmod", Arguments: []string{fmt.Sprintf("%04o", parsed), "--"}, Timeout: 10 * time.Second, MaximumOutput: 4096}
	return m.runPathCommand(ctx, sshSessionID, requestedPath, currentDirectory, command)
}

func (m *FileManager) runPathCommand(ctx context.Context, sshSessionID model.ID, requestedPath, currentDirectory string, command RemoteCommand) error {
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	normalized, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	command.Arguments = append(command.Arguments, normalized)
	if _, err := client.RunCommand(ctx, command); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "远程文件操作失败", err)
	}
	return nil
}

func (m *FileManager) connect(ctx context.Context, sshSessionID model.ID) (*SSHClient, string, error) {
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
	if err != nil {
		_ = client.Close()
		return nil, "", apperror.Wrap(apperror.CodeSFTPPathRejected, "读取远程 Home 目录失败", err)
	}
	if strings.TrimSpace(homeResult.Stdout) == "" {
		_ = client.Close()
		return nil, "", apperror.New(apperror.CodeSFTPPathRejected, "远程 Home 目录为空")
	}
	home, err := model.NormalizeRemotePath(strings.TrimSpace(homeResult.Stdout), "/", "/")
	if err != nil {
		_ = client.Close()
		return nil, "", err
	}
	return client, home, nil
}

func readRemoteText(ctx context.Context, client *SSHClient, normalized string) (*model.RemoteTextDocument, error) {
	before, err := remoteModifiedAt(ctx, client, normalized)
	if err != nil {
		return nil, err
	}
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "cat", Arguments: []string{"--", normalized}, Timeout: 20 * time.Second, MaximumOutput: maximumRemoteTextBytes + 1})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "读取远程文本失败", err)
	}
	if result.Truncated || int64(len(result.Stdout)) > maximumRemoteTextBytes {
		return nil, apperror.New(apperror.CodeSFTPTransferFailed, "远程文本超过 2 MiB 读取限制")
	}
	after, err := remoteModifiedAt(ctx, client, normalized)
	if err != nil {
		return nil, err
	}
	if !before.Equal(after) {
		return nil, apperror.New(apperror.CodeValidationConflict, "读取期间远端文件发生变化，请重试")
	}
	return model.NewRemoteTextDocument(normalized, []byte(result.Stdout), after, maximumRemoteTextBytes)
}

func remoteModifiedAt(ctx context.Context, client *SSHClient, normalized string) (time.Time, error) {
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "stat", Arguments: []string{"--format=%Y", "--", normalized}, Timeout: 5 * time.Second, MaximumOutput: 128})
	if err != nil {
		return time.Time{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取远程文件修改时间失败", err)
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(result.Stdout), 10, 64)
	if err != nil {
		return time.Time{}, apperror.Wrap(apperror.CodeSFTPTransferFailed, "远程文件修改时间无效", err)
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func defaultRemoteDirectory(currentDirectory, home string) string {
	if strings.TrimSpace(currentDirectory) == "" {
		return home
	}
	return currentDirectory
}

func remoteFileKind(value string) model.RemoteFileKind {
	switch value {
	case "f":
		return model.RemoteFileRegular
	case "d":
		return model.RemoteFileDirectory
	case "l":
		return model.RemoteFileSymlink
	default:
		return model.RemoteFileOther
	}
}
