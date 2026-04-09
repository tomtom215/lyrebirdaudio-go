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
