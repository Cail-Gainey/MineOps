package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const (
	maximumServerBackupArchiveBytes int64 = 20 * 1024 * 1024 * 1024
	maximumServerBackupFiles              = 100_000
	maximumServerBackupSingleBytes  int64 = 8 * 1024 * 1024 * 1024
	maximumServerBackupExpanded     int64 = 64 * 1024 * 1024 * 1024
)

// ServerBackup is one verified MineOps-managed remote Server backup archive.
type ServerBackup struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type validatedTarEntry struct {
	relative string
	mode     os.FileMode
	size     int64
	dir      bool
}

// ListBackups returns MineOps-managed archives for one Server without reading archive payloads.
func (m *MinecraftServerManager) ListBackups(ctx context.Context, serverID model.ID) ([]ServerBackup, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return nil, err
	}
	client, home, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	backupDirectory := path.Join(home, "MineOps", "Backup")
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `set -eu
if [ ! -e "$1" ]; then exit 0; fi
test -d "$1"
test ! -L "$1"
find "$1" -mindepth 1 -maxdepth 1 -type f -name "$2" -printf '%f\000%p\000%s\000%T@\000'`, "mineops-server-backups", backupDirectory, server.DirectoryName + "-*.tar.gz"},
		Timeout: 20 * time.Second, MaximumOutput: 1024 * 1024,
	})
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "读取远程 Server 备份列表失败", err)
	}
	fields := strings.Split(result.Stdout, "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%4 != 0 {
		return nil, apperror.New(apperror.CodeIOReadFailed, "远程 Server 备份列表格式无效")
	}
	backups := make([]ServerBackup, 0, len(fields)/4)
	for index := 0; index < len(fields); index += 4 {
		size, sizeErr := strconv.ParseInt(fields[index+2], 10, 64)
		modified, modifiedErr := strconv.ParseFloat(fields[index+3], 64)
		if sizeErr != nil || modifiedErr != nil {
			return nil, apperror.New(apperror.CodeIOReadFailed, "远程 Server 备份元数据无效")
		}
		hashResult, hashErr := client.RunCommand(ctx, RemoteCommand{
			Executable: "cat", Arguments: []string{"--", fields[index+1] + ".sha256"}, Timeout: 5 * time.Second, MaximumOutput: 4096,
		})
		hash := ""
		if hashErr == nil {
			hashFields := strings.Fields(hashResult.Stdout)
			if len(hashFields) > 0 && len(hashFields[0]) == sha256.Size*2 {
				_, decodeErr := hex.DecodeString(hashFields[0])
				if decodeErr == nil {
					hash = strings.ToLower(hashFields[0])
				}
			}
		}
		seconds := int64(modified)
		nanoseconds := int64((modified - float64(seconds)) * float64(time.Second))
		backups = append(backups, ServerBackup{
			Name: fields[index], Path: fields[index+1], Size: size, SHA256: hash,
			ModifiedAt: time.Unix(seconds, nanoseconds).UTC(),
		})
	}
	sort.Slice(backups, func(left, right int) bool { return backups[left].ModifiedAt.After(backups[right].ModifiedAt) })
	return backups, nil
}

// StartBackup starts a remote tar.gz backup Operation for a stopped or ready Server.
func (m *MinecraftServerManager) StartBackup(ctx context.Context, serverID model.ID) (model.ID, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return "", err
	}
	if server.State != enums.LifecycleStopped && server.State != enums.LifecycleReady {
		return "", apperror.New(apperror.CodeValidationConflict, "仅 Stopped 或 Ready Server 可以创建一致性目录备份")
	}
	return m.runner.Start(ctx, OperationRequest{
		Type: enums.OperationBackup, TargetType: enums.OperationTargetServer, TargetID: server.ID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.createServerBackup(operationCtx, reporter, server)
		},
	})
}

// StartRestoreBackup starts safe validation and atomic directory replacement from one managed backup.
func (m *MinecraftServerManager) StartRestoreBackup(ctx context.Context, serverID model.ID, backupPath string) (model.ID, error) {
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return "", err
	}
	if server.State != enums.LifecycleStopped && server.State != enums.LifecycleReady {
		return "", apperror.New(apperror.CodeValidationConflict, "仅 Stopped 或 Ready Server 可以恢复目录备份")
	}
	return m.runner.Start(ctx, OperationRequest{
		Type: enums.OperationRestore, TargetType: enums.OperationTargetServer, TargetID: server.ID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.restoreServerBackup(operationCtx, reporter, server, backupPath)
		},
	})
}

func (m *MinecraftServerManager) createServerBackup(ctx context.Context, reporter OperationReporter, server *model.MinecraftServer) (executionErr error) {
	originalState := server.State
	server.State = enums.LifecycleBackingUp
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return err
	}
	defer func() {
		server.State = originalState
		server.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.MinecraftServers().Update(context.WithoutCancel(ctx), server); err != nil {
			executionErr = errors.Join(executionErr, err)
		}
	}()
	if err := reporter.SetProgress("prepare", 0.05, "正在检查 Server 目录和备份目标"); err != nil {
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
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test -d "$1" && test ! -L "$1" && test "$(stat --format=%d -- "$1")" = "$(stat --format=%d -- "$(dirname -- "$1")")"`, "mineops-server-backup", remotePath}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "Server 备份来源不是普通目录或位于独立挂载根", err)
	}
	specialResult, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "find", Arguments: []string{remotePath, "-xdev", "!", "-type", "f", "!", "-type", "d", "-print", "-quit"}, Timeout: 20 * time.Second, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "检查 Server 备份特殊文件失败", err)
	}
	if strings.TrimSpace(specialResult.Stdout) != "" {
		return apperror.New(apperror.CodeSFTPPathRejected, "Server 目录包含符号链接或特殊文件，拒绝创建不可安全恢复的备份").WithDetails(map[string]any{
			"path": strings.TrimSpace(specialResult.Stdout),
		})
	}
	backupDirectory := path.Join(home, "MineOps", "Backup")
	backupID, err := model.NewID(m.clock.Now())
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, "生成 Server 备份 ID 失败", err)
	}
	filename := fmt.Sprintf("%s-%s-%s.tar.gz", server.DirectoryName, m.clock.Now().UTC().Format("20060102T150405Z"), backupID.String())
	destination := path.Join(backupDirectory, filename)
	temporary := destination + ".partial"
	sidecar := destination + ".sha256"
	completed := false
	defer func() {
		if !completed {
			_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{
				Executable: "rm", Arguments: []string{"-f", "--", temporary, destination, sidecar}, Timeout: 15 * time.Second, MaximumOutput: 4096,
			})
		}
	}()
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "mkdir", Arguments: []string{"-p", "--", backupDirectory}, Timeout: 15 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建远程 Server 备份目录失败", err)
	}
	if err := reporter.SetProgress("archive", 0.25, "正在创建远程 tar.gz 归档"); err != nil {
		return err
	}
	script := `set -eu
tar --one-file-system --hard-dereference -C "$1" -czf "$3" -- "$2"
hash=$(sha256sum -- "$3" | awk '{print $1}')
size=$(stat --format=%s -- "$3")
test "$size" -le "$6"
mv -- "$3" "$4"
printf '%s  %s\n' "$hash" "$7" > "$5"
printf '%s\n%s\n' "$hash" "$size"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops-server-backup", path.Dir(remotePath), path.Base(remotePath), temporary, destination, sidecar, strconv.FormatInt(maximumServerBackupArchiveBytes, 10), filename},
		MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建远程 Server 备份失败", err).WithRetryable(true)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) < 2 || len(fields[0]) != sha256.Size*2 {
		return apperror.New(apperror.CodeIOWriteFailed, "远程 Server 备份校验响应无效")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return apperror.New(apperror.CodeIOWriteFailed, "远程 Server 备份校验响应无效")
	}
	completed = true
	return reporter.SetProgress("complete", 0.98, fmt.Sprintf("备份已创建：%s · %s bytes", filename, fields[1]))
}

func (m *MinecraftServerManager) restoreServerBackup(ctx context.Context, reporter OperationReporter, server *model.MinecraftServer, requestedBackup string) (executionErr error) {
	originalState := server.State
	server.State = enums.LifecycleUpdating
	server.UpdatedAt = m.clock.Now().UTC()
	if err := m.store.MinecraftServers().Update(ctx, server); err != nil {
		return err
	}
	defer func() {
		server.State = originalState
		server.UpdatedAt = m.clock.Now().UTC()
		if err := m.store.MinecraftServers().Update(context.WithoutCancel(ctx), server); err != nil {
			executionErr = errors.Join(executionErr, err)
		}
	}()
	if err := reporter.SetProgress("prepare", 0.03, "正在验证 Server 备份来源"); err != nil {
		return err
	}
	client, home, err := m.connectRemoteServer(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	backupDirectory := path.Join(home, "MineOps", "Backup")
	backupPath, err := model.NormalizeRemotePath(requestedBackup, backupDirectory, home)
	if err != nil {
		return err
	}
	if path.Dir(backupPath) != backupDirectory || !strings.HasPrefix(path.Base(backupPath), server.DirectoryName+"-") || !strings.HasSuffix(backupPath, ".tar.gz") {
		return apperror.New(apperror.CodeSFTPPathRejected, "备份不属于当前 Server 的 MineOps 受控目录")
	}
	sizeResult, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test -f "$1" && test ! -L "$1" && stat --format=%s -- "$1"`, "mineops-server-restore", backupPath},
		Timeout: 10 * time.Second, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "Server 备份必须是普通文件", err)
	}
	var archiveSize int64
	if _, err := fmt.Sscan(strings.TrimSpace(sizeResult.Stdout), &archiveSize); err != nil || archiveSize < 0 || archiveSize > maximumServerBackupArchiveBytes {
		return apperror.New(apperror.CodeSFTPTransferFailed, "Server 备份大小无效或超过 20 GiB 上限")
	}
	hashResult, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "cat", Arguments: []string{"--", backupPath + ".sha256"}, Timeout: 5 * time.Second, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeArtifactChecksumMismatch, "Server 备份缺少 SHA-256 sidecar", err)
	}
	hashFields := strings.Fields(hashResult.Stdout)
	if len(hashFields) == 0 || len(hashFields[0]) != sha256.Size*2 {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "Server 备份 SHA-256 sidecar 无效")
	}
	if _, err := hex.DecodeString(hashFields[0]); err != nil {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "Server 备份 SHA-256 sidecar 无效")
	}
	expectedHash := strings.ToLower(hashFields[0])
	localArchive, err := os.CreateTemp("", "mineops-server-backup-*.tar.gz")
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Server 恢复校验临时文件失败", err)
	}
	localPath := localArchive.Name()
	defer func() {
		_ = localArchive.Close()
		_ = os.Remove(localPath)
	}()
	hash := sha256.New()
	progress := newTransferProgress(reporter, "verify", "正在下载并校验 Server 备份", archiveSize, 0, 0.05, 0.18)
	writer := &progressWriter{ctx: ctx, writer: io.MultiWriter(localArchive, hash), progress: progress}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "cat", Arguments: []string{"--", backupPath}, OutputWriter: writer, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPTransferFailed, "读取 Server 备份流失败", err).WithRetryable(true)
	}
	if writer.written != archiveSize || hex.EncodeToString(hash.Sum(nil)) != expectedHash {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "Server 备份 SHA-256 校验失败").WithRetryable(true)
	}
	entries, expandedBytes, rootMode, err := validateServerTar(localPath, server.DirectoryName)
	if err != nil {
		return err
	}
	if err := reporter.SetProgress("extract", 0.25, fmt.Sprintf("安全校验通过，正在恢复 %d 个条目", len(entries))); err != nil {
		return err
	}
	restoreID, err := model.NewID(m.clock.Now())
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, "生成 Server 恢复 ID 失败", err)
	}
	staging := server.RemotePath + ".mineops-restore-" + restoreID.String()
	previous := server.RemotePath + ".mineops-restore-previous-" + restoreID.String()
	published := false
	defer func() {
		if !published {
			_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{
				Executable: "rm", Arguments: []string{"-rf", "--", staging}, Timeout: 30 * time.Second, MaximumOutput: 4096,
			})
		}
	}()
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "mkdir", Arguments: []string{"--mode=0700", "--", staging}, Timeout: 15 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Server 恢复临时目录失败", err)
	}
	if err := extractServerTar(ctx, reporter, client, localPath, staging, server.DirectoryName, entries, expandedBytes); err != nil {
		return err
	}
	if rootMode == 0 {
		rootMode = 0o755
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "chmod", Arguments: []string{fmt.Sprintf("%04o", rootMode.Perm()), "--", staging}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "应用 Server 恢复根目录权限失败", err)
	}
	if err := reporter.SetProgress("publish", 0.94, "正在原子替换 Server 目录"); err != nil {
		return err
	}
	swapScript := `set -eu
test -d "$1"
test ! -e "$2"
mv -- "$1" "$2"
if mv -- "$3" "$1"; then rm -rf -- "$2"; else mv -- "$2" "$1"; exit 1; fi`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", swapScript, "mineops-server-restore", server.RemotePath, previous, staging}, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "原子替换 Server 恢复目录失败", err)
	}
	published = true
	return reporter.SetProgress("complete", 0.98, "Server 目录已从备份安全恢复")
}

func validateServerTar(localPath, directoryName string) ([]validatedTarEntry, int64, os.FileMode, error) {
	file, gzipReader, tarReader, err := openServerTar(localPath)
	if err != nil {
		return nil, 0, 0, err
	}
	defer func() {
		_ = file.Close()
	}()
	defer func() {
		_ = gzipReader.Close()
	}()
	entries := make([]validatedTarEntry, 0)
	seen := make(map[string]struct{})
	var total int64
	var rootMode os.FileMode
	rootFound := false
	for {
		header, readErr := tarReader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, 0, 0, apperror.Wrap(apperror.CodeSFTPTransferFailed, "读取 Server 备份目录失败", readErr)
		}
		cleaned := path.Clean(strings.ReplaceAll(header.Name, "\\", "/"))
		if cleaned == directoryName {
			if rootFound || header.Typeflag != tar.TypeDir || header.Size != 0 {
				return nil, 0, 0, apperror.New(apperror.CodeSFTPPathRejected, "Server 备份根条目无效或重复")
			}
			rootFound = true
			rootMode = os.FileMode(header.Mode).Perm()
			continue
		}
		prefix := directoryName + "/"
		if !strings.HasPrefix(cleaned, prefix) {
			return nil, 0, 0, apperror.New(apperror.CodeSFTPPathRejected, "Server 备份包含目录前缀外条目").WithDetails(map[string]any{"entry": header.Name})
		}
		relative := strings.TrimPrefix(cleaned, prefix)
		if _, err := model.NormalizeArchiveEntry("/restore", relative); err != nil {
			return nil, 0, 0, err
		}
		if _, exists := seen[relative]; exists {
			return nil, 0, 0, apperror.New(apperror.CodeValidationConflict, "Server 备份包含重复路径").WithDetails(map[string]any{"entry": header.Name})
		}
		seen[relative] = struct{}{}
		directory := header.Typeflag == tar.TypeDir
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 && !directory {
			return nil, 0, 0, apperror.New(apperror.CodeSFTPPathRejected, "Server 备份包含链接或特殊文件").WithDetails(map[string]any{"entry": header.Name})
		}
		if header.Size < 0 || header.Size > maximumServerBackupSingleBytes || total > maximumServerBackupExpanded-header.Size {
			return nil, 0, 0, apperror.New(apperror.CodeSFTPTransferFailed, "Server 备份展开体积超过限制")
		}
		total += header.Size
		if err := model.ValidateArchiveLimits(len(entries)+1, header.Size, total, maximumServerBackupFiles, maximumServerBackupSingleBytes, maximumServerBackupExpanded); err != nil {
			return nil, 0, 0, err
		}
		entries = append(entries, validatedTarEntry{relative: relative, mode: os.FileMode(header.Mode), size: header.Size, dir: directory})
	}
	if !rootFound {
		return nil, 0, 0, apperror.New(apperror.CodeSFTPPathRejected, "Server 备份缺少预期根目录条目")
	}
	return entries, total, rootMode, nil
}

func extractServerTar(ctx context.Context, reporter OperationReporter, client *SSHClient, localPath, staging, directoryName string, entries []validatedTarEntry, totalBytes int64) error {
	file, gzipReader, tarReader, err := openServerTar(localPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	defer func() {
		_ = gzipReader.Close()
	}()
	entryIndex := 0
	var completed int64
	directories := make([]validatedTarEntry, 0)
	for {
		header, readErr := tarReader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return apperror.Wrap(apperror.CodeIOReadFailed, "读取 Server 备份条目失败", readErr)
		}
		cleaned := path.Clean(strings.ReplaceAll(header.Name, "\\", "/"))
		if cleaned == directoryName {
			continue
		}
		if entryIndex >= len(entries) {
			return apperror.New(apperror.CodeSFTPTransferFailed, "Server 备份条目数量在恢复时发生变化")
		}
		entry := entries[entryIndex]
		entryIndex++
		if cleaned != path.Join(directoryName, entry.relative) || entry.dir != (header.Typeflag == tar.TypeDir) || entry.size != header.Size {
			return apperror.New(apperror.CodeSFTPTransferFailed, "Server 备份条目在恢复时发生变化")
		}
		target := path.Join(staging, entry.relative)
		if entry.dir {
			if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "mkdir", Arguments: []string{"-p", "--", target}, Timeout: 15 * time.Second, MaximumOutput: 4096}); err != nil {
				return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Server 恢复目录失败", err)
			}
			directories = append(directories, entry)
			continue
		}
		if _, err := client.RunCommand(ctx, RemoteCommand{Executable: "mkdir", Arguments: []string{"-p", "--", path.Dir(target)}, Timeout: 15 * time.Second, MaximumOutput: 4096}); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Server 恢复父目录失败", err)
		}
		message := fmt.Sprintf("正在恢复 %s（%d/%d）", entry.relative, entryIndex, len(entries))
		progress := newTransferProgress(reporter, "extract", message, totalBytes, completed, 0.25, 0.66)
		if err := extractTarFile(ctx, client, tarReader, target, entry, progress, entryIndex); err != nil {
			return err
		}
		completed += entry.size
	}
	if entryIndex != len(entries) {
		return apperror.New(apperror.CodeSFTPTransferFailed, "Server 备份条目数量在恢复时发生变化")
	}
	for index := len(directories) - 1; index >= 0; index-- {
		mode := directories[index].mode.Perm()
		if mode == 0 {
			mode = 0o755
		}
		if _, err := client.RunCommand(ctx, RemoteCommand{
			Executable: "chmod", Arguments: []string{fmt.Sprintf("%04o", mode), "--", path.Join(staging, directories[index].relative)}, Timeout: 10 * time.Second, MaximumOutput: 4096,
		}); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "应用 Server 恢复目录权限失败", err)
		}
	}
	return nil
}

func extractTarFile(ctx context.Context, client *SSHClient, reader io.Reader, target string, entry validatedTarEntry, progress *transferProgress, index int) error {
	temporary := target + fmt.Sprintf(".mineops-restore-tmp-%d", index)
	hash := sha256.New()
	stream := &progressReader{ctx: ctx, reader: io.TeeReader(io.LimitReader(reader, entry.size), hash), progress: progress}
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `umask 077 && cat > "$1" && sha256sum -- "$1"`, "mineops-server-restore", temporary},
		InputReader: stream, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeSFTPTransferFailed, "写入 Server 恢复条目失败", err).WithRetryable(true)
	}
	if stream.read != entry.size {
		return apperror.New(apperror.CodeSFTPTransferFailed, "Server 恢复条目大小不一致")
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) == 0 || !strings.EqualFold(fields[0], hex.EncodeToString(hash.Sum(nil))) {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "Server 恢复条目 SHA-256 校验失败")
	}
	mode := entry.mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `chmod -- "$1" "$2" && mv -- "$2" "$3"`, "mineops-server-restore", fmt.Sprintf("%04o", mode), temporary, target},
		Timeout: 15 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "发布 Server 恢复条目失败", err)
	}
	return nil
}

func openServerTar(localPath string) (*os.File, *gzip.Reader, *tar.Reader, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, nil, nil, apperror.Wrap(apperror.CodeIOReadFailed, "打开 Server 备份临时文件失败", err)
	}
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, nil, apperror.Wrap(apperror.CodeSFTPTransferFailed, "Server 备份不是有效 gzip", err)
	}
	return file, gzipReader, tar.NewReader(gzipReader), nil
}
