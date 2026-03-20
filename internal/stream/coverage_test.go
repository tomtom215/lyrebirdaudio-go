// SPDX-License-Identifier: MIT

package stream

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBackoffNilReceiver verifies all Backoff methods are safe on nil receiver.
func TestBackoffNilReceiver(t *testing.T) {
	var b *Backoff

	t.Run("RecordFailure", func(t *testing.T) {
		b.RecordFailure() // should not panic
	})

	t.Run("RecordSuccess", func(t *testing.T) {
		b.RecordSuccess(1 * time.Second) // should not panic
	})

	t.Run("CurrentDelay", func(t *testing.T) {
		if d := b.CurrentDelay(); d != 0 {
			t.Errorf("nil.CurrentDelay() = %v, want 0", d)
		}
	})

	t.Run("Attempts", func(t *testing.T) {
		if a := b.Attempts(); a != 0 {
			t.Errorf("nil.Attempts() = %d, want 0", a)
		}
	})

	t.Run("MaxAttempts", func(t *testing.T) {
		if m := b.MaxAttempts(); m != 0 {
			t.Errorf("nil.MaxAttempts() = %d, want 0", m)
		}
	})

	t.Run("ConsecutiveFailures", func(t *testing.T) {
		if c := b.ConsecutiveFailures(); c != 0 {
			t.Errorf("nil.ConsecutiveFailures() = %d, want 0", c)
		}
	})

	t.Run("ShouldStop", func(t *testing.T) {
		if !b.ShouldStop() {
			t.Error("nil.ShouldStop() = false, want true (fail-safe)")
		}
	})

	t.Run("Reset", func(t *testing.T) {
		b.Reset() // should not panic
	})

	t.Run("Wait", func(t *testing.T) {
		b.Wait() // should not panic
	})

	t.Run("WaitContext", func(t *testing.T) {
		err := b.WaitContext(context.Background())
		if err != nil {
			t.Errorf("nil.WaitContext() = %v, want nil", err)
		}
	})
}

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

// TestReleaseLockNil verifies releaseLock is safe when lock is nil.
func TestReleaseLockNil(t *testing.T) {
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
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// lock is nil at this point; releaseLock should not panic
	mgr.releaseLock()
}

// TestBackoffOverflowProtection verifies currentDelay resets if it becomes
// zero or negative (e.g., from time.Duration overflow).
func TestBackoffOverflowProtection(t *testing.T) {
	b := NewBackoff(1*time.Millisecond, 100*time.Millisecond, 50)

	// Manually set delay to 0 to simulate overflow
	b.mu.Lock()
	b.currentDelay = 0
	b.mu.Unlock()

	b.RecordFailure()

	// After overflow protection, delay should be reset to initialDelay
	if b.CurrentDelay() != 1*time.Millisecond {
		t.Errorf("CurrentDelay() = %v after overflow, want initialDelay (1ms)", b.CurrentDelay())
	}
}

// TestRecordSuccessShortRunOverflowProtection verifies overflow protection
// in RecordSuccess for short runs.
func TestRecordSuccessShortRunOverflowProtection(t *testing.T) {
	b := NewBackoff(1*time.Millisecond, 100*time.Millisecond, 50)

	// Manually set delay to 0 to simulate overflow
	b.mu.Lock()
	b.currentDelay = 0
	b.mu.Unlock()

	b.RecordSuccess(1 * time.Second) // short run (< 300s threshold)

	// After overflow protection, delay should be reset to initialDelay
	if b.CurrentDelay() != 1*time.Millisecond {
		t.Errorf("CurrentDelay() = %v after short-run overflow, want initialDelay (1ms)", b.CurrentDelay())
	}
}

// TestRecordSuccessShortRunMaxDelayCap verifies max delay cap in RecordSuccess
// for short runs.
func TestRecordSuccessShortRunMaxDelayCap(t *testing.T) {
	b := NewBackoff(60*time.Millisecond, 100*time.Millisecond, 50)

	// First call should double: 60 -> 120, capped to 100
	b.RecordSuccess(1 * time.Second) // short run

	if b.CurrentDelay() != 100*time.Millisecond {
		t.Errorf("CurrentDelay() = %v, want 100ms (capped)", b.CurrentDelay())
	}
}
