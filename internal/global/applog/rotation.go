package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RotatingWriterOptions 控制按日期与体积轮转以及有界保留。
type RotatingWriterOptions struct {
	Directory          string
	Prefix             string
	MaxFileBytes       int64
	RetentionDays      int
	TotalCapacityBytes int64
	Clock              func() time.Time
}

// RotatingWriter 写入一路带日期与体积轮转的 JSON 日志流。
type RotatingWriter struct {
	mu      sync.Mutex
	options RotatingWriterOptions
	file    *os.File
	path    string
	date    string
	size    int64
}

// RotatingWriterStatus 描述当前文件与有界的日志目录占用。
type RotatingWriterStatus struct {
	Directory   string `json:"directory"`
	CurrentPath string `json:"currentPath"`
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
}

// NewRotatingWriter 归一化策略默认值后打开一个有界日志写入器。
func NewRotatingWriter(options RotatingWriterOptions) (*RotatingWriter, error) {
	if options.Directory == "" || options.Prefix == "" {
		return nil, fmt.Errorf("log directory and prefix are required")
	}
	if options.MaxFileBytes <= 0 {
		options.MaxFileBytes = 20 * 1024 * 1024
	}
	if options.RetentionDays <= 0 {
		options.RetentionDays = 14
	}
	if options.TotalCapacityBytes < options.MaxFileBytes {
		options.TotalCapacityBytes = 500 * 1024 * 1024
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	writer := &RotatingWriter{options: options}
	if err := writer.open(options.Clock()); err != nil {
		return nil, err
	}
	if err := writer.cleanup(options.Clock()); err != nil {
		_ = writer.file.Close()
		return nil, err
	}
	return writer, nil
}

// Write 追加一条日志记录;日期或体积达到上限时先轮转再写入。
func (w *RotatingWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.options.Clock()
	if w.file == nil || w.date != logDate(now) || w.size+int64(len(payload)) > w.options.MaxFileBytes {
		if err := w.rotate(now); err != nil {
			return 0, err
		}
	}
	written, err := w.file.Write(payload)
	w.size += int64(written)
	return written, err
}

// Close 刷盘并关闭当前日志文件。
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// Cleanup 立即应用保留天数与总容量上限。
func (w *RotatingWriter) Cleanup() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cleanup(w.options.Clock())
}

// UpdatePolicy 应用已提交的轮转与保留设置,不替换当前写入器。
func (w *RotatingWriter) UpdatePolicy(maxFileBytes int64, retentionDays int, totalCapacityBytes int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if maxFileBytes <= 0 || retentionDays <= 0 || totalCapacityBytes < maxFileBytes {
		return fmt.Errorf("invalid runtime log policy")
	}
	w.options.MaxFileBytes = maxFileBytes
	w.options.RetentionDays = retentionDays
	w.options.TotalCapacityBytes = totalCapacityBytes
	return w.cleanup(w.options.Clock())
}

// ClearArchived 删除全部已轮转的 MineOps 日志,保留当前文件。
func (w *RotatingWriter) ClearArchived() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := os.ReadDir(w.options.Directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(w.options.Directory, entry.Name())
		if entry.IsDir() || path == w.path || !strings.HasPrefix(entry.Name(), w.options.Prefix+"-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

// Status 返回当前路径与 MineOps 日志目录的总体占用。
func (w *RotatingWriter) Status() (RotatingWriterStatus, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := os.ReadDir(w.options.Directory)
	if err != nil {
		return RotatingWriterStatus{}, err
	}
	status := RotatingWriterStatus{Directory: w.options.Directory, CurrentPath: w.path}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), w.options.Prefix+"-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return RotatingWriterStatus{}, err
		}
		status.Files++
		status.Bytes += info.Size()
	}
	return status, nil
}

func (w *RotatingWriter) rotate(now time.Time) error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
		if w.date == logDate(now) && w.size > 0 {
			for index := 1; ; index++ {
				rotatedPath := filepath.Join(w.options.Directory, fmt.Sprintf("%s-%s.%03d.log", w.options.Prefix, w.date, index))
				if _, err := os.Stat(rotatedPath); os.IsNotExist(err) {
					if err := os.Rename(w.path, rotatedPath); err != nil {
						return err
					}
					break
				}
			}
		}
	}
	if err := w.open(now); err != nil {
		return err
	}
	return w.cleanup(now)
}

func (w *RotatingWriter) open(now time.Time) error {
	w.date = logDate(now)
	w.path = filepath.Join(w.options.Directory, fmt.Sprintf("%s-%s.log", w.options.Prefix, w.date))
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *RotatingWriter) cleanup(now time.Time) error {
	entries, err := os.ReadDir(w.options.Directory)
	if err != nil {
		return err
	}
	type logFile struct {
		path    string
		modTime time.Time
		size    int64
	}
	files := make([]logFile, 0, len(entries))
	cutoff := now.AddDate(0, 0, -w.options.RetentionDays)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), w.options.Prefix+"-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		path := filepath.Join(w.options.Directory, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if path != w.path && info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		files = append(files, logFile{path: path, modTime: info.ModTime(), size: info.Size()})
	}
	sort.Slice(files, func(left, right int) bool { return files[left].modTime.Before(files[right].modTime) })
	var total int64
	for _, file := range files {
		total += file.size
	}
	for _, file := range files {
		if total <= w.options.TotalCapacityBytes {
			break
		}
		if file.path == w.path {
			continue
		}
		if err := os.Remove(file.path); err != nil {
			return err
		}
		total -= file.size
	}
	return nil
}
