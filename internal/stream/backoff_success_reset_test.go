// SPDX-License-Identifier: MIT

package stream

import (
	"testing"
	"time"
)

// TestBackoffSuccessResetsAttemptCeiling guards against a regression where
// successful runs counted toward the max-attempts ceiling. A healthy stream
// that merely restarts many times over its life must never be permanently
// abandoned; only repeated failures WITHOUT an intervening successful run
// should trip the ceiling.
func TestBackoffSuccessResetsAttemptCeiling(t *testing.T) {
	const maxAttempts = 5
	// successThreshold 100ms; each "run" below lasts 200ms => genuine success.
	b := NewBackoffWithThreshold(1*time.Second, 10*time.Second, 100*time.Millisecond, maxAttempts)

	for i := 0; i < maxAttempts*4; i++ {
		b.RecordSuccess(200 * time.Millisecond)
		if b.ShouldStop() {
			t.Fatalf("ShouldStop() became true after %d successful restarts (attempts=%d, max=%d); "+
				"a healthy restarting stream must not be abandoned", i+1, b.Attempts(), b.MaxAttempts())
		}
	}
	if got := b.Attempts(); got != 0 {
		t.Errorf("Attempts() after successful runs = %d, want 0", got)
	}

	// Consecutive failures with no intervening success must still stop.
	for i := 0; i < maxAttempts; i++ {
		b.RecordFailure()
	}
	if !b.ShouldStop() {
		t.Errorf("ShouldStop() = false after %d consecutive failures, want true", maxAttempts)
	}
}
