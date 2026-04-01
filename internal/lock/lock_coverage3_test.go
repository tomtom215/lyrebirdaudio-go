// SPDX-License-Identifier: MIT

//go:build linux

package lock

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestIsLockStaleNonExistentPath covers filelock.go:252-253 —
// the `os.IsNotExist(err) → return false, nil` branch when the lock path does
// not exist at all. A missing lock file is not stale; it simply isn't held.
func TestIsLockStaleNonExistentPath(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "nonexistent.lock")
	// File is never created.

	stale, err := isLockStale(lockPath, DefaultStaleThreshold)
	if err != nil {
		t.Errorf("isLockStale() unexpected error for non-existent path: %v", err)
	}
	if stale {
		t.Error("isLockStale() = true for non-existent path, want false")
	}
}

// TestIsLockStaleEmptyFile covers filelock.go:267-269 —
// the `pidStr == "" → return true, nil` branch when the lock file exists but
// is empty. An empty file cannot contain a valid PID, so the lock is stale.
func TestIsLockStaleEmptyFile(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "empty.lock")
	if err := os.WriteFile(lockPath, []byte{}, 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stale, err := isLockStale(lockPath, DefaultStaleThreshold)
	if err != nil {
		t.Errorf("isLockStale() unexpected error for empty file: %v", err)
	}
	if !stale {
		t.Error("isLockStale() = false for empty lock file, want true")
	}
}

// TestIsLockStaleInvalidPID covers filelock.go:272-275 —
// the `strconv.Atoi error → return true, nil` branch when the lock file
// contains non-numeric content. A non-integer PID is always stale.
func TestIsLockStaleInvalidPID(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "invalid.lock")
	if err := os.WriteFile(lockPath, []byte("not-a-pid\n"), 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stale, err := isLockStale(lockPath, DefaultStaleThreshold)
	if err != nil {
		t.Errorf("isLockStale() unexpected error for invalid PID: %v", err)
	}
	if !stale {
		t.Error("isLockStale() = false for invalid PID content, want true")
	}
}

// TestReleaseLockNotHeld covers filelock.go:197-199 —
// the `fl.file == nil → return error("lock not held")` branch.
// Release on a FileLock that was never acquired must return an error.
func TestReleaseLockNotHeld(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "never-held.lock")
	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}

	err = fl.Release()
	if err == nil {
		t.Error("Release() on unheld lock should return error, got nil")
	}
}

// TestAcquireContextRemovesStaleLock covers filelock.go:124-126 —
// the stale-lock removal branch inside AcquireContext. When the lock file
// contains a dead PID (99999999), isLockStale returns true and AcquireContext
// removes the stale file before attempting to acquire. The acquisition must
// then succeed and the file must contain the current process PID.
func TestAcquireContextRemovesStaleLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "stale.lock")

	// Write a stale lock file referencing a dead PID.
	if err := os.WriteFile(lockPath, []byte("99999999\n"), 0640); err != nil {
		t.Fatalf("WriteFile stale lock: %v", err)
	}

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}

	// AcquireContext must detect the stale lock, remove it, and succeed.
	if err := fl.AcquireContext(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("AcquireContext after stale lock: %v", err)
	}
	defer func() { _ = fl.Release() }()

	// The lock file must now contain the current PID (not the old dead PID).
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile after acquire: %v", err)
	}
	if !strings.Contains(string(data), strconv.Itoa(os.Getpid())) {
		t.Errorf("lock file should contain PID %d after stale removal, got %q",
			os.Getpid(), strings.TrimSpace(string(data)))
	}
}
