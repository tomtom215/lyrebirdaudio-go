// SPDX-License-Identifier: MIT

package stream

import (
	"context"
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
