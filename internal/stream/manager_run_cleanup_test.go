package stream

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestManagerRunResourceCleanup verifies lock is released on all exit paths.
func TestManagerRunResourceCleanup(t *testing.T) {
	lockDir := t.TempDir()

	tests := []struct {
		name        string
		setupCtx    func() (context.Context, context.CancelFunc)
		ffmpegPath  string
		maxAttempts int
	}{
		{
			name: "immediate cancel",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			ffmpegPath:  "/bin/sleep",
			maxAttempts: 3,
		},
		{
			name: "cancel during run",
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 200*time.Millisecond)
			},
			ffmpegPath:  "/bin/sleep",
			maxAttempts: 10,
		},
		{
			name: "max attempts exceeded",
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 1*time.Second)
			},
			ffmpegPath:  "/bin/false",
			maxAttempts: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				DeviceName:   "test_" + tt.name,
				ALSADevice:   "1",
				StreamName:   "test",
				SampleRate:   48000,
				Channels:     2,
				Bitrate:      "128k",
				Codec:        "opus",
				RTSPURL:      "/dev/null",
				OutputFormat: "null",
				LockDir:      lockDir,
				FFmpegPath:   tt.ffmpegPath,
				Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, tt.maxAttempts),
			}

			mgr, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			ctx, cancel := tt.setupCtx()
			defer cancel()

			_ = mgr.Run(ctx)

			// Verify lock was released by trying to acquire it again
			mgr2, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() for second instance error = %v", err)
			}

			err = mgr2.acquireLock(context.Background())
			if err != nil {
				t.Errorf("Lock was not released after Run(): %v", err)
			}
			mgr2.releaseLock()
		})
	}
}

// TestManagerRunStateTransitions verifies correct state transitions.
func TestManagerRunStateTransitions(t *testing.T) {
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
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 5),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Should start in idle
	if mgr.State() != StateIdle {
		t.Errorf("Initial state = %v, want StateIdle", mgr.State())
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for starting/running transition
	time.Sleep(100 * time.Millisecond)

	state := mgr.State()
	if state != StateStarting && state != StateRunning {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Errorf("State during execution = %v, want StateStarting or StateRunning", state)
	}

	// Cancel and wait
	cancel()
	<-errCh

	// Should end in stopped
	if mgr.State() != StateStopped {
		t.Errorf("Final state = %v, want StateStopped", mgr.State())
	}
}

// TestManagerRunWithPanicsInFFmpeg verifies recovery from command panics.
func TestManagerRunWithPanicsInFFmpeg(t *testing.T) {
	lockDir := t.TempDir()

	// Create a shell script that exits immediately (simulating crash)
	scriptPath := filepath.Join(lockDir, "crash.sh")
	scriptContent := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create crash script: %v", err)
	}

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 2),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = mgr.Run(ctx)

	// Should eventually fail or timeout
	if err == nil {
		t.Fatal("Run() should fail when command keeps crashing")
	}

	// Should have recorded failures
	if mgr.Failures() == 0 {
		t.Error("Failures = 0, want > 0 when command crashes")
	}
}

// TestManagerRunConcurrentCalls verifies behavior when Run() is called multiple times.
func TestManagerRunConcurrentCalls(t *testing.T) {
	lockDir := t.TempDir()

	// Create a script that sleeps ignoring all arguments
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\nsleep 60\n" // Long sleep to ensure lock is held
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_concurrent",
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
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 5),
	}

	mgr1, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(1) error = %v", err)
	}

	mgr2, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(2) error = %v", err)
	}

	// First manager runs for 5 seconds (long enough to hold lock)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	// Second manager tries immediately with short timeout
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	// Start first manager
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- mgr1.Run(ctx1)
	}()

	// Wait for first to acquire lock and start running
	time.Sleep(200 * time.Millisecond)

	// Try to start second manager - should fail to acquire lock after 2s timeout
	start := time.Now()
	err2 := mgr2.Run(ctx2)
	elapsed := time.Since(start)

	if err2 == nil {
		t.Fatal("Second Run() should fail due to lock or timeout")
	}

	// Should fail relatively quickly (within context timeout + a bit)
	if elapsed > 35*time.Second {
		t.Errorf("Second Run() took %v, expected < 35s (lock timeout is 30s)", elapsed)
	}

	// Cancel first manager
	cancel1()

	// Wait for first to complete
	<-errCh1
}
