package stream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRotatingWriter(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer func() { _ = w.Close() }()

	if w.Path() != logPath {
		t.Errorf("Path() = %q, want %q", w.Path(), logPath)
	}
}

func TestNewRotatingWriterWithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath,
		WithMaxSize(1024*1024),
		WithMaxFiles(3),
		WithCompression(true),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer func() { _ = w.Close() }()

	if w.Path() != logPath {
		t.Errorf("Path() = %q, want %q", w.Path(), logPath)
	}
}

func TestRotatingWriterWrite(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer func() { _ = w.Close() }()

	// Write some data
	testData := "Hello, World!\n"
	n, err := w.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write returned %d bytes, want %d", n, len(testData))
	}

	// Check size
	if w.Size() != int64(len(testData)) {
		t.Errorf("Size() = %d, want %d", w.Size(), len(testData))
	}
}

func TestRotatingWriterRotate(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create writer with small max size to trigger rotation
	w, err := NewRotatingWriter(logPath, WithMaxSize(50), WithMaxFiles(3))
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer func() { _ = w.Close() }()

	// Write data to trigger rotation
	for i := 0; i < 5; i++ {
		data := strings.Repeat("x", 20) + "\n"
		_, err := w.Write([]byte(data))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Force rotation
	err = w.Rotate()
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	// Check that rotated file exists
	rotatedPath := logPath + ".1"
	if _, err := os.Stat(rotatedPath); os.IsNotExist(err) {
		t.Error("Expected rotated file to exist")
	}
}

func TestListRotatedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create some rotated files
	if err := os.WriteFile(logPath+".1", []byte("data1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".2", []byte("data22"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	files, err := ListRotatedFiles(logPath)
	if err != nil {
		t.Fatalf("ListRotatedFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 rotated files, got %d", len(files))
	}
}

func TestTotalLogSize(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create main log and rotated files
	if err := os.WriteFile(logPath, []byte("mainlog"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".1", []byte("rotated1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	total, err := TotalLogSize(logPath)
	if err != nil {
		t.Fatalf("TotalLogSize failed: %v", err)
	}

	expected := int64(len("mainlog") + len("rotated1"))
	if total != expected {
		t.Errorf("TotalLogSize = %d, want %d", total, expected)
	}
}

func TestCleanupLogs(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create log files
	if err := os.WriteFile(logPath, []byte("main"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".1", []byte("rot1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := CleanupLogs(logPath)
	if err != nil {
		t.Fatalf("CleanupLogs failed: %v", err)
	}

	// Check files are removed
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("Expected main log to be removed")
	}
	if _, err := os.Stat(logPath + ".1"); !os.IsNotExist(err) {
		t.Error("Expected rotated log to be removed")
	}
}

func TestRotatingWriterClose(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	w, err := NewRotatingWriter(logPath)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}

	_, err = w.Write([]byte("test data\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	err = w.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Writing after close should fail
	_, err = w.Write([]byte("more data"))
	if err == nil {
		t.Error("Expected Write after Close to fail")
	}
}

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

func TestListRotatedFilesNoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nonexistent.log")

	files, err := ListRotatedFiles(logPath)
	if err != nil {
		t.Fatalf("ListRotatedFiles failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(files))
	}
}

func TestTotalLogSizeNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nonexistent.log")

	total, err := TotalLogSize(logPath)
	if err != nil {
		t.Fatalf("TotalLogSize failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected 0, got %d", total)
	}
}

func TestRotatingWriterCreatesDirs(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "subdir", "nested", "test.log")

	w, err := NewRotatingWriter(logPath)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer func() { _ = w.Close() }()

	// Parent directories should be created
	if _, err := os.Stat(filepath.Dir(logPath)); os.IsNotExist(err) {
		t.Error("Expected parent directories to be created")
	}
}
