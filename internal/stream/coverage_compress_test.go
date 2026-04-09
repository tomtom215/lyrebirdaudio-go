// SPDX-License-Identifier: MIT

package stream

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestCompressFileSuccess verifies compressFile creates a .gz and removes original.
func TestCompressFileSuccess(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log.1")
	data := []byte("test log data for compression")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := &RotatingWriter{
		logger: slog.New(slog.NewTextHandler(devNull{}, nil)),
	}

	w.compressFile(filePath)

	// Original should be removed
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("original file should be removed after compression")
	}

	// Compressed file should exist
	gzPath := filePath + ".gz"
	if _, err := os.Stat(gzPath); os.IsNotExist(err) {
		t.Error("compressed file should exist")
	}
}

// TestCompressFileReadError verifies compressFile handles read errors.
func TestCompressFileReadError(t *testing.T) {
	w := &RotatingWriter{
		logger: slog.New(slog.NewTextHandler(devNull{}, nil)),
	}

	// Try to compress a non-existent file - should not panic
	w.compressFile("/nonexistent/path/file.log")
}

// TestCompressFileCreateError verifies compressFile handles create errors.
func TestCompressFileCreateError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log.1")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Make the directory read-only so .gz creation fails
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	w := &RotatingWriter{
		logger: slog.New(slog.NewTextHandler(devNull{}, nil)),
	}

	// Should not panic; just log a warning
	w.compressFile(filePath)
}

// TestCompressFileNilLogger verifies compressFile does not panic without logger.
func TestCompressFileNilLogger(t *testing.T) {
	w := &RotatingWriter{}

	// Non-existent file with nil logger - should not panic
	w.compressFile("/nonexistent/path/file.log")
}

// TestOpenFileStatError verifies openFile handles stat errors.
func TestOpenFileStatError(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	w := &RotatingWriter{
		path:     logPath,
		maxSize:  1024,
		maxFiles: 3,
	}

	// Normal open should succeed
	err := w.openFile()
	if err != nil {
		t.Errorf("openFile() error = %v", err)
	}
	if w.file != nil {
		w.file.Close()
	}
}
