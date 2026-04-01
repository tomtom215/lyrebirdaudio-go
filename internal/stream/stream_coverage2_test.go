// SPDX-License-Identifier: MIT

package stream

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRotatingWriterCloseNoFile covers logrotate.go:178 — the `return nil`
// branch in Close() when w.file is nil (no writes have occurred and the file
// was never opened via the public API). We construct the RotatingWriter
// manually to bypass NewRotatingWriter (which always opens the file).
func TestRotatingWriterCloseNoFile(t *testing.T) {
	w := &RotatingWriter{
		path:     filepath.Join(t.TempDir(), "unused.log"),
		maxSize:  DefaultMaxLogSize,
		maxFiles: DefaultMaxLogFiles,
		// file is intentionally nil
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close() on never-opened writer unexpected error: %v", err)
	}
}

// TestAcquireLockNewFileLockError covers manager.go:293-295 — the
// lock.NewFileLock error path in acquireLock. A regular file is placed at
// cfg.LockDir so that os.MkdirAll(lockDir) returns ENOTDIR (not a directory).
// This fails even as root, providing root-safe coverage.
func TestAcquireLockNewFileLockError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular file at the path that would need to be the lock directory.
	blockedDir := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(blockedDir, []byte("file"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName: "test-device",
		ALSADevice: "hw:0,0",
		StreamName: "test",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    blockedDir, // regular file, not a directory
		FFmpegPath: "/fake/ffmpeg",
		Backoff:    NewBackoff(10*time.Millisecond, 100*time.Millisecond, 3),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	err = mgr.acquireLock(t.Context())
	if err == nil {
		t.Error("acquireLock() expected error when lockDir is a file, got nil")
		_ = mgr.releaseLock
	}
}

// TestReleaseLockNilLock covers manager.go:313 — the `if m.lock != nil` guard
// in releaseLock when lock is nil. This is the "no-op when not holding lock"
// path; it should complete without error or panic.
func TestReleaseLockNilLock(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test-device",
		ALSADevice: "hw:0,0",
		StreamName: "test",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    t.TempDir(),
		FFmpegPath: "/fake/ffmpeg",
		Backoff:    NewBackoff(10*time.Millisecond, 100*time.Millisecond, 3),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// mgr.lock is nil (never acquired); releaseLock should be a no-op.
	mgr.releaseLock() // should not panic
}
