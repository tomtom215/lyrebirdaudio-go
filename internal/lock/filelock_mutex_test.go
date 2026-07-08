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

// TestAcquireMutualExclusionWithStaleFile verifies that flock alone guarantees
// mutual exclusion, including when a pre-existing empty ("stale") lock file is
// present. The previous implementation unlinked such a file and recreated it,
// which let two acquirers flock two different inodes at the same path and both
// believe they held the lock. This asserts the invariant that at most one
// holder exists at a time.
func TestAcquireMutualExclusionWithStaleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "device.lock")

	// Pre-seed an empty lock file — the "stale" case the old code unlinked.
	if err := os.WriteFile(path, []byte(""), 0640); err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}

	l1, err := NewFileLock(path)
	if err != nil {
		t.Fatalf("NewFileLock l1: %v", err)
	}
	l2, err := NewFileLock(path)
	if err != nil {
		t.Fatalf("NewFileLock l2: %v", err)
	}

	ctx := context.Background()

	if err := l1.AcquireContext(ctx, time.Second); err != nil {
		t.Fatalf("l1 failed to acquire over a stale file: %v", err)
	}

	// While l1 holds the lock, l2 must not be able to acquire it.
	if err := l2.AcquireContext(ctx, 300*time.Millisecond); err == nil {
		_ = l2.Release()
		_ = l1.Release()
		t.Fatal("l2 acquired the lock while l1 held it — mutual exclusion broken")
	}

	// After l1 releases, l2 must be able to acquire (recovery works without
	// unlinking the file).
	if err := l1.Release(); err != nil {
		t.Fatalf("l1 release: %v", err)
	}
	if err := l2.AcquireContext(ctx, time.Second); err != nil {
		t.Fatalf("l2 failed to acquire after l1 released: %v", err)
	}
	if err := l2.Release(); err != nil {
		t.Fatalf("l2 release: %v", err)
	}
}
