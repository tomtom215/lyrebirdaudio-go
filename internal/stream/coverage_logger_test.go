// SPDX-License-Identifier: MIT

package stream

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWithRotateLogger verifies the WithRotateLogger option sets the logger.
func TestWithRotateLogger(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	w, err := NewRotatingWriter(logPath,
		WithMaxSize(1024),
		WithMaxFiles(3),
		WithRotateLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}
	defer w.Close()

	if w.logger == nil {
		t.Error("WithRotateLogger should set logger on RotatingWriter")
	}
}

// TestLogWriterCreatesWriter verifies LogWriter helper creates a writer.
func TestLogWriterCreatesWriter(t *testing.T) {
	dir := t.TempDir()

	w, err := LogWriter(dir, "test_stream",
		WithMaxSize(DefaultMaxLogSize),
		WithMaxFiles(DefaultMaxLogFiles),
	)
	if err != nil {
		t.Fatalf("LogWriter() error = %v", err)
	}
	defer w.Close()

	// Write some data
	_, err = w.Write([]byte("test log line\n"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
}

// TestNewManagerWithLogDir verifies log writer is created when LogDir is set.
func TestNewManagerWithLogDir(t *testing.T) {
	logDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName: "test",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    t.TempDir(),
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
		LogDir:     logDir,
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	mgr.mu.RLock()
	hasWriter := mgr.logWriter != nil
	mgr.mu.RUnlock()

	if !hasWriter {
		t.Error("logWriter should be set when LogDir is provided")
	}

	if err := mgr.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
