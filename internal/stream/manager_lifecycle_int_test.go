package stream

import (
	"context"
	"testing"
	"time"
)

// TestStreamManagerLifecycle verifies basic stream lifecycle management.
//
// This tests the core state machine:
//
//	idle → starting → running → stopping → stopped
func TestStreamManagerLifecycle(t *testing.T) {
	device, inputFormat := getTestAudioDevice(t)

	cfg := &ManagerConfig{
		DeviceName:  "test_device",
		ALSADevice:  device,
		InputFormat: inputFormat,
		StreamName:  "test_stream",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "aac",
		RTSPURL:     getTestOutputURL(t, "test"),
		LockDir:     t.TempDir(),
		FFmpegPath:  findFFmpegOrSkip(t),
		Backoff:     NewBackoff(1*time.Second, 10*time.Second, 5),
		Logger:      newTestLogger(t),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Verify initial state
	if mgr.State() != StateIdle {
		t.Errorf("Initial state = %v, want StateIdle", mgr.State())
	}

	// Start stream
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for running state
	if !waitForState(t, mgr, StateRunning, 5*time.Second) {
		t.Fatal("Stream did not reach running state")
	}

	// Stop stream
	cancel()

	// Wait for stopped state
	if !waitForState(t, mgr, StateStopped, 5*time.Second) {
		t.Fatal("Stream did not reach stopped state")
	}

	// Verify Run returns without error
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Run() error = %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Run() did not return after context cancellation")
	}
}

// TestStreamManagerFailureRestart verifies exponential backoff on failures.
//
// When FFmpeg exits with an error, the manager should:
// 1. Enter failed state
// 2. Wait according to backoff policy
// 3. Attempt restart
// 4. Stop after max attempts
func TestStreamManagerFailureRestart(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "failing_device",
		ALSADevice: "hw:99,99", // Non-existent device
		StreamName: "failing_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    getTestOutputURL(t, "failing"),
		LockDir:    t.TempDir(),
		FFmpegPath: findFFmpegOrSkip(t),
		Backoff:    NewBackoff(100*time.Millisecond, 1*time.Second, 3),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	startTime := time.Now()
	err = mgr.Run(ctx)

	// Should fail after 3 attempts with backoff
	if err == nil {
		t.Error("Run() expected error for failing device, got nil")
	}

	elapsed := time.Since(startTime)
	// With 3 attempts and backoff of 100ms, 200ms, 400ms = ~700ms minimum
	if elapsed < 500*time.Millisecond {
		t.Errorf("Run() completed too quickly (%v), backoff not working", elapsed)
	}

	// Verify final state
	if mgr.State() != StateFailed {
		t.Errorf("Final state = %v, want StateFailed", mgr.State())
	}

	// Verify attempts counter
	if mgr.Attempts() != 3 {
		t.Errorf("Attempts = %d, want 3", mgr.Attempts())
	}
}

// TestStreamManagerShortRunRestart verifies restart after short successful run.
//
// If FFmpeg runs < 300s (success threshold), treat as failure and restart.
func TestStreamManagerShortRunRestart(t *testing.T) {
	device, inputFormat := getTestAudioDevice(t)

	cfg := &ManagerConfig{
		DeviceName:  "short_run_device",
		ALSADevice:  device,
		InputFormat: inputFormat,
		StreamName:  "short_run",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "aac",
		RTSPURL:     getTestOutputURL(t, "short"),
		LockDir:     t.TempDir(),
		FFmpegPath:  findFFmpegOrSkip(t),
		Backoff:     NewBackoff(100*time.Millisecond, 1*time.Second, 5),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Start stream
	go func() { _ = mgr.Run(ctx) }()

	// Wait for running state
	if !waitForState(t, mgr, StateRunning, 3*time.Second) {
		t.Fatal("Stream did not reach running state")
	}

	// Kill FFmpeg after short run (simulating crash)
	time.Sleep(500 * time.Millisecond)
	if err := mgr.forceStop(); err != nil {
		t.Logf("forceStop() error (expected): %v", err)
	}

	// Should enter failed state
	if !waitForState(t, mgr, StateFailed, 2*time.Second) {
		t.Error("Stream did not enter failed state after short run")
	}

	// Wait a bit longer for backoff and restart attempt
	time.Sleep(300 * time.Millisecond)

	// Manager should be attempting to restart (not stopped)
	// It could be in Starting, Running, or Failed (if it crashed again)
	state := mgr.State()
	if state == StateStopped {
		t.Errorf("State after backoff = %v, manager should be attempting restart", state)
	}

	// Verify it's still trying to recover (not permanently stopped)
	if mgr.Attempts() < 2 {
		t.Errorf("Attempts = %d, expected at least 2 attempts", mgr.Attempts())
	}
}

// TestStreamManagerGracefulShutdown verifies clean shutdown during various states.
func TestStreamManagerGracefulShutdown(t *testing.T) {
	tests := []struct {
		name       string
		shutdownAt State
		waitTime   time.Duration
	}{
		{"shutdown during starting", StateStarting, 100 * time.Millisecond},
		{"shutdown during running", StateRunning, 1 * time.Second},
		{"shutdown during backoff", StateFailed, 200 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, inputFormat := getTestAudioDevice(t)
			cfg := &ManagerConfig{
				DeviceName:  "shutdown_test",
				ALSADevice:  device,
				InputFormat: inputFormat,
				StreamName:  "shutdown",
				SampleRate:  48000,
				Channels:    2,
				Bitrate:     "128k",
				Codec:       "aac",
				RTSPURL:     getTestOutputURL(t, "shutdown"),
				LockDir:     t.TempDir(),
				FFmpegPath:  findFFmpegOrSkip(t),
				Backoff:     NewBackoff(1*time.Second, 10*time.Second, 3),
			}

			ctx, cancel := context.WithCancel(context.Background())

			mgr, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			errCh := make(chan error, 1)
			go func() {
				errCh <- mgr.Run(ctx)
			}()

			// Wait for desired state (with timeout)
			if tt.shutdownAt != StateStarting {
				// For states other than starting, wait for that state
				if !waitForState(t, mgr, tt.shutdownAt, 10*time.Second) {
					cancel()
					t.Fatalf("Failed to reach state %v before shutdown", tt.shutdownAt)
				}
			} else {
				// For starting state, just wait briefly as it transitions quickly
				time.Sleep(tt.waitTime)
			}

			// Cancel context
			cancel()

			// Verify graceful shutdown
			select {
			case err := <-errCh:
				if err != nil && err != context.Canceled {
					t.Errorf("Run() error = %v, want context.Canceled", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("Shutdown timeout - manager did not stop gracefully")
			}

			// Verify stopped state
			if mgr.State() != StateStopped {
				t.Errorf("Final state = %v, want StateStopped", mgr.State())
			}
		})
	}
}
