//go:build linux

package lock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

// TestNewFileLockMkdirAllFailure verifies NewFileLock returns an error when the
// lock directory cannot be created (e.g. a file exists at the parent path).
func TestNewFileLockMkdirAllFailure(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a regular file where the lock parent directory would need to be.
	parentAsFile := filepath.Join(tmpDir, "not-a-dir")
	if err := os.WriteFile(parentAsFile, []byte("blocker"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Try to create a lock whose parent directory is the file above.
	lockPath := filepath.Join(parentAsFile, "subdir", "test.lock")
	_, err := NewFileLock(lockPath)
	if err == nil {
		t.Error("expected error when parent path is a file, got nil")
	}
}

// TestAcquireContextOpenFileFailure verifies AcquireContext returns an error when
// the lock file cannot be opened due to directory permission denial.
func TestAcquireContextOpenFileFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission test not meaningful as root")
	}
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "locks")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatal(err)
	}
	lock, err := NewFileLock(filepath.Join(lockDir, "test.lock"))
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}
	// Remove write permission from the lock directory — OpenFile will fail.
	if err := os.Chmod(lockDir, 0550); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(lockDir, 0750) }() // restore for cleanup

	err = lock.AcquireContext(context.Background(), 100*time.Millisecond)
	if err == nil {
		t.Error("expected error acquiring lock in non-writable directory")
		// Clean up in case it somehow succeeded.
		_ = lock.Release()
	}
}
