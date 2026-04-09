//go:build linux

package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

// TestIsLockStaleStatError verifies isLockStale returns false (safe) when os.Stat
// returns a non-NotExist error (e.g. permission denied on parent directory).
func TestIsLockStaleStatPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission test not meaningful as root")
	}
	tmpDir := t.TempDir()
	lockedDir := filepath.Join(tmpDir, "locked")
	if err := os.MkdirAll(lockedDir, 0750); err != nil {
		t.Fatal(err)
	}
	// Write a lock file inside the locked dir.
	lockPath := filepath.Join(lockedDir, "test.lock")
	if err := os.WriteFile(lockPath, []byte("12345"), 0640); err != nil {
		t.Fatal(err)
	}
	// Remove all permissions on the parent → Stat will return permission denied.
	if err := os.Chmod(lockedDir, 0000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(lockedDir, 0750) }()

	stale, err := isLockStale(lockPath, DefaultStaleThreshold)
	// Stat failure → assume not stale (safe default).
	if stale {
		t.Error("expected isLockStale to return false on stat error")
	}
	_ = err // err may or may not be non-nil depending on OS; stale=false is the key check
}
