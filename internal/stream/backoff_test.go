package stream

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestBackoffInitialState verifies the initial state of a new Backoff.
//
// Reference: mediamtx-stream-manager.sh lines 187-188, 2001
func TestBackoffInitialState(t *testing.T) {
	backoff := NewBackoff(10*time.Second, 300*time.Second, 50)

	if backoff.CurrentDelay() != 10*time.Second {
		t.Errorf("CurrentDelay() = %v, want %v", backoff.CurrentDelay(), 10*time.Second)
	}

	if backoff.Attempts() != 0 {
		t.Errorf("Attempts() = %d, want 0", backoff.Attempts())
	}

	if backoff.ConsecutiveFailures() != 0 {
		t.Errorf("ConsecutiveFailures() = %d, want 0", backoff.ConsecutiveFailures())
	}
}

// TestBackoffExponentialIncrease verifies delay doubles on each failure.
//
// Reference: mediamtx-stream-manager.sh lines 2259-2262
func TestBackoffExponentialIncrease(t *testing.T) {
	backoff := NewBackoff(10*time.Second, 300*time.Second, 50)

	tests := []struct {
		attempt    int
		wantDelay  time.Duration
		wantCapped bool
	}{
		{1, 10 * time.Second, false},  // Initial
		{2, 20 * time.Second, false},  // 10 * 2
		{3, 40 * time.Second, false},  // 20 * 2
		{4, 80 * time.Second, false},  // 40 * 2
		{5, 160 * time.Second, false}, // 80 * 2
		{6, 300 * time.Second, true},  // 160 * 2 = 320, capped to 300
		{7, 300 * time.Second, true},  // Still capped
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := backoff.CurrentDelay()
			if delay != tt.wantDelay {
				t.Errorf("Attempt %d: CurrentDelay() = %v, want %v", tt.attempt, delay, tt.wantDelay)
			}

			// Record failure to advance backoff
			backoff.RecordFailure()

			// Check if we're at max delay
			if tt.wantCapped && backoff.CurrentDelay() != 300*time.Second {
				t.Errorf("Attempt %d should be capped at max delay", tt.attempt)
			}
		})
	}
}

// TestBackoffMaxDelayCap verifies delay never exceeds maximum.
func TestBackoffMaxDelayCap(t *testing.T) {
	backoff := NewBackoff(10*time.Second, 100*time.Second, 50)

	// Simulate many failures
	for i := 0; i < 20; i++ {
		backoff.RecordFailure()
	}

	if backoff.CurrentDelay() > 100*time.Second {
		t.Errorf("CurrentDelay() = %v, exceeds max of %v", backoff.CurrentDelay(), 100*time.Second)
	}
}

// TestBackoffResetOnSuccess verifies delay resets after successful run.
//
// Reference: mediamtx-stream-manager.sh lines 2282-2285
func TestBackoffResetOnSuccess(t *testing.T) {
	backoff := NewBackoff(10*time.Second, 300*time.Second, 50)

	// Simulate several failures
	for i := 0; i < 5; i++ {
		backoff.RecordFailure()
	}

	// Current delay should be > initial
	if backoff.CurrentDelay() <= 10*time.Second {
		t.Errorf("After failures, delay should be > initial")
	}

	// Record success with long run time (> 300s threshold)
	backoff.RecordSuccess(350 * time.Second)

	// Delay should reset to initial
	if backoff.CurrentDelay() != 10*time.Second {
		t.Errorf("After success, CurrentDelay() = %v, want %v", backoff.CurrentDelay(), 10*time.Second)
	}

	// Consecutive failures should reset
	if backoff.ConsecutiveFailures() != 0 {
		t.Errorf("After success, ConsecutiveFailures() = %d, want 0", backoff.ConsecutiveFailures())
	}
}

// TestBackoffNoResetOnShortRun verifies delay doubles for short runs.
//
// Short runs (< 300s) are treated as failures and delay is doubled.
//
// Reference: mediamtx-stream-manager.sh lines 2282-2297
func TestBackoffNoResetOnShortRun(t *testing.T) {
	backoff := NewBackoff(10*time.Second, 300*time.Second, 50)

	// Simulate failure (delay: 10s -> 20s)
	backoff.RecordFailure()
	if backoff.CurrentDelay() != 20*time.Second {
		t.Errorf("After first failure, delay = %v, want 20s", backoff.CurrentDelay())
	}

	// Record success with short run time (< 300s threshold)
	// This counts as another failure, so delay should double again (20s -> 40s)
	backoff.RecordSuccess(60 * time.Second)

	// Delay should have doubled (short run is treated as failure)
	if backoff.CurrentDelay() != 40*time.Second {
		t.Errorf("After short run, delay = %v, want 40s", backoff.CurrentDelay())
	}

	// Consecutive failures should increment (short run counts as failure)
	if backoff.ConsecutiveFailures() != 2 {
		t.Errorf("After short run, ConsecutiveFailures() = %d, want 2", backoff.ConsecutiveFailures())
	}
}

// TestBackoffMaxAttempts verifies attempt limit enforcement.
//
// Reference: mediamtx-stream-manager.sh lines 2243-2246
func TestBackoffMaxAttempts(t *testing.T) {
	maxAttempts := 10
	backoff := NewBackoff(10*time.Second, 300*time.Second, maxAttempts)

	// Simulate failures up to max
	for i := 0; i < maxAttempts; i++ {
		if backoff.ShouldStop() {
			t.Errorf("ShouldStop() = true at attempt %d, want false", i)
		}
		backoff.RecordFailure()
	}

	// After max attempts, should stop
	if !backoff.ShouldStop() {
		t.Error("ShouldStop() = false after max attempts, want true")
	}
}

// TestBackoffConsecutiveFailures verifies consecutive failure tracking.
//
// Reference: mediamtx-stream-manager.sh lines 2238-2241
func TestBackoffConsecutiveFailures(t *testing.T) {
	backoff := NewBackoff(10*time.Second, 300*time.Second, 50)

	// Record 3 failures
	backoff.RecordFailure()
	backoff.RecordFailure()
	backoff.RecordFailure()

	if backoff.ConsecutiveFailures() != 3 {
		t.Errorf("ConsecutiveFailures() = %d, want 3", backoff.ConsecutiveFailures())
	}

	// Success resets consecutive failures
	backoff.RecordSuccess(350 * time.Second)

	if backoff.ConsecutiveFailures() != 0 {
		t.Errorf("ConsecutiveFailures() = %d after success, want 0", backoff.ConsecutiveFailures())
	}
}

// TestBackoffReset verifies manual reset.
func TestBackoffReset(t *testing.T) {
	backoff := NewBackoff(10*time.Second, 300*time.Second, 50)

	// Simulate failures
	for i := 0; i < 5; i++ {
		backoff.RecordFailure()
	}

	// Reset
	backoff.Reset()

	// Everything should be back to initial state
	if backoff.CurrentDelay() != 10*time.Second {
		t.Errorf("After Reset(), CurrentDelay() = %v, want %v", backoff.CurrentDelay(), 10*time.Second)
	}
	if backoff.Attempts() != 0 {
		t.Errorf("After Reset(), Attempts() = %d, want 0", backoff.Attempts())
	}
	if backoff.ConsecutiveFailures() != 0 {
		t.Errorf("After Reset(), ConsecutiveFailures() = %d, want 0", backoff.ConsecutiveFailures())
	}
}

// TestBackoffWaitActuallyWaits verifies Wait() blocks for the correct duration.
func TestBackoffWaitActuallyWaits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}

	backoff := NewBackoff(100*time.Millisecond, 1*time.Second, 50)

	start := time.Now()
	backoff.Wait()
	elapsed := time.Since(start)

	// Should wait ~100ms (initial delay)
	if elapsed < 90*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("Wait() took %v, expected ~100ms", elapsed)
	}
}

// TestBackoffWaitContextCancellation verifies Wait() respects context cancellation.
func TestBackoffWaitContextCancellation(t *testing.T) {
	backoff := NewBackoff(5*time.Second, 300*time.Second, 50)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := backoff.WaitContext(ctx)
	elapsed := time.Since(start)

	// Should return early due to context timeout
	if err == nil {
		t.Error("WaitContext() should return error on context cancellation")
	}

	if elapsed > 200*time.Millisecond {
		t.Errorf("WaitContext() took %v, should cancel quickly", elapsed)
	}
}

// TestBackoffConcurrentAccess verifies thread safety.
func TestBackoffConcurrentAccess(t *testing.T) {
	backoff := NewBackoff(10*time.Millisecond, 100*time.Millisecond, 1000)

	var wg sync.WaitGroup
	const numGoroutines = 10

	// Concurrent RecordFailure
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				backoff.RecordFailure()
			}
		}()
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = backoff.CurrentDelay()
				_ = backoff.Attempts()
			}
		}()
	}

	wg.Wait()

	// Should have recorded all failures
	if backoff.Attempts() != numGoroutines*10 {
		t.Errorf("Attempts() = %d, want %d", backoff.Attempts(), numGoroutines*10)
	}
}

// BenchmarkBackoffRecordFailure measures performance of RecordFailure.
func BenchmarkBackoffRecordFailure(b *testing.B) {
	backoff := NewBackoff(10*time.Second, 300*time.Second, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backoff.RecordFailure()
	}
}
