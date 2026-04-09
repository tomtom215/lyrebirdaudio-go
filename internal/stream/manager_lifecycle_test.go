package stream

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestManagerMetricsInitialState verifies initial metrics state.
func TestManagerMetricsInitialState(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test_device",
		ALSADevice: "hw:0,0",
		StreamName: "test_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    "/tmp",
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	metrics := mgr.Metrics()

	if metrics.DeviceName != "test_device" {
		t.Errorf("Metrics.DeviceName = %q, want \"test_device\"", metrics.DeviceName)
	}

	if metrics.StreamName != "test_stream" {
		t.Errorf("Metrics.StreamName = %q, want \"test_stream\"", metrics.StreamName)
	}

	if metrics.State != StateIdle {
		t.Errorf("Metrics.State = %v, want StateIdle", metrics.State)
	}

	if !metrics.StartTime.IsZero() {
		t.Error("Metrics.StartTime should be zero initially")
	}

	if metrics.Uptime != 0 {
		t.Errorf("Metrics.Uptime = %v, want 0", metrics.Uptime)
	}

	if metrics.Attempts != 0 {
		t.Errorf("Metrics.Attempts = %d, want 0", metrics.Attempts)
	}

	if metrics.Failures != 0 {
		t.Errorf("Metrics.Failures = %d, want 0", metrics.Failures)
	}
}

// TestManagerLogf verifies logging functionality.
func TestManagerLogf(t *testing.T) {
	tests := []struct {
		name        string
		hasLogger   bool
		format      string
		args        []interface{}
		wantContain string // Structured log contains the message
		wantEmpty   bool
	}{
		{
			name:        "with logger",
			hasLogger:   true,
			format:      "test message %d",
			args:        []interface{}{42},
			wantContain: "test message 42",
		},
		{
			name:        "with logger no args",
			hasLogger:   true,
			format:      "simple message",
			args:        []interface{}{},
			wantContain: "simple message",
		},
		{
			name:      "without logger",
			hasLogger: false,
			format:    "test message",
			args:      []interface{}{},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture log output
			var buf bytes.Buffer

			cfg := &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			}

			if tt.hasLogger {
				cfg.Logger = slog.New(slog.NewTextHandler(&buf, nil))
			}

			mgr, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			mgr.logf(tt.format, tt.args...)

			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("logf() output = %q, want empty", got)
				}
			} else if !strings.Contains(got, tt.wantContain) {
				t.Errorf("logf() output = %q, want it to contain %q", got, tt.wantContain)
			}
		})
	}
}

// TestManagerAcquireLock verifies lock acquisition and release.
func TestManagerAcquireLock(t *testing.T) {
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName: "test_lock",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    lockDir,
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Test acquire lock
	err = mgr.acquireLock(context.Background())
	if err != nil {
		t.Fatalf("acquireLock() error = %v", err)
	}

	// Verify lock file exists
	lockPath := filepath.Join(lockDir, "test_lock.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Errorf("Lock file was not created at %s", lockPath)
	}

	// Test release lock
	mgr.releaseLock()

	// Verify lock is released (file may still exist but should be unlockable)
	// We can verify by trying to acquire again
	err = mgr.acquireLock(context.Background())
	if err != nil {
		t.Errorf("Failed to re-acquire lock after release: %v", err)
	}
	mgr.releaseLock()
}

// TestManagerStop verifies stop behavior.
func TestManagerStop(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test_stop",
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

	// Call stop() when no process is running - should not panic
	mgr.stop()

	// Verify state changed to stopping
	if mgr.State() != StateStopping {
		t.Errorf("State after stop() = %v, want StateStopping", mgr.State())
	}
}

// TestManagerForceStop verifies forceStop behavior.
func TestManagerForceStop(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test_force_stop",
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

	// Call forceStop() when no process is running - should return error
	err = mgr.forceStop()
	if err == nil {
		t.Error("forceStop() with no process should return error")
	}
}

// TestStopTimeoutConfigurable verifies H-1 fix: configurable stop timeout.
func TestStopTimeoutConfigurable(t *testing.T) {
	t.Run("default stop timeout is 5s", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName: "test",
			ALSADevice: "hw:0,0",
			StreamName: "stream",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    "/tmp",
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			// StopTimeout not set - should default to 5s
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		if mgr.cfg.StopTimeout != 0 {
			t.Errorf("default StopTimeout should be 0 (manager uses 5s default), got %v", mgr.cfg.StopTimeout)
		}
	})

	t.Run("custom stop timeout accepted", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName:  "test",
			ALSADevice:  "hw:0,0",
			StreamName:  "stream",
			SampleRate:  48000,
			Channels:    2,
			Bitrate:     "128k",
			Codec:       "opus",
			RTSPURL:     "rtsp://localhost:8554/test",
			LockDir:     "/tmp",
			FFmpegPath:  "/usr/bin/ffmpeg",
			Backoff:     NewBackoff(1*time.Second, 10*time.Second, 5),
			StopTimeout: 10 * time.Second,
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		if mgr.cfg.StopTimeout != 10*time.Second {
			t.Errorf("StopTimeout = %v, want 10s", mgr.cfg.StopTimeout)
		}
	})
}

// TestLogStructuredEvent verifies H-4 fix: structured failure event logging.
func TestLogStructuredEvent(t *testing.T) {
	t.Run("structured event logged with all fields", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := &ManagerConfig{
			DeviceName: "test_device",
			ALSADevice: "hw:0,0",
			StreamName: "test_stream",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    "/tmp",
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			Logger:     slog.New(slog.NewTextHandler(&buf, nil)),
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		mgr.logStructuredEvent("stream_failure",
			"error", "test error",
			"attempt", 3,
			"failures", 2,
		)

		output := buf.String()
		for _, want := range []string{"stream_event", "stream_failure", "test_device", "test_stream", "test error"} {
			if !strings.Contains(output, want) {
				t.Errorf("H-4: structured event should contain %q, got: %s", want, output)
			}
		}
	})

	t.Run("no panic without logger", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName: "test",
			ALSADevice: "hw:0,0",
			StreamName: "stream",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    "/tmp",
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			// Logger intentionally nil
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Should not panic
		mgr.logStructuredEvent("stream_failure", "error", "test")
	})
}
