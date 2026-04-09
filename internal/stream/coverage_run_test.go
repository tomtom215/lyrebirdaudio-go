// SPDX-License-Identifier: MIT

package stream

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestManagerRunSuccessfulLongRun verifies that a process running longer than
// the success threshold resets backoff and restarts immediately.
func TestManagerRunSuccessfulLongRun(t *testing.T) {
	lockDir := t.TempDir()

	// Create a script that sleeps briefly (simulating a "long" run by using
	// a very short success threshold).
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	// Script that runs for 200ms then exits cleanly
	scriptContent := "#!/bin/sh\nsleep 0.2\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_long_run",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		// Use a very short success threshold so 200ms counts as "long"
		Backoff: NewBackoffWithThreshold(
			10*time.Millisecond,
			50*time.Millisecond,
			100*time.Millisecond, // 100ms threshold
			10,
		),
		Logger: slog.New(slog.NewTextHandler(devNull{}, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Cancel after a few cycles to stop the loop
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = mgr.Run(ctx)

	// Should be cancelled (not max attempts)
	if err == nil {
		t.Fatal("Run() should return error when context cancelled")
	}

	// Should have multiple attempts (restarts after successful runs)
	if mgr.Attempts() < 2 {
		t.Errorf("Attempts = %d, want >= 2 (should restart after successful long runs)", mgr.Attempts())
	}
}

// TestManagerRunShortCleanExit exercises the Run() path where FFmpeg exits
// with nil error but ran for less than the success threshold (lines 346-369).
// This is distinct from FFmpeg exiting with an error.
func TestManagerRunShortCleanExit(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()

	// Script that exits cleanly (exit 0) immediately.
	scriptPath := filepath.Join(scriptDir, "fast_exit.sh")
	scriptContent := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test_short_clean",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		// Script exits in < 1ms, threshold is 300s, so this is a "short clean exit".
		Backoff: NewBackoff(10*time.Millisecond, 50*time.Millisecond, 3),
		Logger:  slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = mgr.Run(ctx)
	if err == nil {
		t.Fatal("Run() should return error")
	}

	// Should have logged short-run failure event.
	output := logBuf.String()
	if !strings.Contains(output, "stream_short_run_failure") {
		t.Errorf("expected 'stream_short_run_failure' in logs, got:\n%s", output)
	}

	// Failures should be recorded (short clean exits are treated as failures).
	if mgr.Failures() == 0 {
		t.Error("Failures should be > 0 for short clean exits")
	}
}

// TestManagerRunShortCleanExitContextCancelDuringBackoff exercises the path
// where context is cancelled during backoff after a short clean exit.
func TestManagerRunShortCleanExitContextCancelDuringBackoff(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()

	scriptPath := filepath.Join(scriptDir, "fast_exit.sh")
	scriptContent := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_short_cancel",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(5*time.Second, 10*time.Second, 10), // Long backoff
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for first clean exit and backoff to start.
	time.Sleep(200 * time.Millisecond)

	// Cancel during the backoff wait.
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not complete quickly after cancel during backoff")
	}

	if mgr.State() != StateStopped {
		t.Errorf("State = %v, want StateStopped", mgr.State())
	}
}

// TestManagerRunContextDoneAtLoopTop exercises the ctx.Done() path at the
// top of the Run() main loop (lines 286-289). This requires context to be
// cancelled between iterations (after backoff, before next startFFmpeg).
func TestManagerRunContextDoneAtLoopTop(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()

	// Script that exits with error immediately.
	scriptPath := filepath.Join(scriptDir, "fail.sh")
	scriptContent := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_loop_top",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		// Very short backoff so we quickly get to the top of the loop.
		Backoff: NewBackoff(1*time.Millisecond, 5*time.Millisecond, 100),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Use a very short timeout - the context will expire between iterations.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = mgr.Run(ctx)

	// Should eventually return due to context timeout.
	if err == nil {
		t.Fatal("Run() should return error")
	}

	// The state should be stopped (context done at loop top sets StateStopped).
	state := mgr.State()
	if state != StateStopped && state != StateFailed {
		t.Errorf("State = %v, want StateStopped or StateFailed", state)
	}
}
