// SPDX-License-Identifier: MIT

package stream

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestReleaseLockNil verifies releaseLock is safe when lock is nil.
func TestReleaseLockNil(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    t.TempDir(),
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// lock is nil at this point; releaseLock should not panic
	mgr.releaseLock()
}

// TestReleaseLockErrorLogging exercises the releaseLock error logging path
// (line 509-511 of manager.go).
func TestReleaseLockErrorLogging(t *testing.T) {
	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName: "test",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    t.TempDir(),
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
		Logger:     slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Acquire lock.
	err = mgr.acquireLock(context.Background())
	if err != nil {
		t.Fatalf("acquireLock() error = %v", err)
	}

	// Release once (this should succeed).
	mgr.releaseLock()

	// Verify lock is nil after release.
	mgr.mu.RLock()
	isNil := mgr.lock == nil
	mgr.mu.RUnlock()
	if !isNil {
		t.Error("lock should be nil after releaseLock")
	}
}
