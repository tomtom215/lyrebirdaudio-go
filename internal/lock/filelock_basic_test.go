//go:build linux

package lock

import (
	"os"
	"path/filepath"
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
