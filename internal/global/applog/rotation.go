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

// RotatingWriterOptions controls date/size rotation and bounded retention.
type RotatingWriterOptions struct {
	Directory          string
	Prefix             string
	MaxFileBytes       int64
	RetentionDays      int
	TotalCapacityBytes int64
	Clock              func() time.Time
}

// RotatingWriter writes one JSON log stream with date and size rotation.
type RotatingWriter struct {
	mu      sync.Mutex
	options RotatingWriterOptions
	file    *os.File
	path    string
	date    string
	size    int64
}

// RotatingWriterStatus describes the active file and bounded log directory usage.
type RotatingWriterStatus struct {
	Directory   string `json:"directory"`
	CurrentPath string `json:"currentPath"`
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
}

// NewRotatingWriter opens a bounded log writer after normalizing policy defaults.
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

// Write appends a log record, rotating before the write when date or size limits require it.
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

// Close flushes and closes the active log file.
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

// Cleanup applies retention-day and total-capacity limits immediately.
func (w *RotatingWriter) Cleanup() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cleanup(w.options.Clock())
}

// UpdatePolicy applies committed rotation and retention settings without replacing the active writer.
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

// ClearArchived removes every rotated MineOps log while preserving the active file.
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

// Status returns the active path and aggregate MineOps log directory usage.
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
