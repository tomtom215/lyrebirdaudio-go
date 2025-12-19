package stream

import (
	"context"
	"sync"
	"time"
)

// Backoff implements exponential backoff for stream restart logic.
//
// Provides:
//   - Exponential delay increase on failures (delay *= 2)
//   - Configurable maximum delay cap
//   - Reset on successful runs (run time > threshold)
//   - Attempt counting and limits
//   - Thread-safe operations
//
// Reference: mediamtx-stream-manager.sh lines 187-188, 2243-2305
type Backoff struct {
	mu                  sync.RWMutex
	initialDelay        time.Duration
	maxDelay            time.Duration
	successThreshold    time.Duration // Run time threshold to consider success
	maxAttempts         int
	currentDelay        time.Duration
	attempts            int
	consecutiveFailures int
}

const (
	// DefaultSuccessThreshold is the run time threshold to reset backoff.
	// Matches bash: WRAPPER_SUCCESS_DURATION=300
	DefaultSuccessThreshold = 300 * time.Second
)

// NewBackoff creates a new exponential backoff instance.
//
// Parameters:
//   - initialDelay: Starting delay (e.g., 10s)
//   - maxDelay: Maximum delay cap (e.g., 300s)
//   - maxAttempts: Maximum number of restart attempts (e.g., 50)
//
// Returns:
//   - Backoff instance with initial state
//
// Example:
//
//	backoff := NewBackoff(10*time.Second, 300*time.Second, 50)
//
// Reference: mediamtx-stream-manager.sh lines 187-188
func NewBackoff(initialDelay, maxDelay time.Duration, maxAttempts int) *Backoff {
	return &Backoff{
		initialDelay:     initialDelay,
		maxDelay:         maxDelay,
		successThreshold: DefaultSuccessThreshold,
		maxAttempts:      maxAttempts,
		currentDelay:     initialDelay,
		attempts:         0,
	}
}

// NewBackoffWithThreshold creates a backoff with custom success threshold.
func NewBackoffWithThreshold(initialDelay, maxDelay, successThreshold time.Duration, maxAttempts int) *Backoff {
	return &Backoff{
		initialDelay:     initialDelay,
		maxDelay:         maxDelay,
		successThreshold: successThreshold,
		maxAttempts:      maxAttempts,
		currentDelay:     initialDelay,
		attempts:         0,
	}
}

// RecordFailure records a failed attempt and increases delay.
//
// Doubles the current delay (up to max delay cap) and increments counters.
// No-op if receiver is nil.
//
// Reference: mediamtx-stream-manager.sh lines 2259-2262, 2292-2295
func (b *Backoff) RecordFailure() {
	if b == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.attempts++
	b.consecutiveFailures++

	// Double the delay, capped at max
	b.currentDelay *= 2
	if b.currentDelay > b.maxDelay {
		b.currentDelay = b.maxDelay
	}

	// Protect against overflow (if delay somehow became negative or zero)
	if b.currentDelay <= 0 {
		b.currentDelay = b.initialDelay
	}
}

// RecordSuccess records a successful run and may reset delay.
//
// If runTime exceeds the success threshold, resets delay to initial
// and clears consecutive failures. Otherwise, treats as another failure.
// No-op if receiver is nil.
//
// Parameters:
//   - runTime: How long the process ran before exiting
//
// Reference: mediamtx-stream-manager.sh lines 2282-2297
func (b *Backoff) RecordSuccess(runTime time.Duration) {
	if b == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.attempts++

	if runTime > b.successThreshold {
		// Successful run - reset backoff
		b.currentDelay = b.initialDelay
		b.consecutiveFailures = 0
	} else {
		// Short run - treat as failure
		b.consecutiveFailures++

		// Double the delay, capped at max
		b.currentDelay *= 2
		if b.currentDelay > b.maxDelay {
			b.currentDelay = b.maxDelay
		}

		// Protect against overflow
		if b.currentDelay <= 0 {
			b.currentDelay = b.initialDelay
		}
	}
}

// CurrentDelay returns the current backoff delay.
// Returns 0 if receiver is nil.
func (b *Backoff) CurrentDelay() time.Duration {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentDelay
}

// Attempts returns the total number of attempts (successes + failures).
// Returns 0 if receiver is nil.
func (b *Backoff) Attempts() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.attempts
}

// MaxAttempts returns the maximum number of attempts allowed.
// Returns 0 if receiver is nil.
func (b *Backoff) MaxAttempts() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.maxAttempts
}

// ConsecutiveFailures returns the number of consecutive failures.
// Returns 0 if receiver is nil.
func (b *Backoff) ConsecutiveFailures() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.consecutiveFailures
}

// ShouldStop returns true if max attempts reached.
// Returns true if receiver is nil (fail-safe behavior).
//
// Reference: mediamtx-stream-manager.sh lines 2243-2246
func (b *Backoff) ShouldStop() bool {
	if b == nil {
		return true
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.attempts >= b.maxAttempts
}

// Reset resets the backoff to initial state.
// No-op if receiver is nil.
func (b *Backoff) Reset() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.currentDelay = b.initialDelay
	b.attempts = 0
	b.consecutiveFailures = 0
}

// Wait blocks for the current backoff delay.
// Returns immediately if receiver is nil.
//
// This is equivalent to: sleep ${RESTART_DELAY}
//
// Reference: mediamtx-stream-manager.sh line 2305
func (b *Backoff) Wait() {
	if b == nil {
		return
	}
	delay := b.CurrentDelay()
	time.Sleep(delay)
}

// WaitContext blocks for the current backoff delay or until context is cancelled.
// Returns nil immediately if receiver is nil.
//
// Returns:
//   - nil if wait completed or receiver is nil
//   - context error if context was cancelled
func (b *Backoff) WaitContext(ctx context.Context) error {
	if b == nil {
		return nil
	}
	delay := b.CurrentDelay()

	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
