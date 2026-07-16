package model

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// RemoteFileKind identifies the POSIX entry type shown by the SFTP browser.
type RemoteFileKind string

const (
	RemoteFileRegular   RemoteFileKind = "file"
	RemoteFileDirectory RemoteFileKind = "directory"
	RemoteFileSymlink   RemoteFileKind = "symlink"
	RemoteFileOther     RemoteFileKind = "other"
)

// RemoteFile is the infrastructure-neutral DTO for one remote POSIX entry.
type RemoteFile struct {
	Path       string         `json:"path"`
	Name       string         `json:"name"`
	Kind       RemoteFileKind `json:"kind"`
	Size       int64          `json:"size"`
	Mode       uint32         `json:"mode"`
	ModifiedAt time.Time      `json:"modifiedAt"`
	LinkTarget string         `json:"linkTarget,omitempty"`
}

// RemoteTextDocument contains bounded UTF-8 content and a conflict-detection version token.
type RemoteTextDocument struct {
	Path         string    `json:"path"`
	Content      string    `json:"content"`
	Encoding     string    `json:"encoding"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modifiedAt"`
	VersionToken string    `json:"versionToken"`
}

// NormalizeRemotePath resolves Home aliases and relative paths into a clean POSIX absolute path.
func NormalizeRemotePath(value, currentDirectory, homeDirectory string) (string, error) {
	if strings.ContainsRune(value, '\x00') || strings.ContainsRune(currentDirectory, '\x00') || strings.ContainsRune(homeDirectory, '\x00') {
		return "", apperror.New(apperror.CodeSFTPPathRejected, "远程路径包含 NUL 字符")
	}
	value = strings.TrimSpace(value)
	currentDirectory = path.Clean(strings.TrimSpace(currentDirectory))
	homeDirectory = path.Clean(strings.TrimSpace(homeDirectory))
	if value == "" {
		value = currentDirectory
	}
	if value == "~" {
		value = homeDirectory
	} else if strings.HasPrefix(value, "~/") {
		value = path.Join(homeDirectory, strings.TrimPrefix(value, "~/"))
	} else if !strings.HasPrefix(value, "/") {
		value = path.Join(currentDirectory, value)
	}
	normalized := path.Clean(value)
	if !strings.HasPrefix(normalized, "/") || normalized == "." || normalized == "" {
		return "", apperror.New(apperror.CodeSFTPPathRejected, "远程路径必须解析为 POSIX 绝对路径")
	}
	return normalized, nil
}

// ValidateRemoteDelete rejects root, Home, and Home ancestors before destructive operations.
func ValidateRemoteDelete(target, homeDirectory string, recursive bool) error {
	target = path.Clean(target)
	homeDirectory = path.Clean(homeDirectory)
	if target == "/" || target == "." || target == "" || target == homeDirectory || isPathAncestor(target, homeDirectory) {
		return apperror.New(apperror.CodeSFTPPathRejected, "拒绝删除远程根目录、Home 或 Home 上级目录")
	}
	if recursive && path.Dir(target) == "/" {
		return apperror.New(apperror.CodeSFTPPathRejected, "拒绝递归删除远程根目录下的一级路径")
	}
	return nil
}

// NewRemoteTextDocument validates bounded UTF-8 content and creates a stable conflict token.
func NewRemoteTextDocument(remotePath string, content []byte, modifiedAt time.Time, maximumBytes int64) (*RemoteTextDocument, error) {
	if maximumBytes <= 0 || int64(len(content)) > maximumBytes {
		return nil, apperror.New(apperror.CodeSFTPTransferFailed, "远程文本超过允许的读取大小").WithDetails(map[string]any{
			"size": len(content), "maximumBytes": maximumBytes,
		})
	}
	if !utf8.Valid(content) || strings.IndexByte(string(content), 0) >= 0 {
		return nil, apperror.New(apperror.CodeSFTPTransferFailed, "远程文件不是受支持的 UTF-8 文本")
	}
	normalized, err := NormalizeRemotePath(remotePath, "/", "/")
	if err != nil {
		return nil, err
	}
	version := RemoteTextVersionToken(content, modifiedAt)
	return &RemoteTextDocument{
		Path: normalized, Content: string(content), Encoding: "utf-8", Size: int64(len(content)),
		ModifiedAt: modifiedAt.UTC(), VersionToken: version,
	}, nil
}

// RemoteTextVersionToken hashes content metadata for conservative save conflict detection.
func RemoteTextVersionToken(content []byte, modifiedAt time.Time) string {
	hash := sha256.New()
	_, _ = hash.Write(content)
	_, _ = hash.Write([]byte(modifiedAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(hash.Sum(nil))
}

// NormalizeArchiveEntry rejects absolute paths, parent traversal, and entries escaping a destination root.
func NormalizeArchiveEntry(destinationRoot, entryName string) (string, error) {
	if strings.ContainsRune(entryName, '\x00') || strings.HasPrefix(entryName, "/") || strings.HasPrefix(entryName, "\\") {
		return "", apperror.New(apperror.CodeSFTPPathRejected, "压缩包条目路径无效")
	}
	cleaned := path.Clean(strings.ReplaceAll(entryName, "\\", "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", apperror.New(apperror.CodeSFTPPathRejected, "压缩包条目包含路径穿越")
	}
	destinationRoot = path.Clean(destinationRoot)
	target := path.Join(destinationRoot, cleaned)
	if target != destinationRoot && !strings.HasPrefix(target, strings.TrimSuffix(destinationRoot, "/")+"/") {
		return "", apperror.New(apperror.CodeSFTPPathRejected, "压缩包条目越过目标目录")
	}
	return target, nil
}

// ValidateArchiveLimits enforces file-count, per-file, and total uncompressed size bounds.
func ValidateArchiveLimits(fileCount int, singleBytes, totalBytes int64, maximumFiles int, maximumSingleBytes, maximumTotalBytes int64) error {
	if fileCount < 0 || fileCount > maximumFiles || singleBytes < 0 || singleBytes > maximumSingleBytes || totalBytes < 0 || totalBytes > maximumTotalBytes {
		return apperror.New(apperror.CodeSFTPTransferFailed, "压缩包超过安全解压限制").WithDetails(map[string]any{
			"fileCount": fileCount, "singleBytes": singleBytes, "totalBytes": totalBytes,
		})
	}
	return nil
}

func isPathAncestor(candidate, target string) bool {
	if candidate == "/" {
		return true
	}
	return strings.HasPrefix(target, strings.TrimSuffix(candidate, "/")+"/")
}
