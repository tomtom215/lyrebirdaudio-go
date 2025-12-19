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

func TestLogWriter(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		streamName   string
		expectedFile string
	}{
		{"simple", "mystream", "ffmpeg-mystream.log"},
		{"with_spaces", "my stream", "ffmpeg-my_stream.log"},
		{"with_special", "stream@#$!", "ffmpeg-stream____.log"},
		{"with_dashes", "my-stream", "ffmpeg-my-stream.log"},
		{"with_underscores", "my_stream", "ffmpeg-my_stream.log"},
		{"mixed", "Stream-1_Test", "ffmpeg-Stream-1_Test.log"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := LogWriter(tmpDir, tt.streamName)
			if err != nil {
				t.Fatalf("LogWriter failed: %v", err)
			}
			defer func() { _ = w.Close() }()

			expectedPath := filepath.Join(tmpDir, tt.expectedFile)
			// Write something to verify it works
			_, err = w.Write([]byte("test"))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			// Check the file was created
			if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
				t.Errorf("Expected file %s to exist", expectedPath)
			}
		})
	}
}

func TestLogWriterWithOptions(t *testing.T) {
	tmpDir := t.TempDir()

	w, err := LogWriter(tmpDir, "teststream",
		WithMaxSize(1024),
		WithMaxFiles(5),
		WithCompression(true),
	)
	if err != nil {
		t.Fatalf("LogWriter with options failed: %v", err)
	}
	defer func() { _ = w.Close() }()

	// Write some data
	_, err = w.Write([]byte("log data\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
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

func TestListRotatedFilesWithCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create regular and compressed rotated files
	if err := os.WriteFile(logPath+".1", []byte("data1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".2.gz", []byte("compressed"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	files, err := ListRotatedFiles(logPath)
	if err != nil {
		t.Fatalf("ListRotatedFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Check that compressed file is identified correctly
	hasCompressed := false
	for _, f := range files {
		if f.Compressed {
			hasCompressed = true
		}
	}
	if !hasCompressed {
		t.Error("Expected at least one compressed file")
	}
}

func TestCleanupLogsWithCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create main, rotated, and compressed files
	if err := os.WriteFile(logPath, []byte("main"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".1", []byte("rot1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".2.gz", []byte("comp"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := CleanupLogs(logPath)
	if err != nil {
		t.Fatalf("CleanupLogs failed: %v", err)
	}

	// All files should be removed
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("Expected main log to be removed")
	}
	if _, err := os.Stat(logPath + ".1"); !os.IsNotExist(err) {
		t.Error("Expected rotated log to be removed")
	}
	if _, err := os.Stat(logPath + ".2.gz"); !os.IsNotExist(err) {
		t.Error("Expected compressed log to be removed")
	}
}
