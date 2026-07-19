// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunSupervisedRestartsAfterPanic verifies a panicking task is recovered,
// logged, and restarted — not allowed to crash the process — and that it stops
// cleanly once ctx is cancelled.
func TestRunSupervisedRestartsAfterPanic(t *testing.T) {
	// Shorten the restart delay so the test is fast; restore afterwards.
	orig := superviseRestartDelay
	superviseRestartDelay = time.Millisecond
	t.Cleanup(func() { superviseRestartDelay = orig })

	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logMu{w: &logBuf}, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithCancel(context.Background())

	var runs atomic.Int32
	done := make(chan struct{})
	go func() {
		runSupervised(ctx, logger, "flaky-task", func() {
			n := runs.Add(1)
			if n <= 3 {
				panic("boom")
			}
			// 4th run onward: block until shutdown like a real loop.
			<-ctx.Done()
		})
		close(done)
	}()

	// Wait for it to survive the panics and reach the blocking run.
	deadline := time.After(2 * time.Second)
	for runs.Load() < 4 {
		select {
		case <-deadline:
			t.Fatalf("task did not restart past panics; runs=%d", runs.Load())
		case <-time.After(time.Millisecond):
		}
	}

	// Cancelling ctx must make runSupervised return (no restart after shutdown).
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runSupervised did not return after ctx cancel")
	}

	if got := runs.Load(); got < 4 {
		t.Errorf("task ran %d times, want >= 4 (3 panics + 1 stable run)", got)
	}
	if ls := logBuf.String(); !strings.Contains(ls, "flaky-task") || !strings.Contains(ls, "panicked") {
		t.Errorf("expected panic to be logged with task name, got: %q", ls)
	}
}

// TestRunSupervisedExitsOnCancelWithoutPanic verifies the normal path: a task
// that blocks until ctx is cancelled returns exactly once and is not restarted.
func TestRunSupervisedExitsOnCancelWithoutPanic(t *testing.T) {
	orig := superviseRestartDelay
	superviseRestartDelay = time.Millisecond
	t.Cleanup(func() { superviseRestartDelay = orig })

	ctx, cancel := context.WithCancel(context.Background())

	var runs atomic.Int32
	done := make(chan struct{})
	go func() {
		runSupervised(ctx, nil, "clean-task", func() {
			runs.Add(1)
			<-ctx.Done()
		})
		close(done)
	}()

	// Give it a moment to enter the blocking run, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runSupervised did not return after ctx cancel")
	}
	if got := runs.Load(); got != 1 {
		t.Errorf("clean task ran %d times, want exactly 1 (no spurious restart)", got)
	}
}

// logMu is a tiny mutex-guarded io.Writer so the slog handler is safe to use
// from the supervised goroutine while the test goroutine reads the buffer.
type logMu struct {
	mu sync.Mutex
	w  *strings.Builder
}

func (l *logMu) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
