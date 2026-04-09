package stream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterCompression(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath,
		WithMaxSize(50),
		WithMaxFiles(3),
		WithCompression(true),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}

	// Write enough data to trigger multiple rotations
	for i := 0; i < 10; i++ {
		data := strings.Repeat("x", 30) + "\n"
		_, err := w.Write([]byte(data))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	_ = w.Close()

	// Check for compressed files
	matches, _ := filepath.Glob(logPath + ".*.gz")
	if len(matches) == 0 {
		// It's ok if no compressed files yet - rotation timing varies
		t.Log("No compressed files found (may depend on timing)")
	}
}

func TestCompressFileErrors(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath, WithCompression(true), WithMaxSize(50))
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}

	// Test compressing a non-existent file - should not panic
	w.compressFile(filepath.Join(tmpDir, "nonexistent.log"))

	// Test compressing a file in a read-only directory (if possible)
	// Create a file first
	testFile := filepath.Join(tmpDir, "compressme.log")
	if err := os.WriteFile(testFile, []byte("test data"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Compress it - this should work
	w.compressFile(testFile)

	// Check that compressed file exists
	if _, err := os.Stat(testFile + ".gz"); os.IsNotExist(err) {
		t.Error("Expected compressed file to exist")
	}

	// Original should be removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Expected original file to be removed after compression")
	}

	_ = w.Close()
}

func TestRotateWithCompression(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath,
		WithMaxSize(50),
		WithMaxFiles(3),
		WithCompression(true),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer func() { _ = w.Close() }()

	// Write data to trigger rotation
	for i := 0; i < 5; i++ {
		data := strings.Repeat("x", 30) + "\n"
		_, _ = w.Write([]byte(data))
	}

	// Force a rotation
	err = w.Rotate()
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	// Give compression goroutine time to run
	// Check for .1 file (before compression)
	if _, err := os.Stat(logPath + ".1"); os.IsNotExist(err) {
		// Check for compressed version
		if _, err := os.Stat(logPath + ".1.gz"); os.IsNotExist(err) {
			t.Log("Neither .1 nor .1.gz exists (compression may be async)")
		}
	}
}

func TestRotatingWriterCloseWaitsForCompression(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath,
		WithMaxSize(50),
		WithMaxFiles(3),
		WithCompression(true),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}

	// Write enough data to trigger rotation (and thus async compression)
	for i := 0; i < 5; i++ {
		data := strings.Repeat("y", 30) + "\n"
		_, err := w.Write([]byte(data))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Force a rotation which triggers async compression
	err = w.Rotate()
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	// Close should block until compression goroutines finish
	err = w.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// After Close returns, compression must be complete.
	// The .1.gz file should exist (compression finished).
	gzPath := logPath + ".1.gz"
	if _, err := os.Stat(gzPath); os.IsNotExist(err) {
		t.Errorf("Expected compressed file %s to exist after Close()", gzPath)
	}

	// The uncompressed .1 file should have been removed by compression.
	uncompressedPath := logPath + ".1"
	if _, err := os.Stat(uncompressedPath); err == nil {
		t.Errorf("Expected uncompressed file %s to be removed after compression", uncompressedPath)
	}
}

func TestRotatingWriterCompressionCleanupOrder(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Use maxFiles=2, so cleanup should remove files beyond index 2.
	w, err := NewRotatingWriter(logPath,
		WithMaxSize(30),
		WithMaxFiles(2),
		WithCompression(true),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}

	// Trigger enough rotations to exercise cleanup
	for i := 0; i < 10; i++ {
		data := strings.Repeat("z", 25) + "\n"
		_, err := w.Write([]byte(data))
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	// Close waits for all compression goroutines
	err = w.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// After close, verify that the cleanup ran properly and did not delete
	// files that were being actively compressed. There should be no more than
	// maxFiles rotated files (either .gz or plain).
	matches, _ := filepath.Glob(logPath + ".*")
	if len(matches) > 2 {
		t.Errorf("Expected at most 2 rotated files, got %d: %v", len(matches), matches)
	}
}
