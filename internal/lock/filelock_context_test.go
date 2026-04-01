// SPDX-License-Identifier: MIT

//go:build linux

package lock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestAcquireContextPreCancelled covers the early-exit branch in AcquireContext
// (supervisor.go:116-121) where the context is already cancelled before the
// function begins. The initial `select { case <-ctx.Done(): ... }` fires and
// the function returns ctx.Err() immediately, before opening the lock file.
func TestAcquireContextPreCancelled(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before calling AcquireContext

	err = fl.AcquireContext(ctx, 1*time.Second)
	if err == nil {
		t.Error("expected error for pre-cancelled context, got nil")
		_ = fl.Release()
		return
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestAcquireContextCancelledDuringWait covers the ctx.Done branch inside the
// retry ticker loop (filelock.go:149-153). A holder goroutine holds the lock
// while the contender loops waiting; a context with a short deadline fires
// ctx.Done() mid-loop, causing AcquireContext to close the file and return
// ctx.Err() (context.DeadlineExceeded).
func TestAcquireContextCancelledDuringWait(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "held.lock")

	holder, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock holder: %v", err)
	}
	if err := holder.Acquire(5 * time.Second); err != nil {
		t.Fatalf("holder.Acquire: %v", err)
	}
	defer func() { _ = holder.Release() }()

	contender, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock contender: %v", err)
	}

	// Use a context that expires quickly; pass a long lock timeout so the
	// context fires before the lock timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	acquireErr := contender.AcquireContext(ctx, 10*time.Second)
	if acquireErr == nil {
		t.Error("expected context cancellation error, got nil")
		_ = contender.Release()
		return
	}
	if !errors.Is(acquireErr, context.DeadlineExceeded) && !errors.Is(acquireErr, context.Canceled) {
		t.Errorf("expected context error, got: %v", acquireErr)
	}
}
