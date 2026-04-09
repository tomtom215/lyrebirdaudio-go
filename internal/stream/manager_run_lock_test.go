package stream

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestManagerRunLockAcquisitionFailure verifies behavior when lock acquisition fails.
// With context-aware lock acquisition, this test completes quickly (< 1 second).
func TestManagerRunLockAcquisitionFailure(t *testing.T) {
	// Create a lock dir and pre-acquire a lock to force second acquisition to fail
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "1",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep",
		Backoff:      NewBackoff(100*time.Millisecond, 1*time.Second, 3),
	}

	// First manager acquires the lock
	mgr1, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(1) error = %v", err)
	}

	if err := mgr1.acquireLock(context.Background()); err != nil {
		t.Fatalf("First lock acquisition should succeed: %v", err)
	}
	defer mgr1.releaseLock()

	// Second manager should fail quickly when context is cancelled
	mgr2, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(2) error = %v", err)
	}

	// Use a cancelled context - lock acquisition should fail immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	start := time.Now()
	err = mgr2.Run(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run() should fail when context is cancelled")
	}

	// Should fail quickly (< 1 second) due to context cancellation
	if elapsed > 1*time.Second {
		t.Errorf("Lock acquisition took %v, expected < 1s with cancelled context", elapsed)
	}

	if !strings.Contains(err.Error(), "failed to acquire lock") {
		t.Errorf("Run() error = %q, want error containing 'failed to acquire lock'", err.Error())
	}

	// Should still be in idle state (never got past lock acquisition)
	state := mgr2.State()
	if state != StateIdle {
		t.Errorf("State after lock failure = %v, expected Idle", state)
	}
}

// TestManagerRunContextCancelledImmediately verifies graceful shutdown when context cancelled immediately.
// With context-aware lock acquisition, this fails during lock acquisition (before starting).
func TestManagerRunContextCancelledImmediately(t *testing.T) {
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "10", // Numeric argument for sleep
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep",
		Backoff:      NewBackoff(100*time.Millisecond, 1*time.Second, 3),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = mgr.Run(ctx)

	// Should fail during lock acquisition with wrapped context.Canceled error
	if err == nil {
		t.Fatal("Run() should fail with cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want error wrapping context.Canceled", err)
	}

	// Should remain in idle state (never got past lock acquisition)
	if mgr.State() != StateIdle {
		t.Errorf("State after immediate cancel = %v, want StateIdle", mgr.State())
	}
}

// TestManagerRunContextCancelledDuringRun verifies graceful shutdown during execution.
func TestManagerRunContextCancelledDuringRun(t *testing.T) {
	lockDir := t.TempDir()

	// Create a script that sleeps ignoring all arguments
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\nsleep 10\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy", // Arguments don't matter - script ignores them
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(100*time.Millisecond, 1*time.Second, 3),
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

	// Wait for manager to start
	time.Sleep(200 * time.Millisecond)

	// Verify it's running
	if mgr.State() != StateRunning {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Errorf("State before cancel = %v, want StateRunning", mgr.State())
	}

	// Cancel context
	cancel()

	// Wait for Run to complete
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not complete within timeout")
	}

	// Should be in stopped state
	if mgr.State() != StateStopped {
		t.Errorf("State after cancel = %v, want StateStopped", mgr.State())
	}
}

// TestManagerRunMaxAttemptsExceeded verifies behavior when max restart attempts exceeded.
func TestManagerRunMaxAttemptsExceeded(t *testing.T) {
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
		FFmpegPath:   "/bin/false", // Always fails immediately
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 3),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	err = mgr.Run(ctx)

	if err == nil {
		t.Fatal("Run() should fail when max attempts exceeded")
	}

	if !strings.Contains(err.Error(), "max restart attempts") {
		t.Errorf("Run() error = %q, want error containing 'max restart attempts'", err.Error())
	}

	// Should be in failed state
	if mgr.State() != StateFailed {
		t.Errorf("State after max attempts = %v, want StateFailed", mgr.State())
	}

	// Verify attempts counter
	if mgr.Attempts() < 3 {
		t.Errorf("Attempts = %d, want >= 3", mgr.Attempts())
	}

	// Verify failures counter
	if mgr.Failures() < 3 {
		t.Errorf("Failures = %d, want >= 3", mgr.Failures())
	}
}
