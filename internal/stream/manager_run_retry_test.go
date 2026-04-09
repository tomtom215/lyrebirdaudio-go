package stream

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestManagerRunShortRunTreatedAsFailure verifies FFmpeg runs < 300s are treated as failures.
func TestManagerRunShortRunTreatedAsFailure(t *testing.T) {
	lockDir := t.TempDir()

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "0.1", // Argument to sleep
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep", // Sleep for short duration
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 2),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = mgr.Run(ctx)

	// Should timeout or hit max attempts
	if err == nil {
		t.Fatal("Run() should fail for short runs")
	}

	// Verify failures were recorded
	if mgr.Failures() == 0 {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Error("Failures = 0, want > 0 for short runs")
	}
}

// TestManagerRunContextCancelledDuringBackoff verifies context cancellation during backoff wait.
func TestManagerRunContextCancelledDuringBackoff(t *testing.T) {
	lockDir := t.TempDir()

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy", // Argument doesn't matter for /bin/false
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/false",                                  // Always fails
		Backoff:      NewBackoff(5*time.Second, 10*time.Second, 10), // Long backoff
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for first failure and backoff to start
	time.Sleep(200 * time.Millisecond)

	// Verify state is failed (in backoff)
	if mgr.State() != StateFailed {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Errorf("State during backoff = %v, want StateFailed", mgr.State())
	}

	// Cancel during backoff
	cancel()

	// Should complete quickly (not wait full backoff)
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not complete quickly after cancel during backoff")
	}

	// Should be in stopped state
	if mgr.State() != StateStopped {
		t.Errorf("State after cancel during backoff = %v, want StateStopped", mgr.State())
	}
}

// TestManagerRunMetricsUpdate verifies metrics are updated correctly.
func TestManagerRunMetricsUpdate(t *testing.T) {
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "0.05", // Short sleep
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep",
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 3),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Initial metrics
	initialMetrics := mgr.Metrics()
	if initialMetrics.Attempts != 0 {
		t.Errorf("Initial attempts = %d, want 0", initialMetrics.Attempts)
	}
	if initialMetrics.Failures != 0 {
		t.Errorf("Initial failures = %d, want 0", initialMetrics.Failures)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = mgr.Run(ctx)

	// Verify metrics were updated
	finalMetrics := mgr.Metrics()
	if finalMetrics.Attempts == 0 {
		t.Error("Attempts should be > 0 after Run()")
	}
	if finalMetrics.Failures == 0 {
		t.Error("Failures should be > 0 after short runs")
	}

	// Verify device and stream names in metrics
	if finalMetrics.DeviceName != "test" {
		t.Errorf("Metrics.DeviceName = %q, want \"test\"", finalMetrics.DeviceName)
	}
	if finalMetrics.StreamName != "test" {
		t.Errorf("Metrics.StreamName = %q, want \"test\"", finalMetrics.StreamName)
	}
}
