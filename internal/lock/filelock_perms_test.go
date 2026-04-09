//go:build linux

package lock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
