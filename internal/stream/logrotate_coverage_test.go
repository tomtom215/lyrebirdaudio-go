// SPDX-License-Identifier: MIT

package stream

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestNewRotatingWriterOpenFileError covers the openFile() error path in
// NewRotatingWriter by placing a directory at the log file path so that
// os.OpenFile (which expects a regular file) returns an error.
func TestNewRotatingWriterOpenFileError(t *testing.T) {
	dir := t.TempDir()
	// Create a directory at the exact path that NewRotatingWriter will try to
	// open as a regular file. Even as root, opening a directory with O_WRONLY fails.
	logPath := filepath.Join(dir, "app.log")
	if err := os.Mkdir(logPath, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := NewRotatingWriter(logPath)
	if err == nil {
		t.Error("expected error when log path is a directory, got nil")
	}
}

// TestCompressFileCreateErrorRootSafe covers the os.Create error path in
// compressFile. We place a directory at the expected .gz path so that
// os.Create fails even when running as root.
func TestCompressFileCreateErrorRootSafe(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log.1")
	if err := os.WriteFile(filePath, []byte("log data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create a DIRECTORY at filePath+".gz" so os.Create(gzPath) fails.
	// This works even as root — you cannot O_TRUNC a directory.
	gzPath := filePath + ".gz"
	if err := os.Mkdir(gzPath, 0750); err != nil {
		t.Fatalf("mkdir gz path: %v", err)
	}

	w := &RotatingWriter{
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	// Should not panic; logger should warn about create failure.
	w.compressFile(filePath)
}

// TestCompressFileCreateErrorNilLoggerRootSafe covers the nil-logger branch
// of the os.Create error path in compressFile (no logger.Warn called).
func TestCompressFileCreateErrorNilLoggerRootSafe(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log.1")
	if err := os.WriteFile(filePath, []byte("log data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Directory at .gz path to force os.Create failure even as root.
	gzPath := filePath + ".gz"
	if err := os.Mkdir(gzPath, 0750); err != nil {
		t.Fatalf("mkdir gz path: %v", err)
	}

	w := &RotatingWriter{} // nil logger
	// Should not panic.
	w.compressFile(filePath)
}

// TestWriteFileNilAfterClose covers the w.file == nil branch in Write.
// We set w.file to nil and replace the log path with a directory so that
// the re-open attempt inside Write fails, returning an error.
func TestWriteFileNilAfterClose(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")

	w, err := NewRotatingWriter(logPath)
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	// Do NOT defer w.Close() — we're going to corrupt the state intentionally.

	// Close and nil the file under the lock.
	w.mu.Lock()
	_ = w.file.Close()
	w.file = nil
	w.mu.Unlock()

	// Replace the regular file with a directory so openFile fails.
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("Remove log file: %v", err)
	}
	if err := os.Mkdir(logPath, 0750); err != nil {
		t.Fatalf("Mkdir at log path: %v", err)
	}

	_, writeErr := w.Write([]byte("test"))
	if writeErr == nil {
		t.Error("expected error writing when file is nil and can't be re-opened")
	}
}

// TestListRotatedFilesNonExistentDir covers the os.ReadDir error path in
// ListRotatedFiles when the parent directory does not exist.
func TestListRotatedFilesNonExistentDir(t *testing.T) {
	_, err := ListRotatedFiles("/nonexistent-dir-xyz/app.log")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

// TestTotalLogSizeNonExistentDir covers the ListRotatedFiles error propagation
// in TotalLogSize when the parent directory does not exist.
func TestTotalLogSizeNonExistentDir(t *testing.T) {
	_, err := TotalLogSize("/nonexistent-dir-xyz/app.log")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

// TestCleanupLogsNonExistentDir covers the ListRotatedFiles error propagation
// in CleanupLogs when the parent directory does not exist.
func TestCleanupLogsNonExistentDir(t *testing.T) {
	err := CleanupLogs("/nonexistent-dir-xyz/app.log")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

// TestListRotatedFilesWithDirEntry covers the entry.IsDir() == true branch
// by creating a directory whose name matches the log rotation pattern.
func TestListRotatedFilesWithDirEntry(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")

	// Create a directory named "app.log.1" (matches the rotation pattern but is a dir).
	rotDirPath := logPath + ".1"
	if err := os.Mkdir(rotDirPath, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	// Also create a real rotated file so we confirm it IS picked up.
	if err := os.WriteFile(logPath+".2", []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files, err := ListRotatedFiles(logPath)
	if err != nil {
		t.Fatalf("ListRotatedFiles: %v", err)
	}
	// The directory entry should have been skipped; only the file entry returned.
	for _, f := range files {
		if f.Path == rotDirPath {
			t.Errorf("directory entry %q should have been skipped", rotDirPath)
		}
	}
}

// TestRotateErrorWithLogger covers the rotate() error + non-nil logger branch
// in Write. We force rotate() to fail by making shiftFiles fail (rename into
// a non-writable directory). We set maxSize to 1 byte so the first Write triggers
// rotation.
func TestRotateErrorWithLogger(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission restriction not meaningful as root")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")

	w, err := NewRotatingWriter(logPath,
		WithMaxSize(1), // tiny max so first write triggers rotation
		WithMaxFiles(3),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	defer func() { _ = w.Close() }()

	w.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Write one byte to fill the file to maxSize.
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatalf("initial Write: %v", err)
	}

	// Make the dir non-writable so rename (shiftFiles) fails.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0755) }()

	// This Write should trigger rotation which will fail; the logger should warn.
	// The write itself may or may not succeed depending on whether the partial
	// rotation opened a new file. We just verify no panic.
	_, _ = w.Write([]byte("y"))
}
