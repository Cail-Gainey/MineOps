package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const maximumTransferBatchFiles = 100

type uploadSource struct {
	path string
	name string
	size int64
}

// StartUpload starts one durable Operation that streams selected local files through SSH stdin.
func (m *FileManager) StartUpload(ctx context.Context, sshSessionID model.ID, localPaths []string, remoteDirectory, currentDirectory string) (model.ID, error) {
	if !sshSessionID.Valid() {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
	}
	if len(localPaths) == 0 || len(localPaths) > maximumTransferBatchFiles {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "单次上传必须选择 1 到 100 个本地文件")
	}
	transferID, err := model.NewID(m.clock.Now())
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, "生成文件传输目标 ID 失败", err)
	}
	selected := append([]string(nil), localPaths...)
	return m.runner.Start(ctx, OperationRequest{
		Type: enums.OperationUpload, TargetType: enums.OperationTargetFile, TargetID: transferID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.uploadFiles(operationCtx, reporter, sshSessionID, selected, remoteDirectory, currentDirectory, transferID)
		},
	})
}

// StartDownload starts one durable Operation that streams a remote regular file into a local temporary file.
func (m *FileManager) StartDownload(ctx context.Context, sshSessionID model.ID, remotePath, currentDirectory, localDestination string) (model.ID, error) {
	if !sshSessionID.Valid() {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
	}
	if strings.TrimSpace(localDestination) == "" {
		return "", apperror.New(apperror.CodeValidationRequired, "本地下载目标不能为空")
	}
	transferID, err := model.NewID(m.clock.Now())
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, "生成文件传输目标 ID 失败", err)
	}
	return m.runner.Start(ctx, OperationRequest{
		Type: enums.OperationDownload, TargetType: enums.OperationTargetFile, TargetID: transferID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.downloadFile(operationCtx, reporter, sshSessionID, remotePath, currentDirectory, localDestination)
		},
	})
}

func (m *FileManager) uploadFiles(ctx context.Context, reporter OperationReporter, sshSessionID model.ID, localPaths []string, remoteDirectory, currentDirectory string, transferID model.ID) error {
	if err := reporter.SetProgress("prepare", 0.01, "正在检查本地上传文件"); err != nil {
		return err
	}
	sources, totalBytes, failures := inspectUploadSources(localPaths)
	if len(sources) == 0 {
		return transferBatchError("没有可上传的普通文件", 0, failures)
	}
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()
	destinationDirectory, err := model.NormalizeRemotePath(remoteDirectory, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "test", Arguments: []string{"-d", destinationDirectory}, Timeout: 10 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "远程上传目标不是可用目录", err)
	}

	var completedBytes int64
	var succeeded int
	for index, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := path.Join(destinationDirectory, source.name)
		message := fmt.Sprintf("正在上传 %s（%d/%d）", source.name, index+1, len(sources))
		progress := newTransferProgress(reporter, "upload", message, totalBytes, completedBytes, 0.05, 0.82)
		uploadErr := uploadLocalFile(ctx, client, source, target, transferID, index, progress)
		if uploadErr != nil {
			failures = append(failures, transferFailure(source.path, uploadErr))
			continue
		}
		completedBytes += source.size
		succeeded++
		if err := progress.report(source.size, true); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		return transferBatchError("部分文件上传失败，可重新选择失败文件重试", succeeded, failures)
	}
	return reporter.SetProgress("publish", 0.98, fmt.Sprintf("已上传 %d 个文件，远程文件已原子发布", succeeded))
}

func (m *FileManager) downloadFile(ctx context.Context, reporter OperationReporter, sshSessionID model.ID, requestedPath, currentDirectory, localDestination string) error {
	if err := reporter.SetProgress("prepare", 0.01, "正在检查远程文件和本地目标"); err != nil {
		return err
	}
	client, home, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()
	remotePath, err := model.NormalizeRemotePath(requestedPath, defaultRemoteDirectory(currentDirectory, home), home)
	if err != nil {
		return err
	}
	sizeResult, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test -f "$1" && test ! -L "$1" && stat --format=%s -- "$1"`, "mineops-file-download", remotePath},
		Timeout: 10 * time.Second, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "仅支持下载远程普通文件，拒绝目录和符号链接", err)
	}
	var remoteSize int64
	if _, err := fmt.Sscan(strings.TrimSpace(sizeResult.Stdout), &remoteSize); err != nil || remoteSize < 0 {
		return apperror.New(apperror.CodeSFTPTransferFailed, "远程文件大小响应无效")
	}
	destination, err := validateLocalDownloadDestination(localDestination)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".mineops-download-*")
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建本地下载临时文件失败", err)
	}
	temporaryPath := temporary.Name()
	completed := false
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !completed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return apperror.Wrap(apperror.CodeIOPermissionDenied, "设置本地下载临时文件权限失败", err)
	}
	hash := sha256.New()
	progress := newTransferProgress(reporter, "download", "正在下载 "+path.Base(remotePath), remoteSize, 0, 0.05, 0.82)
	writer := &progressWriter{ctx: ctx, writer: io.MultiWriter(temporary, hash), progress: progress}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "cat", Arguments: []string{"--", remotePath}, OutputWriter: writer, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPTransferFailed, "远程文件下载流中断", err).WithRetryable(true)
	}
	if writer.written != remoteSize {
		return apperror.New(apperror.CodeSFTPTransferFailed, "下载字节数与远程文件大小不一致").WithDetails(map[string]any{
			"expectedBytes": remoteSize, "actualBytes": writer.written,
		}).WithRetryable(true)
	}
	if err := reporter.SetProgress("verify", 0.9, "正在校验远程与本地 SHA-256"); err != nil {
		return err
	}
	remoteHash, err := remoteSHA256(ctx, client, remotePath)
	if err != nil {
		return err
	}
	localHash := hex.EncodeToString(hash.Sum(nil))
	if remoteHash != localHash {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "远程文件在下载期间发生变化或传输校验失败").WithDetails(map[string]any{
			"remoteSHA256": remoteHash, "localSHA256": localHash,
		}).WithRetryable(true)
	}
	if err := temporary.Sync(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "同步本地下载临时文件失败", err)
	}
	if err := temporary.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭本地下载临时文件失败", err)
	}
	closed = true
	if err := replaceLocalFile(temporaryPath, destination); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "原子发布本地下载文件失败", err)
	}
	completed = true
	return reporter.SetProgress("publish", 0.98, "下载完成并已原子替换本地目标")
}

func inspectUploadSources(localPaths []string) ([]uploadSource, int64, []map[string]any) {
	sources := make([]uploadSource, 0, len(localPaths))
	failures := make([]map[string]any, 0)
	names := make(map[string]string, len(localPaths))
	var total int64
	for _, selectedPath := range localPaths {
		absolute, err := filepath.Abs(filepath.Clean(selectedPath))
		if err != nil {
			failures = append(failures, transferFailure(selectedPath, err))
			continue
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			failures = append(failures, transferFailure(absolute, err))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			failures = append(failures, transferFailure(absolute, errors.New("仅支持上传本地普通文件，拒绝目录和符号链接")))
			continue
		}
		name := filepath.Base(absolute)
		if previous, exists := names[name]; exists {
			failures = append(failures, transferFailure(absolute, fmt.Errorf("与 %s 的文件名重复", previous)))
			continue
		}
		names[name] = absolute
		sources = append(sources, uploadSource{path: absolute, name: name, size: info.Size()})
		total += info.Size()
	}
	return sources, total, failures
}

func uploadLocalFile(ctx context.Context, client *SSHClient, source uploadSource, target string, transferID model.ID, index int, progress *transferProgress) error {
	file, err := os.Open(source.path)
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "打开本地上传文件失败", err)
	}
	defer func() {
		_ = file.Close()
	}()
	openedInfo, err := file.Stat()
	if err != nil {
		return apperror.Wrap(apperror.CodeIOReadFailed, "读取本地上传文件状态失败", err)
	}
	selectedInfo, err := os.Lstat(source.path)
	if err != nil || selectedInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(selectedInfo, openedInfo) || openedInfo.Size() != source.size {
		return apperror.New(apperror.CodeValidationConflict, "本地上传文件在选择后发生变化")
	}
	temporary := target + ".mineops-upload-" + transferID.String() + fmt.Sprintf("-%d", index)
	published := false
	defer func() {
		if !published {
			_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{
				Executable: "rm", Arguments: []string{"-f", "--", temporary}, Timeout: 10 * time.Second, MaximumOutput: 4096,
			})
		}
	}()
	hash := sha256.New()
	reader := &progressReader{ctx: ctx, reader: io.TeeReader(file, hash), progress: progress}
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", `test ! -d "$2" && umask 077 && cat > "$1" && chmod 0644 -- "$1" && sha256sum -- "$1"`, "mineops-file-upload", temporary, target},
		InputReader: reader, MaximumOutput: 4096,
	})
	if err != nil {
		return apperror.Wrap(apperror.CodeSFTPTransferFailed, "SSH stdin 上传流中断", err).WithRetryable(true)
	}
	finalInfo, statErr := file.Stat()
	if reader.read != source.size || statErr != nil || finalInfo.Size() != openedInfo.Size() || !finalInfo.ModTime().Equal(openedInfo.ModTime()) {
		return apperror.New(apperror.CodeValidationConflict, "本地上传文件在传输期间发生变化").WithRetryable(true)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) == 0 {
		return apperror.New(apperror.CodeSFTPTransferFailed, "远程上传 SHA-256 响应为空")
	}
	localHash := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(fields[0], localHash) {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "上传文件 SHA-256 校验失败").WithDetails(map[string]any{
			"remoteSHA256": fields[0], "localSHA256": localHash,
		}).WithRetryable(true)
	}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "mv", Arguments: []string{"-f", "--", temporary, target}, Timeout: 15 * time.Second, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "原子发布远程上传文件失败", err)
	}
	published = true
	return nil
}

func validateLocalDownloadDestination(value string) (string, error) {
	destination, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "本地下载目标路径无效", err)
	}
	parent, err := os.Stat(filepath.Dir(destination))
	if err != nil || !parent.IsDir() {
		return "", apperror.Wrap(apperror.CodeIOWriteFailed, "本地下载目标目录不可用", err)
	}
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", apperror.New(apperror.CodeValidationConflict, "本地下载目标已存在且不是普通文件")
		}
	} else if !os.IsNotExist(err) {
		return "", apperror.Wrap(apperror.CodeIOWriteFailed, "检查本地下载目标失败", err)
	}
	return destination, nil
}

func remoteSHA256(ctx context.Context, client *SSHClient, remotePath string) (string, error) {
	result, err := client.RunCommand(ctx, RemoteCommand{Executable: "sha256sum", Arguments: []string{"--", remotePath}, MaximumOutput: 4096})
	if err != nil {
		return "", apperror.Wrap(apperror.CodeSFTPTransferFailed, "计算远程文件 SHA-256 失败", err)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", apperror.New(apperror.CodeSFTPTransferFailed, "远程文件 SHA-256 响应无效")
	}
	return strings.ToLower(fields[0]), nil
}

func transferFailure(filePath string, err error) map[string]any {
	dto := apperror.ToDTO(err)
	return map[string]any{"path": filePath, "code": dto.Code, "message": dto.Message, "retryable": dto.Retryable}
}

func transferBatchError(message string, succeeded int, failures []map[string]any) error {
	return apperror.New(apperror.CodeSFTPTransferFailed, message).WithDetails(map[string]any{
		"succeeded": succeeded, "failed": len(failures), "failures": failures,
	}).WithRetryable(true)
}

type transferProgress struct {
	reporter    OperationReporter
	stage       string
	message     string
	total       int64
	base        int64
	start       float64
	span        float64
	lastBytes   int64
	lastUpdated time.Time
}

func newTransferProgress(reporter OperationReporter, stage, message string, total, base int64, start, span float64) *transferProgress {
	return &transferProgress{reporter: reporter, stage: stage, message: message, total: total, base: base, start: start, span: span}
}

func (p *transferProgress) report(current int64, force bool) error {
	now := time.Now()
	if !force && current-p.lastBytes < 1024*1024 && !p.lastUpdated.IsZero() && now.Sub(p.lastUpdated) < 150*time.Millisecond {
		return nil
	}
	value := p.start
	if p.total > 0 {
		value += p.span * float64(p.base+current) / float64(p.total)
	}
	if value > p.start+p.span {
		value = p.start + p.span
	}
	p.lastBytes = current
	p.lastUpdated = now
	return p.reporter.SetProgress(p.stage, value, p.message)
}

type progressReader struct {
	ctx      context.Context
	reader   io.Reader
	progress *transferProgress
	read     int64
}

// Read 读取数据并按已读字节数回报传输进度。
func (r *progressReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	read, err := r.reader.Read(buffer)
	if read > 0 {
		r.read += int64(read)
		if progressErr := r.progress.report(r.read, false); progressErr != nil {
			return read, progressErr
		}
	}
	return read, err
}

type progressWriter struct {
	ctx      context.Context
	writer   io.Writer
	progress *transferProgress
	written  int64
}

// Write 写入数据并按已写字节数回报传输进度。
func (w *progressWriter) Write(buffer []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	written, err := w.writer.Write(buffer)
	if written > 0 {
		w.written += int64(written)
		if progressErr := w.progress.report(w.written, false); progressErr != nil {
			return written, progressErr
		}
	}
	return written, err
}
