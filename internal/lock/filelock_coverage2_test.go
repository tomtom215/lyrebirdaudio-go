// SPDX-License-Identifier: MIT

package lock

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsLockStaleStatNonEnoentError covers filelock.go:255-257 —
// the `return false, err` branch when os.Stat returns an error that is NOT
// os.IsNotExist (e.g., EACCES). This is triggered by placing the lock file
// inside a directory whose search permission has been removed (mode 0),
// making the path inaccessible without triggering ENOENT.
//
// Root bypasses permission checks, so the test is skipped when running as root.
func TestIsLockStaleStatNonEnoentError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test permission-denied stat as root")
	}

	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, "locks")
	if err := os.Mkdir(locksDir, 0750); err != nil {
		t.Fatalf("Mkdir locksDir: %v", err)
	}

	// Create the lock file inside locksDir.
	lockPath := filepath.Join(locksDir, "device.lock")
	if err := os.WriteFile(lockPath, []byte("12345\n"), 0640); err != nil {
		t.Fatalf("WriteFile lockPath: %v", err)
	}

	// Remove all permissions from locksDir so os.Stat(lockPath) returns EACCES,
	// which is not os.IsNotExist — triggering the `return false, err` branch.
	if err := os.Chmod(locksDir, 0); err != nil {
		t.Fatalf("Chmod locksDir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locksDir, 0750) })

	stale, err := isLockStale(lockPath, DefaultStaleThreshold)
	// EACCES is not os.IsNotExist; the function returns (false, err).
	if err == nil {
		t.Error("isLockStale() expected non-nil error for EACCES stat, got nil")
	}
	if stale {
		t.Error("isLockStale() returned stale=true for EACCES stat, want false")
	}
}
