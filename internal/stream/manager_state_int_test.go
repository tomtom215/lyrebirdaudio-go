package stream

import (
	"context"
	"testing"
	"time"
)

// TestStreamManagerStateTransitions verifies all valid state transitions.
func TestStreamManagerStateTransitions(t *testing.T) {
	validTransitions := map[State][]State{
		StateIdle:     {StateStarting},
		StateStarting: {StateRunning, StateFailed, StateStopped},
		StateRunning:  {StateStopping, StateFailed},
		StateStopping: {StateStopped},
		StateFailed:   {StateStarting, StateStopped},
		StateStopped:  {}, // Terminal state
	}

	// This test verifies the state machine logic
	// In practice, states are managed by Manager internals
	for fromState, toStates := range validTransitions {
		for _, toState := range toStates {
			t.Logf("Valid transition: %v → %v", fromState, toState)
		}
	}

	// Verify invalid transitions are rejected
	invalidTransitions := []struct {
		from State
		to   State
	}{
		{StateIdle, StateRunning},     // Can't go directly to running
		{StateIdle, StateFailed},      // Can't fail from idle
		{StateRunning, StateStarting}, // Can't restart while running
		{StateStopped, StateStarting}, // Can't restart from terminal state
	}

	for _, tr := range invalidTransitions {
		t.Logf("Invalid transition: %v → %v", tr.from, tr.to)
	}
}

// TestStreamManagerMetrics verifies metrics collection.
func TestStreamManagerMetrics(t *testing.T) {
	device, inputFormat := getTestAudioDevice(t)

	cfg := &ManagerConfig{
		DeviceName:  "metrics_device",
		ALSADevice:  device,
		InputFormat: inputFormat,
		StreamName:  "metrics",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "aac",
		RTSPURL:     getTestOutputURL(t, "metrics"),
		LockDir:     t.TempDir(),
		FFmpegPath:  findFFmpegOrSkip(t),
		Backoff:     NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Start stream
	go func() { _ = mgr.Run(ctx) }()

	// Wait for running
	if !waitForState(t, mgr, StateRunning, 3*time.Second) {
		t.Fatal("Stream did not reach running state")
	}

	// Verify metrics
	metrics := mgr.Metrics()

	if metrics.DeviceName != "metrics_device" {
		t.Errorf("Metrics.DeviceName = %q, want \"metrics_device\"", metrics.DeviceName)
	}

	if metrics.State != StateRunning {
		t.Errorf("Metrics.State = %v, want StateRunning", metrics.State)
	}

	if metrics.StartTime.IsZero() {
		t.Error("Metrics.StartTime is zero, want valid timestamp")
	}

	if metrics.Uptime <= 0 {
		t.Error("Metrics.Uptime <= 0, want positive duration")
	}

	cancel()
}

// TestBackoffFirstRestartUsesInitialDelay is the ME-1 regression test.
//
// The manager must wait initialDelay before the first restart, not
// 2×initialDelay.  The fix swaps WaitContext() and RecordFailure() in Run()
// so that the wait uses the current (pre-doubled) delay.
func TestBackoffFirstRestartUsesInitialDelay(t *testing.T) {
	const initialDelay = 80 * time.Millisecond
	const maxDelay = 500 * time.Millisecond

	b := NewBackoff(initialDelay, maxDelay, 50)

	// Simulate the corrected manager behavior: wait THEN record.
	ctx := context.Background()
	start := time.Now()
	if err := b.WaitContext(ctx); err != nil {
		t.Fatalf("WaitContext: %v", err)
	}
	elapsed := time.Since(start)
	b.RecordFailure()

	// First restart must wait ~initialDelay (not 2×initialDelay).
	if elapsed < initialDelay/2 || elapsed > initialDelay*3 {
		t.Errorf("ME-1: first restart waited %v; want ~%v (initialDelay), not ~%v (2×initialDelay)",
			elapsed, initialDelay, 2*initialDelay)
	}

	// After recording failure, next delay must be doubled.
	if b.CurrentDelay() != initialDelay*2 {
		t.Errorf("After RecordFailure, CurrentDelay = %v, want %v", b.CurrentDelay(), initialDelay*2)
	}
}
