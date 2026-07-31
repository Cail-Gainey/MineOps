package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const (
	maximumRemoteZIPBytes       int64 = 2 * 1024 * 1024 * 1024
	maximumArchiveFiles               = 10_000
	maximumArchiveSingleBytes   int64 = 2 * 1024 * 1024 * 1024
	maximumArchiveExpandedBytes int64 = 8 * 1024 * 1024 * 1024
)

type validatedZIPEntry struct {
	file       *zip.File
	remotePath string
	mode       os.FileMode
}

// StartExtractZIP 启动一个持久化任务:在本地校验远端 ZIP,并原子发布一个新的远端目录。
func (m *FileManager) StartExtractZIP(ctx context.Context, sshSessionID model.ID, archivePath, destinationPath, currentDirectory string) (model.ID, error) {
	if !sshSessionID.Valid() {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
	}
	transferID, err := model.NewID(m.clock.Now())
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, "生成 ZIP 解压目标 ID 失败", err)
	}
	return m.runner.Start(ctx, OperationRequest{
		Type: enums.OperationExtract, TargetType: enums.OperationTargetFile, TargetID: transferID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.extractRemoteZIP(operationCtx, reporter, sshSessionID, archivePath, destinationPath, currentDirectory, transferID)
		},
	})
}

func (m *FileManager) extractRemoteZIP(ctx context.Context, reporter OperationReporter, sshSessionID model.ID, requestedArchive, requestedDestination, currentDirectory string, transferID model.ID) error {
	if err := reporter.SetProgress("prepare", 0.01, "正在检查远程 ZIP 和目标目录"); err != nil {
		return err
	}
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()
	archivePath, err := model.NormalizeRemotePath(requestedArchive, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	destination, err := model.NormalizeRemotePath(requestedDestination, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	if destination == "/" || destination == home || path.Dir(destination) == "/" {
		return apperror.New(apperror.CodeSFTPPathRejected, "ZIP 解压目标不能是根目录、Home 或根目录下一级路径")
	}
	sizeResult, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test -f "$1" && test ! -L "$1" && stat --format=%s -- "$1"`, "mineops-zip-extract", archivePath},
		Timeout: 10 * time.Second, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "ZIP 来源必须是远程普通文件，拒绝符号链接", err)
	}
	var archiveSize int64
	if _, err := fmt.Sscan(strings.TrimSpace(sizeResult.Stdout), &archiveSize); err != nil || archiveSize < 0 || archiveSize > maximumRemoteZIPBytes {
		return apperror.New(apperror.CodeSFTPTransferFailed, "远程 ZIP 大小无效或超过 2 GiB 上限")
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "test", Arguments: []string{"-d", path.Dir(destination)}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "ZIP 解压目标父目录不存在", err)
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "test", Arguments: []string{"!", "-e", destination}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeValidationConflict, "ZIP 解压目标已存在，拒绝合并或覆盖", err)
	}

	localArchive, err := os.CreateTemp("", "mineops-remote-zip-*.zip")
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 ZIP 校验临时文件失败", err)
	}
	localArchivePath := localArchive.Name()
	defer func() {
		_ = localArchive.Close()
		_ = os.Remove(localArchivePath)
	}()
	archiveHash := sha256.New()
	downloadProgress := newTransferProgress(reporter, "inspect", "正在流式读取远程 ZIP 以执行安全校验", archiveSize, 0, 0.03, 0.17)
	downloadWriter := &progressWriter{ctx: ctx, writer: io.MultiWriter(localArchive, archiveHash), progress: downloadProgress}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "cat", Arguments: []string{"--", archivePath}, OutputWriter: downloadWriter, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPTransferFailed, "读取远程 ZIP 流失败", err).WithRetryable(true)
	}
	if downloadWriter.written != archiveSize {
		return apperror.New(apperror.CodeSFTPTransferFailed, "ZIP 读取字节数与远程大小不一致").WithRetryable(true)
	}
	remoteHash, err := remoteSHA256(ctx, client, archivePath)
	if err != nil {
		return err
	}
	if localHash := hex.EncodeToString(archiveHash.Sum(nil)); localHash != remoteHash {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "远程 ZIP 在校验期间发生变化").WithDetails(map[string]any{
			"remoteSHA256": remoteHash, "localSHA256": localHash,
		}).WithRetryable(true)
	}
	if err := localArchive.Sync(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "同步 ZIP 校验临时文件失败", err)
	}
	entries, totalExpanded, err := validateZIPArchive(localArchive, archiveSize, destination)
	if err != nil {
		return err
	}
	if err := reporter.SetProgress("extract", 0.21, fmt.Sprintf("安全校验通过，正在解压 %d 个条目", len(entries))); err != nil {
		return err
	}

	staging := destination + ".mineops-extract-" + transferID.String()
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
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建远程 ZIP 解压临时目录失败", err)
	}

	var extractedBytes int64
	directories := make([]validatedZIPEntry, 0)
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		relative := strings.TrimPrefix(entry.remotePath, strings.TrimSuffix(destination, "/")+"/")
		stagingPath := path.Join(staging, relative)
		if entry.file.FileInfo().IsDir() {
			if _, err := client.RunCommand(ctx, RemoteCommand{
				Executable: "mkdir", Arguments: []string{"-p", "--", stagingPath}, Timeout: 15 * time.Second, MaximumOutput: 4096,
			}); err != nil {
				return apperror.Wrap(apperror.CodeIOWriteFailed, "创建远程 ZIP 目录失败", err)
			}
			directories = append(directories, entry)
			continue
		}
		if _, err := client.RunCommand(ctx, RemoteCommand{
			Executable: "mkdir", Arguments: []string{"-p", "--", path.Dir(stagingPath)}, Timeout: 15 * time.Second, MaximumOutput: 4096,
		}); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "创建远程 ZIP 文件父目录失败", err)
		}
		reader, err := entry.file.Open()
		if err != nil {
			return apperror.Wrap(apperror.CodeIOReadFailed, "打开 ZIP 条目失败", err)
		}
		message := fmt.Sprintf("正在解压 %s（%d/%d）", entry.file.Name, index+1, len(entries))
		progress := newTransferProgress(reporter, "extract", message, totalExpanded, extractedBytes, 0.21, 0.7)
		extractErr := extractZIPFile(ctx, client, reader, stagingPath, entry.mode, entry.file.UncompressedSize64, progress, index)
		closeErr := reader.Close()
		if extractErr != nil {
			return extractErr
		}
		if closeErr != nil {
			return apperror.Wrap(apperror.CodeIOReadFailed, "关闭 ZIP 条目失败", closeErr)
		}
		extractedBytes += int64(entry.file.UncompressedSize64)
	}
	for index := len(directories) - 1; index >= 0; index-- {
		relative := strings.TrimPrefix(directories[index].remotePath, strings.TrimSuffix(destination, "/")+"/")
		mode := directories[index].mode.Perm()
		if mode == 0 {
			mode = 0o755
		}
		if _, err := client.RunCommand(ctx, RemoteCommand{
			Executable: "chmod", Arguments: []string{fmt.Sprintf("%04o", mode), "--", path.Join(staging, relative)}, Timeout: 10 * time.Second, MaximumOutput: 4096,
		}); err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "应用远程 ZIP 目录权限失败", err)
		}
	}
	if err := reporter.SetProgress("publish", 0.95, "正在原子发布解压目录"); err != nil {
		return err
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "mv", Arguments: []string{"--", staging, destination}, Timeout: 30 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "原子发布远程 ZIP 解压目录失败", err)
	}
	published = true
	return reporter.SetProgress("publish", 0.98, fmt.Sprintf("ZIP 已安全解压到 %s", destination))
}

func validateZIPArchive(file *os.File, archiveSize int64, destination string) ([]validatedZIPEntry, int64, error) {
	reader, err := zip.NewReader(file, archiveSize)
	if err != nil {
		return nil, 0, apperror.Wrap(apperror.CodeSFTPTransferFailed, "远程文件不是有效 ZIP", err)
	}
	entries := make([]validatedZIPEntry, 0, len(reader.File))
	seen := make(map[string]struct{}, len(reader.File))
	var total int64
	for _, entry := range reader.File {
		if entry.Mode()&os.ModeSymlink != 0 {
			return nil, 0, apperror.New(apperror.CodeSFTPPathRejected, "ZIP 包含符号链接，已拒绝解压").WithDetails(map[string]any{"entry": entry.Name})
		}
		if !entry.FileInfo().IsDir() && !entry.Mode().IsRegular() {
			return nil, 0, apperror.New(apperror.CodeSFTPPathRejected, "ZIP 包含不支持的特殊文件").WithDetails(map[string]any{"entry": entry.Name})
		}
		remotePath, err := model.NormalizeArchiveEntry(destination, entry.Name)
		if err != nil {
			return nil, 0, err
		}
		if _, exists := seen[remotePath]; exists {
			return nil, 0, apperror.New(apperror.CodeValidationConflict, "ZIP 包含重复目标路径").WithDetails(map[string]any{"entry": entry.Name})
		}
		seen[remotePath] = struct{}{}
		if entry.UncompressedSize64 > uint64(maximumArchiveSingleBytes) || total > maximumArchiveExpandedBytes-int64(entry.UncompressedSize64) {
			return nil, 0, apperror.New(apperror.CodeSFTPTransferFailed, "ZIP 解压体积超过安全限制").WithDetails(map[string]any{"entry": entry.Name})
		}
		total += int64(entry.UncompressedSize64)
		if err := model.ValidateArchiveLimits(len(entries)+1, int64(entry.UncompressedSize64), total, maximumArchiveFiles, maximumArchiveSingleBytes, maximumArchiveExpandedBytes); err != nil {
			return nil, 0, err
		}
		entries = append(entries, validatedZIPEntry{file: entry, remotePath: remotePath, mode: entry.Mode()})
	}
	return entries, total, nil
}

func extractZIPFile(ctx context.Context, client *SSHClient, reader io.Reader, target string, mode os.FileMode, expectedSize uint64, progress *transferProgress, index int) error {
	temporary := target + fmt.Sprintf(".mineops-zip-tmp-%d", index)
	hash := sha256.New()
	stream := &progressReader{ctx: ctx, reader: io.TeeReader(reader, hash), progress: progress}
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `umask 077 && cat > "$1" && sha256sum -- "$1"`, "mineops-zip-entry", temporary},
		InputReader: stream, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeSFTPTransferFailed, "写入远程 ZIP 条目失败", err).WithRetryable(true)
	}
	if uint64(stream.read) != expectedSize {
		return apperror.New(apperror.CodeSFTPTransferFailed, "ZIP 条目实际大小与目录记录不一致").WithDetails(map[string]any{
			"expectedBytes": expectedSize, "actualBytes": stream.read,
		})
	}
	fields := strings.Fields(result.Stdout)
	localHash := hex.EncodeToString(hash.Sum(nil))
	if len(fields) == 0 || !strings.EqualFold(fields[0], localHash) {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "ZIP 条目远程写入校验失败")
	}
	permissions := mode.Perm()
	if permissions == 0 {
		permissions = 0o644
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `chmod -- "$1" "$2" && mv -- "$2" "$3"`, "mineops-zip-entry", fmt.Sprintf("%04o", permissions), temporary, target},
		Timeout: 15 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "发布远程 ZIP 条目失败", err)
	}
	return nil
}
