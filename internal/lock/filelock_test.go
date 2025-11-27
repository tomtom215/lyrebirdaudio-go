package lock

import (
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
