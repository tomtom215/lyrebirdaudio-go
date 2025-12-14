package stream

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxLogSize is the default maximum log file size before rotation.
	DefaultMaxLogSize = 10 * 1024 * 1024 // 10 MB

	// DefaultMaxLogFiles is the default number of rotated log files to keep.
	DefaultMaxLogFiles = 5

	// LogRotateSuffix is the suffix for rotated log files.
	LogRotateSuffix = ".log"
)

// RotatingWriter is an io.Writer that rotates log files when they exceed a size limit.
//
// Features:
//   - Automatic rotation when file exceeds maxSize
//   - Retention of up to maxFiles rotated logs
//   - Optional gzip compression of rotated logs
//   - Thread-safe writes
//
// Reference: mediamtx-stream-manager.sh log rotation
type RotatingWriter struct {
	path     string
	maxSize  int64
	maxFiles int
	compress bool

	mu       sync.Mutex
	file     *os.File
	size     int64
	rotation int // Current rotation number
}

// RotatingWriterOption is a functional option for configuring RotatingWriter.
type RotatingWriterOption func(*RotatingWriter)

// WithMaxSize sets the maximum log file size before rotation.
func WithMaxSize(size int64) RotatingWriterOption {
	return func(w *RotatingWriter) {
		w.maxSize = size
	}
}

// WithMaxFiles sets the maximum number of rotated files to keep.
func WithMaxFiles(count int) RotatingWriterOption {
	return func(w *RotatingWriter) {
		w.maxFiles = count
	}
}

// WithCompression enables gzip compression for rotated logs.
func WithCompression(compress bool) RotatingWriterOption {
	return func(w *RotatingWriter) {
		w.compress = compress
	}
}

// NewRotatingWriter creates a new rotating log writer.
//
// Parameters:
//   - path: Path to the log file
//   - opts: Configuration options
//
// Returns:
//   - *RotatingWriter: Configured writer
//   - error: if log file cannot be created
//
// Example:
//
//	w, err := NewRotatingWriter("/var/log/ffmpeg.log",
//	    WithMaxSize(10*1024*1024),
//	    WithMaxFiles(5),
//	    WithCompression(true))
func NewRotatingWriter(path string, opts ...RotatingWriterOption) (*RotatingWriter, error) {
	w := &RotatingWriter{
		path:     path,
		maxSize:  DefaultMaxLogSize,
		maxFiles: DefaultMaxLogFiles,
		compress: false,
	}

	for _, opt := range opts {
		opt(w)
	}

	// Create parent directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open or create log file
	if err := w.openFile(); err != nil {
		return nil, err
	}

	return w, nil
}

// Write implements io.Writer.
//
// If the write would exceed maxSize, the log is rotated first.
func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if rotation needed
	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			// Log rotation failed, but try to write anyway
			// (better to potentially exceed size than lose logs)
		}
	}

	n, err = w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Close closes the log file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// Rotate forces a log rotation.
func (w *RotatingWriter) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotate()
}

// rotate performs the actual rotation (must hold lock).
func (w *RotatingWriter) rotate() error {
	// Close current file
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("failed to close log file: %w", err)
		}
		w.file = nil
	}

	// Rename existing rotated files (shift numbers up)
	if err := w.shiftFiles(); err != nil {
		return err
	}

	// Rename current log to .1
	rotatedPath := w.rotatedPath(1)
	if err := os.Rename(w.path, rotatedPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to rotate log file: %w", err)
	}

	// Compress if enabled
	if w.compress {
		go w.compressFile(rotatedPath) // Async compression
	}

	// Clean up old files
	w.cleanup()

	// Open new log file
	return w.openFile()
}

// openFile opens the log file for writing.
func (w *RotatingWriter) openFile() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Get current size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	w.file = file
	w.size = info.Size()
	return nil
}

// shiftFiles renames existing rotated files (2->3, 1->2, etc.).
func (w *RotatingWriter) shiftFiles() error {
	// Start from highest number and work down
	for i := w.maxFiles - 1; i >= 1; i-- {
		oldPath := w.rotatedPath(i)
		newPath := w.rotatedPath(i + 1)

		// Check for both compressed and uncompressed
		for _, ext := range []string{"", ".gz"} {
			old := oldPath + ext
			new := newPath + ext
			if _, err := os.Stat(old); err == nil {
				if err := os.Rename(old, new); err != nil {
					return fmt.Errorf("failed to shift log file %s -> %s: %w", old, new, err)
				}
			}
		}
	}
	return nil
}

// rotatedPath returns the path for a rotated log file.
func (w *RotatingWriter) rotatedPath(n int) string {
	return fmt.Sprintf("%s.%d", w.path, n)
}

// compressFile compresses a log file with gzip.
func (w *RotatingWriter) compressFile(path string) {
	// Read original file
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Create compressed file
	gzPath := path + ".gz"
	gzFile, err := os.Create(gzPath)
	if err != nil {
		return
	}
	defer gzFile.Close()

	// Write compressed data
	gzWriter := gzip.NewWriter(gzFile)
	if _, err := gzWriter.Write(data); err != nil {
		os.Remove(gzPath)
		return
	}
	if err := gzWriter.Close(); err != nil {
		os.Remove(gzPath)
		return
	}

	// Remove original
	os.Remove(path)
}

// cleanup removes log files beyond maxFiles.
func (w *RotatingWriter) cleanup() {
	for i := w.maxFiles + 1; i <= w.maxFiles+10; i++ {
		path := w.rotatedPath(i)
		os.Remove(path)
		os.Remove(path + ".gz")
	}
}

// Size returns the current log file size.
func (w *RotatingWriter) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

// Path returns the log file path.
func (w *RotatingWriter) Path() string {
	return w.path
}

// ListRotatedFiles returns all rotated log files for a path.
func ListRotatedFiles(basePath string) ([]RotatedFile, error) {
	dir := filepath.Dir(basePath)
	base := filepath.Base(basePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []RotatedFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Match base name with rotation number
		if !strings.HasPrefix(name, base+".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, RotatedFile{
			Path:       filepath.Join(dir, name),
			Name:       name,
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			Compressed: strings.HasSuffix(name, ".gz"),
		})
	}

	// Sort by modification time, newest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	return files, nil
}

// RotatedFile contains information about a rotated log file.
type RotatedFile struct {
	Path       string
	Name       string
	Size       int64
	ModTime    time.Time
	Compressed bool
}

// TotalLogSize returns the total size of all log files for a base path.
func TotalLogSize(basePath string) (int64, error) {
	var total int64

	// Include main log file
	if info, err := os.Stat(basePath); err == nil {
		total += info.Size()
	}

	// Include rotated files
	files, err := ListRotatedFiles(basePath)
	if err != nil {
		return total, err
	}

	for _, f := range files {
		total += f.Size
	}

	return total, nil
}

// CleanupLogs removes all log files for a base path.
func CleanupLogs(basePath string) error {
	// Remove main log
	os.Remove(basePath)

	// Remove rotated files
	files, err := ListRotatedFiles(basePath)
	if err != nil {
		return err
	}

	for _, f := range files {
		os.Remove(f.Path)
	}

	return nil
}

// LogWriter creates a log writer for a stream manager.
//
// Parameters:
//   - logDir: Directory for log files
//   - streamName: Name of the stream (used in filename)
//   - opts: RotatingWriter options
//
// Returns:
//   - io.WriteCloser: Writer for FFmpeg stderr
//   - error: if writer creation fails
func LogWriter(logDir, streamName string, opts ...RotatingWriterOption) (io.WriteCloser, error) {
	// Sanitize stream name for filename
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, streamName)

	path := filepath.Join(logDir, fmt.Sprintf("ffmpeg-%s.log", safeName))

	return NewRotatingWriter(path, opts...)
}
