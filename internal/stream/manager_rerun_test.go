// SPDX-License-Identifier: MIT

package stream

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/lock"
)

// TestRunReopensLogWriterAfterClose verifies that a Manager whose Run returned
// an error and was then Close()d — the exact sequence streamService performs
// before the supervisor re-runs the same service — recreates its FFmpeg log
// writer on the next Run.
//
// Field scenario: another process holds the device lock at startup, so the
// first Run fails with a lock-acquire error and streamService closes the
// manager. suture then re-runs the service; the lock is free now and FFmpeg
// streams happily — but without reopening the rotating writer, its stderr is
// silently discarded FOREVER on an unattended device, leaving no logs for any
// later failure.
func TestRunReopensLogWriterAfterClose(t *testing.T) {
	lockDir := t.TempDir()
	logDir := t.TempDir()
	scriptDir := t.TempDir()

	scriptPath := filepath.Join(scriptDir, "mock_ffmpeg.sh")
	script := "#!/bin/sh\necho 'REOPEN_MARKER' >&2\nsleep 60\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_rerun",
		ALSADevice:   "dummy",
		StreamName:   "test_rerun",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		LogDir:       logDir,
		FFmpegPath:   scriptPath,
		StopTimeout:  time.Second,
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer func() { _ = mgr.Close() }()

	// Hold the device lock externally so the first Run fails at lock acquire,
	// like a competing process at startup.
	external, err := lock.NewFileLock(filepath.Join(lockDir, "test_rerun.lock"))
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	if err := external.Acquire(0); err != nil {
		t.Fatalf("external Acquire() error = %v", err)
	}

	runCtx, runCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	err = mgr.Run(runCtx)
	runCancel()
	if err == nil {
		t.Fatal("first Run() = nil error, want lock-acquire failure")
	}

	// streamService.Run closes the manager after EVERY Run return.
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := external.Release(); err != nil {
		t.Fatalf("external Release() error = %v", err)
	}

	// Second Run: the supervisor re-runs the same service. FFmpeg stderr must
	// land in the rotating log again.
	ctx2, cancel2 := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Run(ctx2) }()

	logPath := filepath.Join(logDir, "ffmpeg-test_rerun.log")
	deadline := time.Now().Add(5 * time.Second)
	var sawMarker bool
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(logPath); err == nil && strings.Contains(string(data), "REOPEN_MARKER") {
			sawMarker = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel2()
	<-done

	if !sawMarker {
		t.Fatal("FFmpeg stderr never reached the log file after Close()+re-Run: log writer was not reopened")
	}
}
