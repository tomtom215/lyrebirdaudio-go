// SPDX-License-Identifier: MIT

//go:build linux

package lock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIsLockStaleReadFileError covers the isLockStale branch where the lock
// file exists but cannot be read. We simulate this by placing a directory at
// the lock path — os.ReadFile on a directory returns an error.
func TestIsLockStaleReadFileError(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a directory with the lock file name. Stat will succeed (it exists),
	// but ReadFile will fail (is a directory).
	lockPath := filepath.Join(tmpDir, "test.lock")
	if err := os.Mkdir(lockPath, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	stale, err := isLockStale(lockPath, DefaultStaleThreshold)
	// ReadFile error → return true (assume stale)
	if !stale {
		t.Errorf("expected isLockStale=true when ReadFile fails, got false (err=%v)", err)
	}
}

// TestReleaseOnManuallyClosedFile covers the Flock error path in Release.
// We close fl.file externally; after close, Fd() returns an invalid descriptor
// so syscall.Flock returns EBADF, triggering the error branch.
func TestReleaseOnManuallyClosedFile(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}

	if err := fl.AcquireContext(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("AcquireContext: %v", err)
	}

	// Close the underlying file directly (without going through Release).
	// This invalidates the file descriptor.
	fl.mu.Lock()
	underlyingFile := fl.file
	fl.mu.Unlock()

	if underlyingFile == nil {
		t.Fatal("fl.file is nil after acquire")
	}
	if err := underlyingFile.Close(); err != nil {
		t.Fatalf("direct Close: %v", err)
	}

	// Now Release should fail at Flock (EBADF) or at file.Close (already closed).
	releaseErr := fl.Release()
	if releaseErr == nil {
		t.Error("expected Release to return an error after fd closed externally")
	}
}

// TestAcquireContextOpenFileFailureNonRoot covers the os.OpenFile error path
// in AcquireContext. On non-root systems, removing write permission from the
// lock directory causes OpenFile to fail.
func TestAcquireContextOpenFileFailureNonRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission restriction not meaningful as root")
	}

	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "locks")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatal(err)
	}

	fl, err := NewFileLock(filepath.Join(lockDir, "test.lock"))
	if err != nil {
		t.Fatalf("NewFileLock: %v", err)
	}

	// Remove write permission so os.OpenFile fails.
	if err := os.Chmod(lockDir, 0550); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(lockDir, 0750) }()

	err = fl.AcquireContext(context.Background(), 100*time.Millisecond)
	if err == nil {
		t.Error("expected error opening lock file in non-writable directory")
		_ = fl.Release()
	}
}

// TestAcquireContextTimeoutExpiredPath verifies the timeout-expired branch
// inside the ticker loop (time.Now().After(deadline)) by holding the lock
// in another goroutine so AcquireContext retries until timeout.
func TestAcquireContextTimeoutExpiredPath(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "held.lock")

	holder, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock holder: %v", err)
	}
	if err := holder.Acquire(5 * time.Second); err != nil {
		t.Fatalf("holder.Acquire: %v", err)
	}
	defer func() { _ = holder.Release() }()

	// Contender tries to acquire with a very short timeout — it will loop
	// through the ticker at least once before the deadline fires.
	contender, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock contender: %v", err)
	}
	ctx := context.Background()
	err = contender.AcquireContext(ctx, 150*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error, got nil")
		_ = contender.Release()
	}
}
