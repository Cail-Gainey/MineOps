package httpclient

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// DownloadRequest defines one checksum-enforced atomic artifact download.
type DownloadRequest struct {
	URL              string
	Destination      string
	ExpectedSHA256   string
	ExpectedSHA512   string
	ExpectedSize     int64
	MaximumSize      int64
	ProgressInterval time.Duration
}

// DownloadProgress reports durable byte-level progress without owning cancellation.
type DownloadProgress struct {
	Downloaded int64
	Total      int64
	Progress   float64
}

// DownloadResult contains the actual verified artifact evidence.
type DownloadResult struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	SHA512 string `json:"sha512"`
}

// Downloader streams artifacts through a temporary file, verifies SHA-256 or SHA-512, and atomically publishes the result.
type Downloader struct {
	client *Client
}

// NewDownloader creates a downloader backed by the shared HTTP client.
func NewDownloader(client *Client) *Downloader {
	return &Downloader{client: client}
}

// Download executes a cancellable checksum-enforced download and cleans every failed temporary file.
func (d *Downloader) Download(ctx context.Context, request DownloadRequest, onProgress func(DownloadProgress)) (result DownloadResult, err error) {
	sha256Checksum := strings.ToLower(strings.TrimSpace(request.ExpectedSHA256))
	sha512Checksum := strings.ToLower(strings.TrimSpace(request.ExpectedSHA512))
	if d == nil || d.client == nil || request.URL == "" || request.Destination == "" {
		return DownloadResult{}, apperror.New(apperror.CodeValidationRequired, "下载 URL 和目标路径不能为空")
	}
	if sha256Checksum != "" {
		if len(sha256Checksum) != sha256.Size*2 {
			return DownloadResult{}, apperror.New(apperror.CodeValidationInvalidArgument, "Artifact SHA-256 长度无效")
		}
		if _, decodeErr := hex.DecodeString(sha256Checksum); decodeErr != nil {
			return DownloadResult{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "Artifact SHA-256 无效", decodeErr)
		}
	}
	if sha512Checksum != "" {
		if len(sha512Checksum) != sha512.Size*2 {
			return DownloadResult{}, apperror.New(apperror.CodeValidationInvalidArgument, "Artifact SHA-512 长度无效")
		}
		if _, decodeErr := hex.DecodeString(sha512Checksum); decodeErr != nil {
			return DownloadResult{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "Artifact SHA-512 无效", decodeErr)
		}
	}
	if request.MaximumSize <= 0 {
		request.MaximumSize = 2 * 1024 * 1024 * 1024
	}
	if request.ProgressInterval <= 0 {
		request.ProgressInterval = 100 * time.Millisecond
	}
	if err := os.MkdirAll(filepath.Dir(request.Destination), 0o750); err != nil {
		return DownloadResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Artifact 目标目录失败", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(request.Destination), ".mineops-download-*")
	if err != nil {
		return DownloadResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Artifact 临时文件失败", err)
	}
	temporaryPath := temporary.Name()
	temporaryClosed := false
	defer func() {
		var closeError error
		if !temporaryClosed {
			closeError = temporary.Close()
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
		if err == nil && closeError != nil {
			err = apperror.Wrap(apperror.CodeIOWriteFailed, "关闭 Artifact 临时文件失败", closeError)
			_ = os.Remove(temporaryPath)
		}
	}()

	response, err := d.client.Do(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return DownloadResult{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.ContentLength > request.MaximumSize || request.ExpectedSize > request.MaximumSize {
		return DownloadResult{}, apperror.New(apperror.CodeArtifactSizeExceeded, "Artifact 超过大小限制").WithDetails(map[string]any{
			"contentLength": response.ContentLength, "maximumBytes": request.MaximumSize,
		})
	}

	sha256Hash := sha256.New()
	sha512Hash := sha512.New()
	buffer := make([]byte, 128*1024)
	var downloaded int64
	lastProgress := time.Time{}
	for {
		read, readErr := response.Body.Read(buffer)
		if read > 0 {
			downloaded += int64(read)
			if downloaded > request.MaximumSize {
				return DownloadResult{}, apperror.New(apperror.CodeArtifactSizeExceeded, "Artifact 下载超过大小限制")
			}
			if _, writeErr := temporary.Write(buffer[:read]); writeErr != nil {
				return DownloadResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "写入 Artifact 临时文件失败", writeErr)
			}
			_, _ = sha256Hash.Write(buffer[:read])
			_, _ = sha512Hash.Write(buffer[:read])
			now := time.Now()
			if onProgress != nil && (lastProgress.IsZero() || now.Sub(lastProgress) >= request.ProgressInterval) {
				total := response.ContentLength
				progress := float64(0)
				if total > 0 {
					progress = float64(downloaded) / float64(total)
				}
				onProgress(DownloadProgress{Downloaded: downloaded, Total: total, Progress: progress})
				lastProgress = now
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return DownloadResult{}, apperror.Wrap(apperror.CodeIOReadFailed, "读取 Artifact 下载流失败", readErr).WithRetryable(true)
		}
	}
	actualSHA256 := hex.EncodeToString(sha256Hash.Sum(nil))
	actualSHA512 := hex.EncodeToString(sha512Hash.Sum(nil))
	if request.ExpectedSize > 0 && downloaded != request.ExpectedSize {
		return DownloadResult{}, apperror.New(apperror.CodeArtifactSizeExceeded, "Artifact 实际大小与目录元数据不一致").WithDetails(map[string]any{
			"expectedBytes": request.ExpectedSize, "actualBytes": downloaded,
		})
	}
	if sha256Checksum != "" && actualSHA256 != sha256Checksum {
		return DownloadResult{}, apperror.New(apperror.CodeArtifactChecksumMismatch, "Artifact SHA-256 校验失败").WithDetails(map[string]any{
			"expected": sha256Checksum, "actual": actualSHA256,
		})
	}
	if sha512Checksum != "" && actualSHA512 != sha512Checksum {
		return DownloadResult{}, apperror.New(apperror.CodeArtifactChecksumMismatch, "Artifact SHA-512 校验失败").WithDetails(map[string]any{
			"expected": sha512Checksum, "actual": actualSHA512,
		})
	}
	if err := temporary.Sync(); err != nil {
		return DownloadResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "同步 Artifact 临时文件失败", err)
	}
	if err := temporary.Close(); err != nil {
		return DownloadResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "关闭 Artifact 临时文件失败", err)
	}
	temporaryClosed = true
	if err := os.Rename(temporaryPath, request.Destination); err != nil {
		return DownloadResult{}, apperror.Wrap(apperror.CodeIOWriteFailed, "原子发布 Artifact 失败", fmt.Errorf("%s: %w", request.Destination, err))
	}
	if onProgress != nil {
		onProgress(DownloadProgress{Downloaded: downloaded, Total: downloaded, Progress: 1})
	}
	return DownloadResult{Path: request.Destination, Size: downloaded, SHA256: actualSHA256, SHA512: actualSHA512}, nil
}
