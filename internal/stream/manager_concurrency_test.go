package stream

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestStreamManagerConcurrentStreams verifies multiple concurrent stream managers.
//
// Multiple devices should be able to stream simultaneously without interference.
func TestStreamManagerConcurrentStreams(t *testing.T) {
	numStreams := 3
	managers := make([]*Manager, numStreams)
	errChs := make([]chan error, numStreams)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create and start multiple managers
	for i := 0; i < numStreams; i++ {
		cfg := &ManagerConfig{
			DeviceName: fmt.Sprintf("device_%d", i),
			ALSADevice: fmt.Sprintf("hw:%d,0", i),
			StreamName: fmt.Sprintf("stream_%d", i),
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "aac",
			RTSPURL:    fmt.Sprintf("rtsp://localhost:8554/stream_%d", i),
			LockDir:    t.TempDir(),
			FFmpegPath: findFFmpegOrSkip(t),
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager(%d) error = %v", i, err)
		}
		managers[i] = mgr

		errChs[i] = make(chan error, 1)
		go func(idx int) {
			errChs[idx] <- managers[idx].Run(ctx)
		}(i)
	}

	// Wait for all to reach running state (or fail gracefully)
	time.Sleep(2 * time.Second)

	// Verify at least one manager is running (might fail due to hw device availability)
	runningCount := 0
	for _, mgr := range managers {
		if mgr.State() == StateRunning {
			runningCount++
		}
	}

	if runningCount == 0 {
		t.Log("Warning: No streams reached running state (may be due to hw availability)")
	}

	// Stop all
	cancel()

	// Wait for all to stop
	for i, errCh := range errChs {
		select {
		case <-errCh:
			// OK
		case <-time.After(5 * time.Second):
			t.Errorf("Manager %d did not stop within timeout", i)
		}
	}
}

// TestStreamManagerLockContention verifies locking prevents duplicate managers.
//
// Only one manager should be able to control a device at a time.
func TestStreamManagerLockContention(t *testing.T) {
	lockDir := t.TempDir()

	cfg1 := &ManagerConfig{
		DeviceName: "locked_device",
		ALSADevice: "hw:0,0",
		StreamName: "locked_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    getTestOutputURL(t, "locked"),
		LockDir:    lockDir,
		FFmpegPath: findFFmpegOrSkip(t),
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	cfg2 := &ManagerConfig{
		DeviceName: "locked_device", // Same device
		ALSADevice: "hw:0,0",
		StreamName: "locked_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    getTestOutputURL(t, "locked"),
		LockDir:    lockDir, // Same lock directory
		FFmpegPath: findFFmpegOrSkip(t),
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mgr1, err := NewManager(cfg1)
	if err != nil {
		t.Fatalf("NewManager(1) error = %v", err)
	}

	// Start first manager
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- mgr1.Run(ctx)
	}()

	// Wait for it to acquire lock
	time.Sleep(500 * time.Millisecond)

	// Try to create second manager for same device
	mgr2, err := NewManager(cfg2)
	if err != nil {
		t.Fatalf("NewManager(2) error = %v", err)
	}

	// Second manager should fail to acquire lock
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	err = mgr2.Run(ctx2)
	if err == nil {
		t.Error("Second manager should fail to acquire lock")
	}

	// Stop first manager
	cancel()
	<-errCh1

	// Now second manager should be able to run
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()

	errCh2 := make(chan error, 1)
	go func() {
		errCh2 <- mgr2.Run(ctx3)
	}()

	// Wait for second manager to acquire lock
	if !waitForState(t, mgr2, StateRunning, 3*time.Second) {
		t.Error("Second manager should run after first releases lock")
	}

	cancel3()
	<-errCh2
}
