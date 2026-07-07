//go:build linux

package lock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestFileLockAcquireContextNormal verifies normal lock acquisition with context.
func TestFileLockAcquireContextNormal(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	ctx := context.Background()
	if err := lock.AcquireContext(ctx, 5*time.Second); err != nil {
		t.Fatalf("AcquireContext() error = %v", err)
	}

	// Verify lock file exists and contains our PID
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Lock file is empty, expected PID")
	}

	// Release lock
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

// TestFileLockAcquireContextCancelledBeforeAcquire verifies behavior when context is already cancelled.
func TestFileLockAcquireContextCancelledBeforeAcquire(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	start := time.Now()
	err = lock.AcquireContext(ctx, 30*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("AcquireContext() should fail with cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("AcquireContext() error = %v, want context.Canceled", err)
	}

	// Should fail very quickly (< 500ms)
	if elapsed > 500*time.Millisecond {
		t.Errorf("AcquireContext() took %v, expected < 500ms with cancelled context", elapsed)
	}
}

// TestFileLockAcquireContextCancelledDuringAcquire verifies context cancellation during acquisition.
func TestFileLockAcquireContextCancelledDuringAcquire(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// First lock holds the lock
	lock1, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock(1) error = %v", err)
	}
	defer func() { _ = lock1.Close() }()

	if err := lock1.Acquire(5 * time.Second); err != nil {
		t.Fatalf("lock1.Acquire() error = %v", err)
	}

	// Second lock tries to acquire with context that will be cancelled
	lock2, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock(2) error = %v", err)
	}
	defer func() { _ = lock2.Close() }()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after 200ms
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err = lock2.AcquireContext(ctx, 30*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("AcquireContext() should fail when context is cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("AcquireContext() error = %v, want context.Canceled", err)
	}

	// Should fail around 200-400ms (cancelled after 200ms + up to one tick)
	if elapsed < 150*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("AcquireContext() took %v, expected ~200-400ms", elapsed)
	}
}

// TestFileLockAcquireContextTimeout verifies timeout with valid context.
func TestFileLockAcquireContextTimeout(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// First lock holds the lock
	lock1, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock(1) error = %v", err)
	}
	defer func() { _ = lock1.Close() }()

	if err := lock1.Acquire(5 * time.Second); err != nil {
		t.Fatalf("lock1.Acquire() error = %v", err)
	}

	// Second lock tries to acquire with timeout (context won't cancel)
	lock2, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock(2) error = %v", err)
	}
	defer func() { _ = lock2.Close() }()

	ctx := context.Background()

	start := time.Now()
	err = lock2.AcquireContext(ctx, 500*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("AcquireContext() should fail due to timeout")
	}

	if !strings.Contains(err.Error(), "failed to acquire lock") {
		t.Errorf("AcquireContext() error = %v, want error containing 'failed to acquire lock'", err)
	}

	// Should timeout after ~500ms
	if elapsed < 400*time.Millisecond || elapsed > 1*time.Second {
		t.Errorf("AcquireContext() took %v, expected ~500ms", elapsed)
	}
}

// TestFileLockAcquireContextConcurrent verifies concurrent lock acquisition with context cancellation.
func TestFileLockAcquireContextConcurrent(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// First lock holds the lock
	lock1, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock(1) error = %v", err)
	}
	defer func() { _ = lock1.Close() }()

	if err := lock1.Acquire(5 * time.Second); err != nil {
		t.Fatalf("lock1.Acquire() error = %v", err)
	}

	// Launch multiple goroutines trying to acquire with cancelled contexts
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			lock, err := NewFileLock(lockPath)
			if err != nil {
				t.Errorf("Goroutine %d: NewFileLock() error = %v", id, err)
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			err = lock.AcquireContext(ctx, 30*time.Second)
			if err == nil {
				t.Errorf("Goroutine %d: AcquireContext() should fail with cancelled context", id)
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Goroutine %d: error = %v, want context.Canceled", id, err)
			}
		}(i)
	}

	wg.Wait()
}
