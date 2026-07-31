package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
)

const (
	maximumJDKArchiveBytes  int64 = 2 * 1024 * 1024 * 1024
	maximumJDKExpandedBytes int64 = 8 * 1024 * 1024 * 1024
	maximumJDKEntryBytes    int64 = 2 * 1024 * 1024 * 1024
	maximumJDKEntries             = 100_000
)

// StartInstall 启动一次受管的远端 Eclipse Temurin 安装 Operation。
func (m *JavaRuntimeManager) StartInstall(ctx context.Context, sshSessionID model.ID, majorVersion int, architecture string) (model.ID, error) {
	if !sshSessionID.Valid() {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "SSH Session ID 无效")
	}
	architecture = normalizeJDKArchitecture(architecture)
	if architecture == "" || majorVersion != 8 && majorVersion != 11 && majorVersion != 17 && majorVersion != 21 && majorVersion != 24 && majorVersion != 25 {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "远程 JDK 安装版本或架构无效")
	}
	if _, err := m.store.SSHSessions().Get(ctx, sshSessionID); err != nil {
		return "", err
	}
	installID, err := model.NewID(m.clock.Now())
	if err != nil {
		return "", apperror.Wrap(apperror.CodeInternal, "生成 Java 安装 ID 失败", err)
	}
	return m.runner.Start(ctx, OperationRequest{
		Type: enums.OperationInstall, TargetType: enums.OperationTargetJava, TargetID: installID,
		Handler: func(operationCtx context.Context, reporter OperationReporter) error {
			return m.installManagedJDK(operationCtx, reporter, sshSessionID, majorVersion, architecture, installID)
		},
	})
}

func (m *JavaRuntimeManager) installManagedJDK(ctx context.Context, reporter OperationReporter, sshSessionID model.ID, majorVersion int, architecture string, installID model.ID) error {
	if err := reporter.SetProgress("resolve", 0.02, "正在解析 Eclipse Temurin Artifact"); err != nil {
		return err
	}
	artifact, err := m.catalog.Resolve(ctx, majorVersion, architecture, "linux")
	if err != nil {
		return err
	}
	if err := validateJDKArtifact(artifact, majorVersion, architecture); err != nil {
		return err
	}
	_, client, err := m.connect(ctx, sshSessionID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	homeResult, err := client.RunCommand(ctx, RemoteCommand{Executable: "printenv", Arguments: []string{"HOME"}, Timeout: 5 * time.Second, MaximumOutput: 4096})
	if err != nil || strings.TrimSpace(homeResult.Stdout) == "" {
		return apperror.Wrap(apperror.CodeSFTPPathRejected, "读取远程 Home 目录失败", err)
	}
	home, err := model.NormalizeRemotePath(strings.TrimSpace(homeResult.Stdout), "/", "/")
	if err != nil {
		return err
	}
	mineOpsRoot := path.Join(home, "MineOps")
	downloadDirectory := path.Join(mineOpsRoot, "Downloads")
	archivePath := path.Join(downloadDirectory, fmt.Sprintf("temurin-%d-%s.tar.gz", majorVersion, architecture))
	temporaryArchive := archivePath + ".part-" + installID.String()
	if err := reporter.SetProgress("download", 0.05, "正在远程下载 Eclipse Temurin"); err != nil {
		return err
	}
	downloadProgress := newTransferProgress(reporter, "download", "正在远程下载 Eclipse Temurin", artifact.Size, 0, 0.05, 0.42)
	script := `set -eu
test ! -L "$1"
if [ -e "$1" ]; then test -d "$1"; else mkdir --mode=0750 -- "$1"; fi
test ! -L "$2"
if [ -e "$2" ]; then test -d "$2"; else mkdir --mode=0750 -- "$2"; fi
rm -f -- "$4"
curl --fail --location --retry 2 --connect-timeout 15 --output "$4" "$5" &
download_pid=$!
while kill -0 "$download_pid" 2>/dev/null; do bytes=$(wc -c < "$4" 2>/dev/null | tr -d ' ' || printf '0'); printf 'MINEOPS_PROGRESS %s\n' "${bytes:-0}" >&2; sleep 1; done
wait "$download_pid"
size=$(stat --format=%s -- "$4")
test "$size" -le "$7"
test "$size" = "$8"
hash=$(sha256sum -- "$4" | awk '{print $1}')
test "$hash" = "$6"
mv -f -- "$4" "$3"
printf '%s\n%s\n' "$hash" "$size"`
	result, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", script, "mineops-java-install", mineOpsRoot, downloadDirectory, archivePath, temporaryArchive, artifact.URL, strings.ToLower(artifact.SHA256), strconv.FormatInt(maximumJDKArchiveBytes, 10), strconv.FormatInt(artifact.Size, 10)},
		Timeout: 30 * time.Minute, MaximumOutput: 128 * 1024,
		OnOutput: func(output RemoteCommandOutput) {
			for _, line := range strings.Split(output.Data, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "MINEOPS_PROGRESS ") {
					continue
				}
				bytes, parseErr := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "MINEOPS_PROGRESS ")), 10, 64)
				if parseErr == nil {
					_ = downloadProgress.report(bytes, false)
				}
			}
		},
	})
	if err != nil {
		_, _ = client.RunCommand(context.WithoutCancel(ctx), RemoteCommand{Executable: "rm", Arguments: []string{"-f", "--", temporaryArchive}, Timeout: 10 * time.Second, MaximumOutput: 4096})
		return apperror.Wrap(apperror.CodeInstallationJavaFailed, "远程下载或校验 Eclipse Temurin 失败", err).WithRetryable(true)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) < 2 || !strings.EqualFold(fields[0], artifact.SHA256) {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "远程 JDK 下载校验响应无效")
	}
	archiveSize, parseErr := strconv.ParseInt(fields[1], 10, 64)
	if parseErr != nil || archiveSize != artifact.Size {
		return apperror.New(apperror.CodeArtifactSizeExceeded, "远程 JDK 下载大小响应无效")
	}

	localArchive, err := os.CreateTemp("", "mineops-jdk-*.tar.gz")
	if err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 JDK 安全检查临时文件失败", err)
	}
	localPath := localArchive.Name()
	defer func() {
		_ = localArchive.Close()
		_ = os.Remove(localPath)
	}()
	hash := sha256.New()
	verifyProgress := newTransferProgress(reporter, "verify", "正在读取并安全检查 JDK 归档", archiveSize, 0, 0.49, 0.16)
	writer := &progressWriter{ctx: ctx, writer: io.MultiWriter(localArchive, hash), progress: verifyProgress}
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "cat", Arguments: []string{"--", archivePath}, OutputWriter: writer, MaximumOutput: 4096,
	}); err != nil {
		return apperror.Wrap(apperror.CodeSFTPTransferFailed, "读取远程 JDK 归档失败", err).WithRetryable(true)
	}
	if writer.written != archiveSize || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), artifact.SHA256) {
		return apperror.New(apperror.CodeArtifactChecksumMismatch, "JDK 归档本地安全检查前摘要不一致")
	}
	if err := localArchive.Sync(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "同步 JDK 安全检查临时文件失败", err)
	}
	if err := localArchive.Close(); err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "关闭 JDK 安全检查临时文件失败", err)
	}
	archiveRoot, expandedBytes, err := validateJDKTar(localPath)
	if err != nil {
		return err
	}
	if err := reporter.SetProgress("extract", 0.68, "JDK 归档安全检查通过，正在远程 staging 解压"); err != nil {
		return err
	}
	managedRoot := path.Join(mineOpsRoot, "Runtime")
	target := path.Join(managedRoot, fmt.Sprintf("java-%d-%s-%s", majorVersion, architecture, strings.ToLower(artifact.SHA256[:12])))
	staging := path.Join(managedRoot, ".mineops-jdk-staging-"+installID.String())
	extractScript := `set -eu
test -f "$1"
test ! -L "$1"
hash=$(sha256sum -- "$1" | awk '{print $1}')
test "$hash" = "$6"
test -d "$8"
test ! -L "$8"
test ! -L "$3"
if [ -e "$3" ]; then test -d "$3"; else mkdir --mode=0750 -- "$3"; fi
if [ -e "$4" ]; then test -d "$4" && test ! -L "$4" && test -x "$4/bin/java"; printf 'existing'; exit 0; fi
available=$(df -PB1 "$3" | awk 'NR == 2 { print $4 }')
test -n "$available"
test "$available" -ge "$7"
test ! -L "$4"
rm -rf -- "$5"
mkdir --mode=0700 -- "$5"
trap 'rm -rf -- "$5"' EXIT
tar --extract --gzip --file "$1" --directory "$5" --no-same-owner
test -d "$5/$2"
test ! -L "$5/$2"
test -x "$5/$2/bin/java"
mv -- "$5/$2" "$4"
rm -rf -- "$5"
trap - EXIT
printf 'installed'`
	if _, err := client.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", extractScript, "mineops-java-install", archivePath, archiveRoot, managedRoot, target, staging, strings.ToLower(artifact.SHA256), strconv.FormatInt(expandedBytes, 10), mineOpsRoot},
		Timeout: 10 * time.Minute, MaximumOutput: 16 * 1024,
	}); err != nil {
		return apperror.Wrap(apperror.CodeInstallationJavaFailed, "远程安全解压或发布 JDK 失败", err).WithRetryable(true)
	}
	candidate, err := m.validateWithClient(ctx, client, path.Join(target, "bin/java"), model.JavaSourceManaged)
	if err != nil {
		return err
	}
	javaRuntime, err := m.registerCandidate(ctx, sshSessionID, candidate)
	if err != nil {
		return err
	}
	return reporter.SetProgress("complete", 0.98, fmt.Sprintf("Java Runtime 已安装并注册：%s", javaRuntime.Version))
}

func validateJDKArtifact(artifact port.JDKArtifact, majorVersion int, architecture string) error {
	if artifact.MajorVersion != majorVersion || normalizeJDKArchitecture(artifact.Architecture) != architecture || strings.ToLower(artifact.OperatingSystem) != "linux" || artifact.ArchiveType != "tar.gz" {
		return apperror.New(apperror.CodeValidationConflict, "JDK Catalog Artifact 与请求版本、架构或 Linux tar.gz 策略不一致")
	}
	if artifact.Size <= 0 || artifact.Size > maximumJDKArchiveBytes || len(artifact.SHA256) != sha256.Size*2 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "JDK Artifact 大小或 SHA-256 无效")
	}
	if _, err := hex.DecodeString(artifact.SHA256); err != nil {
		return apperror.Wrap(apperror.CodeValidationInvalidArgument, "JDK Artifact SHA-256 无效", err)
	}
	artifactURL, err := url.Parse(artifact.URL)
	if err != nil || artifactURL.Scheme != "https" || artifactURL.Host == "" || artifactURL.User != nil {
		return apperror.New(apperror.CodeValidationInvalidArgument, "JDK Artifact 必须使用无凭据 HTTPS URL")
	}
	return nil
}

func validateJDKTar(localPath string) (string, int64, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", 0, apperror.Wrap(apperror.CodeIOReadFailed, "打开 JDK 归档失败", err)
	}
	defer func() {
		_ = file.Close()
	}()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", 0, apperror.Wrap(apperror.CodeSFTPTransferFailed, "JDK 归档不是有效 gzip", err)
	}
	defer func() {
		_ = gzipReader.Close()
	}()
	tarReader := tar.NewReader(gzipReader)
	seen := make(map[string]struct{})
	root := ""
	javaFound := false
	var expanded int64
	entries := 0
	for {
		header, readErr := tarReader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", 0, apperror.Wrap(apperror.CodeSFTPTransferFailed, "读取 JDK 归档条目失败", readErr)
		}
		cleaned := path.Clean(strings.ReplaceAll(header.Name, "\\", "/"))
		if _, err := model.NormalizeArchiveEntry("/jdk", cleaned); err != nil {
			return "", 0, err
		}
		parts := strings.Split(cleaned, "/")
		if len(parts) == 0 || parts[0] == "" || parts[0] == "." {
			return "", 0, apperror.New(apperror.CodeSFTPPathRejected, "JDK 归档根目录无效")
		}
		if root == "" {
			root = parts[0]
		} else if root != parts[0] {
			return "", 0, apperror.New(apperror.CodeSFTPPathRejected, "JDK 归档包含多个顶级目录")
		}
		if _, exists := seen[cleaned]; exists {
			return "", 0, apperror.New(apperror.CodeValidationConflict, "JDK 归档包含重复路径").WithDetails(map[string]any{"entry": header.Name})
		}
		seen[cleaned] = struct{}{}
		entries++
		if entries > maximumJDKEntries {
			return "", 0, apperror.New(apperror.CodeArtifactSizeExceeded, "JDK 归档条目数量超过限制")
		}
		if header.Mode&0o7000 != 0 {
			return "", 0, apperror.New(apperror.CodeSFTPPathRejected, "JDK 归档包含 setuid、setgid 或 sticky 权限").WithDetails(map[string]any{"entry": header.Name})
		}
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg, 0:
			if header.Size < 0 || header.Size > maximumJDKEntryBytes || expanded > maximumJDKExpandedBytes-header.Size {
				return "", 0, apperror.New(apperror.CodeArtifactSizeExceeded, "JDK 归档展开体积超过限制")
			}
			expanded += header.Size
			if cleaned == path.Join(root, "bin/java") {
				javaFound = true
			}
		case tar.TypeSymlink:
			if err := validateJDKLink(root, cleaned, header.Linkname, false); err != nil {
				return "", 0, err
			}
		case tar.TypeLink:
			if err := validateJDKLink(root, cleaned, header.Linkname, true); err != nil {
				return "", 0, err
			}
		default:
			return "", 0, apperror.New(apperror.CodeSFTPPathRejected, "JDK 归档包含设备、FIFO 或其他特殊文件").WithDetails(map[string]any{"entry": header.Name})
		}
	}
	if root == "" || !javaFound {
		return "", 0, apperror.New(apperror.CodeSFTPPathRejected, "JDK 归档缺少顶级目录或 bin/java")
	}
	return root, expanded, nil
}

func validateJDKLink(root, entry, linkName string, hardLink bool) error {
	if strings.ContainsRune(linkName, '\x00') || path.IsAbs(linkName) {
		return apperror.New(apperror.CodeSFTPPathRejected, "JDK 归档链接目标无效").WithDetails(map[string]any{"entry": entry})
	}
	target := path.Clean(strings.ReplaceAll(linkName, "\\", "/"))
	if !hardLink {
		target = path.Clean(path.Join(path.Dir(entry), target))
	}
	if target != root && !strings.HasPrefix(target, root+"/") {
		return apperror.New(apperror.CodeSFTPPathRejected, "JDK 归档链接逃逸顶级目录").WithDetails(map[string]any{"entry": entry, "target": linkName})
	}
	return nil
}

func normalizeJDKArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x64", "x86_64", "amd64":
		return "x64"
	case "aarch64", "arm64":
		return "aarch64"
	default:
		return ""
	}
}
