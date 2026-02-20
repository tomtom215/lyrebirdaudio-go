//go:build linux

package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestFileLockAcquireRelease verifies basic lock acquisition and release.
//
// Reference: mediamtx-stream-manager.sh acquire_lock() lines 837-906
func TestFileLockAcquireRelease(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// Acquire lock
	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	if err := lock.Acquire(5 * time.Second); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	// Verify lock file exists and contains our PID
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Should contain PID
	if len(data) == 0 {
		t.Error("Lock file is empty, expected PID")
	}

	// Release lock
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

// TestFileLockConcurrentAcquisition verifies that only one process can hold the lock.
func TestFileLockConcurrentAcquisition(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// First lock acquires successfully
	lock1, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock1.Close() }()

	if err := lock1.Acquire(5 * time.Second); err != nil {
		t.Fatalf("lock1.Acquire() error = %v", err)
	}

	// Second lock should timeout
	lock2, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock2.Close() }()

	start := time.Now()
	err = lock2.Acquire(1 * time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("lock2.Acquire() should have failed due to lock1 holding lock")
	}

	// Should have waited ~1 second
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("Acquire() waited %v, expected ~1s", elapsed)
	}

	// Release first lock
	if err := lock1.Release(); err != nil {
		t.Fatalf("lock1.Release() error = %v", err)
	}

	// Now second lock should succeed
	if err := lock2.Acquire(1 * time.Second); err != nil {
		t.Fatalf("lock2.Acquire() after lock1 release error = %v", err)
	}
}

// TestFileLockStaleLockRemoval verifies stale lock detection and removal.
//
// Reference: mediamtx-stream-manager.sh is_lock_stale() lines 765-805
func TestFileLockStaleLockRemoval(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// Create fake stale lock file with non-existent PID
	stalePID := 99999 // Very unlikely to exist
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", stalePID)), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Set old modification time (simulating stale lock)
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	// Attempt to acquire lock - should remove stale lock and succeed
	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	if err := lock.Acquire(5 * time.Second); err != nil {
		t.Fatalf("Acquire() with stale lock error = %v", err)
	}

	// Lock file should now contain our PID
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	currentPID := os.Getpid()
	if !strings.Contains(string(data), strconv.Itoa(currentPID)) {
		t.Errorf("Lock file contains %q, expected PID %d", string(data), currentPID)
	}
}

// TestFileLockIsStale verifies stale lock detection logic.
func TestFileLockIsStale(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(path string) error
		wantStale bool
	}{
		{
			name: "no lock file",
			setupFunc: func(path string) error {
				// Don't create file
				return nil
			},
			wantStale: false, // No file = not stale
		},
		{
			name: "empty lock file",
			setupFunc: func(path string) error {
				return os.WriteFile(path, []byte(""), 0644)
			},
			wantStale: true, // Invalid PID
		},
		{
			name: "invalid PID",
			setupFunc: func(path string) error {
				return os.WriteFile(path, []byte("invalid"), 0644)
			},
			wantStale: true,
		},
		{
			name: "non-existent PID",
			setupFunc: func(path string) error {
				return os.WriteFile(path, []byte("99999"), 0644)
			},
			wantStale: true, // Process doesn't exist
		},
		{
			name: "current process PID",
			setupFunc: func(path string) error {
				pid := os.Getpid()
				return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
			},
			wantStale: false, // Our own process is running
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockPath := filepath.Join(t.TempDir(), "test.lock")

			if err := tt.setupFunc(lockPath); err != nil {
				t.Fatalf("setupFunc() error = %v", err)
			}

			stale, err := isLockStale(lockPath, 300*time.Second)
			if err != nil && tt.wantStale {
				// Errors often indicate stale locks
				return
			}

			if stale != tt.wantStale {
				t.Errorf("isLockStale() = %v, want %v", stale, tt.wantStale)
			}
		})
	}
}

// TestFileLockConcurrentGoroutines verifies thread safety with multiple goroutines.
func TestFileLockConcurrentGoroutines(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	const numGoroutines = 10
	const iterations = 5

	var (
		wg           sync.WaitGroup
		successCount int32 // Atomic counter
	)

	// Start multiple goroutines trying to acquire the same lock
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				lock, err := NewFileLock(lockPath)
				if err != nil {
					t.Logf("Goroutine %d: NewFileLock() error = %v", id, err)
					continue
				}

				// Try to acquire with short timeout
				if err := lock.Acquire(100 * time.Millisecond); err == nil {
					// Successfully acquired
					atomic.AddInt32(&successCount, 1)

					// Hold lock briefly
					time.Sleep(10 * time.Millisecond)

					// Release
					if err := lock.Release(); err != nil {
						t.Logf("Goroutine %d: Release() error = %v", id, err)
					}
				}

				_ = lock.Close()
			}
		}(i)
	}

	wg.Wait()

	// At least some goroutines should have succeeded
	if successCount == 0 {
		t.Error("No goroutines successfully acquired lock")
	}

	t.Logf("Successful lock acquisitions: %d / %d attempts", successCount, numGoroutines*iterations)
}

// TestFileLockRaceDetector runs with -race flag to detect race conditions.
func TestFileLockRaceDetector(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock1, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock1.Close() }()

	lock2, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock2.Close() }()

	var wg sync.WaitGroup

	// Goroutine 1: Acquire and release repeatedly
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := lock1.Acquire(100 * time.Millisecond); err == nil {
				time.Sleep(5 * time.Millisecond)
				_ = lock1.Release()
			}
		}
	}()

	// Goroutine 2: Acquire and release repeatedly
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := lock2.Acquire(100 * time.Millisecond); err == nil {
				time.Sleep(5 * time.Millisecond)
				_ = lock2.Release()
			}
		}
	}()

	wg.Wait()
}

// TestFileLockErrorPaths tests error conditions.
func TestFileLockErrorPaths(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		_, err := NewFileLock("")
		if err == nil {
			t.Error("NewFileLock(\"\") should return error")
		}
		if err != nil && err.Error() != "lock path cannot be empty" {
			t.Errorf("Error message = %q, want \"lock path cannot be empty\"", err.Error())
		}
	})

	t.Run("release without acquire", func(t *testing.T) {
		lockPath := filepath.Join(t.TempDir(), "test.lock")
		lock, err := NewFileLock(lockPath)
		if err != nil {
			t.Fatalf("NewFileLock() error = %v", err)
		}

		// Release without acquiring should return error
		err = lock.Release()
		if err == nil {
			t.Error("Release() without acquire should return error")
		}
	})

	t.Run("double release", func(t *testing.T) {
		lockPath := filepath.Join(t.TempDir(), "test.lock")
		lock, err := NewFileLock(lockPath)
		if err != nil {
			t.Fatalf("NewFileLock() error = %v", err)
		}

		err = lock.Acquire(1 * time.Second)
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}

		// First release
		err = lock.Release()
		if err != nil {
			t.Errorf("First Release() error = %v", err)
		}

		// Second release should return error (lock not held)
		err = lock.Release()
		if err == nil {
			t.Error("Second Release() should return error")
		}
	})

	t.Run("close without acquire", func(t *testing.T) {
		lockPath := filepath.Join(t.TempDir(), "test.lock")
		lock, err := NewFileLock(lockPath)
		if err != nil {
			t.Fatalf("NewFileLock() error = %v", err)
		}

		err = lock.Close()
		if err != nil {
			t.Errorf("Close() without acquire error = %v", err)
		}
	})

	t.Run("acquire timeout", func(t *testing.T) {
		lockPath := filepath.Join(t.TempDir(), "test.lock")

		lock1, err := NewFileLock(lockPath)
		if err != nil {
			t.Fatalf("NewFileLock(1) error = %v", err)
		}
		defer func() { _ = lock1.Close() }()

		// Acquire lock with first instance
		err = lock1.Acquire(1 * time.Second)
		if err != nil {
			t.Fatalf("Acquire(1) error = %v", err)
		}
		defer func() { _ = lock1.Release() }()

		// Try to acquire with second instance - should timeout
		lock2, err := NewFileLock(lockPath)
		if err != nil {
			t.Fatalf("NewFileLock(2) error = %v", err)
		}
		defer func() { _ = lock2.Close() }()

		err = lock2.Acquire(100 * time.Millisecond)
		if err == nil {
			t.Error("Acquire(2) should timeout when lock is held")
		}
	})
}

// TestFileLockInvalidPath tests lock creation with invalid paths.
func TestFileLockInvalidPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "valid path",
			path:    filepath.Join(t.TempDir(), "valid.lock"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFileLock(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFileLock() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestFileLockAcquireZeroTimeout tests immediate lock acquisition attempt.
func TestFileLockAcquireZeroTimeout(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// First lock acquires with zero timeout
	lock1, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock1.Close() }()

	if err := lock1.Acquire(0); err != nil {
		t.Fatalf("First lock.Acquire(0) error = %v", err)
	}

	// Second lock should fail immediately with zero timeout
	lock2, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock2.Close() }()

	start := time.Now()
	err = lock2.Acquire(0)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Second lock.Acquire(0) should fail immediately")
	}

	// Should fail almost immediately (within 200ms to account for retry loop)
	if elapsed > 200*time.Millisecond {
		t.Errorf("Acquire(0) took %v, expected immediate failure", elapsed)
	}
}

// TestFileLockStaleOldAgeAliveProcess is the C-1 regression test.
//
// A lock held by a running process must NEVER be considered stale, even when
// the lock file's mtime exceeds DefaultStaleThreshold.  The original bug ran
// an unconditional age check after signal(0) confirmed the process was alive,
// which caused lock theft for any stream running longer than ~5 minutes.
func TestFileLockStaleOldAgeAliveProcess(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// Write our own PID — this process is definitely alive.
	pid := os.Getpid()
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Set mtime 24 hours in the past — far beyond DefaultStaleThreshold (300s).
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	// Must NOT be stale: signal(0) confirms the process is alive.
	stale, err := isLockStale(lockPath, 300*time.Second)
	if err != nil {
		t.Fatalf("isLockStale() error = %v", err)
	}
	if stale {
		t.Error("C-1 regression: lock held by a live process must not be stale regardless of mtime")
	}
}

// TestFileLockStaleDeadProcessOldAge verifies that a lock owned by a dead PID
// is always stale.
func TestFileLockStaleDeadProcessOldAge(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// PID 99999 is very unlikely to exist on any test system.
	if err := os.WriteFile(lockPath, []byte("99999\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	oldTime := time.Now().Add(-400 * time.Second)
	if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	stale, err := isLockStale(lockPath, 300*time.Second)
	if err != nil {
		t.Fatalf("isLockStale() error = %v", err)
	}
	if !stale {
		t.Error("Lock with a dead PID must always be stale")
	}
}

// TestFileLockPIDZero tests handling of PID 0 in lock file.
func TestFileLockPIDZero(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// Create lock file with PID 0
	if err := os.WriteFile(lockPath, []byte("0\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// PID 0 doesn't exist as a normal process, should be stale
	stale, err := isLockStale(lockPath, 300*time.Second)
	if err != nil {
		t.Logf("isLockStale() returned error (acceptable): %v", err)
	}

	// Either stale=true or error is acceptable
	if !stale && err == nil {
		t.Error("Lock with PID 0 should be considered stale or return error")
	}
}

// TestFileLockMultipleReleases tests calling Release multiple times.
func TestFileLockMultipleReleases(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")
	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}

	// Acquire lock
	if err := lock.Acquire(1 * time.Second); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	// First release should succeed
	if err := lock.Release(); err != nil {
		t.Errorf("First Release() error = %v", err)
	}

	// Second release should fail
	if err := lock.Release(); err == nil {
		t.Error("Second Release() should return error")
	}

	// Third release should also fail
	if err := lock.Release(); err == nil {
		t.Error("Third Release() should return error")
	}
}

// TestFileLockCloseIdempotent tests that Close() is idempotent.
func TestFileLockCloseIdempotent(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")
	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}

	// Close without acquiring should be OK
	if err := lock.Close(); err != nil {
		t.Errorf("Close() without acquire error = %v", err)
	}

	// Second close should also be OK
	if err := lock.Close(); err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

// TestFileLockAcquireAfterClose tests acquiring after close.
func TestFileLockAcquireAfterClose(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")
	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}

	// Acquire and close
	if err := lock.Acquire(1 * time.Second); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to acquire again - should fail (lock released)
	// Create new lock instance to try again
	lock2, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock(2) error = %v", err)
	}
	defer func() { _ = lock2.Close() }()

	// Should succeed since first lock was released
	if err := lock2.Acquire(1 * time.Second); err != nil {
		t.Errorf("Acquire after close should succeed: %v", err)
	}
}

// TestFileLockNegativePID tests handling of negative PID in lock file.
func TestFileLockNegativePID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	// Create lock file with negative PID
	if err := os.WriteFile(lockPath, []byte("-1\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Should be considered stale (invalid PID)
	stale, err := isLockStale(lockPath, 300*time.Second)
	if err != nil {
		t.Logf("isLockStale() returned error (acceptable): %v", err)
	}

	if !stale && err == nil {
		t.Error("Lock with negative PID should be stale or error")
	}
}

// TestFileLockLargeTimeout tests lock acquisition with very large timeout.
func TestFileLockLargeTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test with large timeout in short mode")
	}

	lockPath := filepath.Join(t.TempDir(), "test.lock")
	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	// Acquire with very large timeout (1 hour)
	// Should succeed immediately since lock is available
	start := time.Now()
	if err := lock.Acquire(1 * time.Hour); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	elapsed := time.Since(start)

	// Should be very fast (< 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Acquire with no contention took %v, expected < 100ms", elapsed)
	}
}

// BenchmarkFileLockAcquireRelease measures lock acquisition performance.
func BenchmarkFileLockAcquireRelease(b *testing.B) {
	lockPath := filepath.Join(b.TempDir(), "bench.lock")

	lock, err := NewFileLock(lockPath)
	if err != nil {
		b.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lock.Acquire(5 * time.Second); err != nil {
			b.Fatalf("Acquire() error = %v", err)
		}
		if err := lock.Release(); err != nil {
			b.Fatalf("Release() error = %v", err)
		}
	}
}

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

	if err != context.Canceled {
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

	if err != context.Canceled {
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
			if err != context.Canceled {
				t.Errorf("Goroutine %d: error = %v, want context.Canceled", id, err)
			}
		}(i)
	}

	wg.Wait()
}

// TestFileLockConcurrentAcquireReleaseSingleInstance exercises concurrent Acquire/Release
// on a single FileLock instance to verify that fl.file is protected by a mutex.
// This test is designed to trigger the race detector if mutex protection is missing.
func TestFileLockConcurrentAcquireReleaseSingleInstance(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = fl.Close() }()

	const goroutines = 4
	const iterations = 20

	var wg sync.WaitGroup
	var successCount int32

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := fl.Acquire(200 * time.Millisecond); err == nil {
					atomic.AddInt32(&successCount, 1)
					time.Sleep(time.Millisecond)
					_ = fl.Release()
				}
			}
		}()
	}

	wg.Wait()

	// At least some acquisitions should succeed
	if successCount == 0 {
		t.Error("No goroutines successfully acquired the lock")
	}
	t.Logf("Successful acquisitions: %d / %d attempts", successCount, goroutines*iterations)
}

// TestFileLockConcurrentAcquireContextReleaseSingleInstance exercises concurrent
// AcquireContext/Release on a single FileLock instance to verify thread safety.
func TestFileLockConcurrentAcquireContextReleaseSingleInstance(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = fl.Close() }()

	const goroutines = 4
	const iterations = 20

	ctx := context.Background()

	var wg sync.WaitGroup
	var successCount int32

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := fl.AcquireContext(ctx, 200*time.Millisecond); err == nil {
					atomic.AddInt32(&successCount, 1)
					time.Sleep(time.Millisecond)
					_ = fl.Release()
				}
			}
		}()
	}

	wg.Wait()

	if successCount == 0 {
		t.Error("No goroutines successfully acquired the lock")
	}
	t.Logf("Successful acquisitions: %d / %d attempts", successCount, goroutines*iterations)
}

// TestFileLockConcurrentCloseRelease exercises concurrent Close and Release
// on a single FileLock instance to verify mutex protection of fl.file reads.
func TestFileLockConcurrentCloseRelease(t *testing.T) {
	const iterations = 20

	for i := 0; i < iterations; i++ {
		lockPath := filepath.Join(t.TempDir(), fmt.Sprintf("test-%d.lock", i))

		fl, err := NewFileLock(lockPath)
		if err != nil {
			t.Fatalf("NewFileLock() error = %v", err)
		}

		if err := fl.Acquire(1 * time.Second); err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		// One goroutine calls Release, the other calls Close
		go func() {
			defer wg.Done()
			_ = fl.Release()
		}()

		go func() {
			defer wg.Done()
			_ = fl.Close()
		}()

		wg.Wait()
	}
}

// TestFileLockDirectoryPermissions verifies SEC-2: lock directory is created with 0750.
func TestFileLockDirectoryPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "newlockdir")
	lockPath := filepath.Join(lockDir, "test.lock")

	_, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}

	info, err := os.Stat(lockDir)
	if err != nil {
		t.Fatalf("Stat lock dir error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0750 {
		t.Errorf("lock directory permissions = %04o, want 0750", perm)
	}
}

// TestFileLockFilePermissions verifies SEC-2: lock file is created with 0640.
func TestFileLockFilePermissions(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	if err := lock.Acquire(5 * time.Second); err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Stat lock file error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0640 {
		t.Errorf("lock file permissions = %04o, want 0640", perm)
	}
}

// TestFileLockFilePermissionsContext verifies SEC-2: lock file via AcquireContext is 0640.
func TestFileLockFilePermissionsContext(t *testing.T) {
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

	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Stat lock file error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0640 {
		t.Errorf("lock file permissions = %04o, want 0640", perm)
	}
}

// BenchmarkFileLockAcquireContextRelease measures context-aware lock acquisition performance.
func BenchmarkFileLockAcquireContextRelease(b *testing.B) {
	lockPath := filepath.Join(b.TempDir(), "bench.lock")

	lock, err := NewFileLock(lockPath)
	if err != nil {
		b.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lock.AcquireContext(ctx, 5*time.Second); err != nil {
			b.Fatalf("AcquireContext() error = %v", err)
		}
		if err := lock.Release(); err != nil {
			b.Fatalf("Release() error = %v", err)
		}
	}
}
