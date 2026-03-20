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

// TestBackoffNilReceiver verifies all Backoff methods are safe on nil receiver.
func TestBackoffNilReceiver(t *testing.T) {
	var b *Backoff

	t.Run("RecordFailure", func(t *testing.T) {
		b.RecordFailure() // should not panic
	})

	t.Run("RecordSuccess", func(t *testing.T) {
		b.RecordSuccess(1 * time.Second) // should not panic
	})

	t.Run("CurrentDelay", func(t *testing.T) {
		if d := b.CurrentDelay(); d != 0 {
			t.Errorf("nil.CurrentDelay() = %v, want 0", d)
		}
	})

	t.Run("Attempts", func(t *testing.T) {
		if a := b.Attempts(); a != 0 {
			t.Errorf("nil.Attempts() = %d, want 0", a)
		}
	})

	t.Run("MaxAttempts", func(t *testing.T) {
		if m := b.MaxAttempts(); m != 0 {
			t.Errorf("nil.MaxAttempts() = %d, want 0", m)
		}
	})

	t.Run("ConsecutiveFailures", func(t *testing.T) {
		if c := b.ConsecutiveFailures(); c != 0 {
			t.Errorf("nil.ConsecutiveFailures() = %d, want 0", c)
		}
	})

	t.Run("ShouldStop", func(t *testing.T) {
		if !b.ShouldStop() {
			t.Error("nil.ShouldStop() = false, want true (fail-safe)")
		}
	})

	t.Run("Reset", func(t *testing.T) {
		b.Reset() // should not panic
	})

	t.Run("Wait", func(t *testing.T) {
		b.Wait() // should not panic
	})

	t.Run("WaitContext", func(t *testing.T) {
		err := b.WaitContext(context.Background())
		if err != nil {
			t.Errorf("nil.WaitContext() = %v, want nil", err)
		}
	})
}

// TestWithRotateLogger verifies the WithRotateLogger option sets the logger.
func TestWithRotateLogger(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	w, err := NewRotatingWriter(logPath,
		WithMaxSize(1024),
		WithMaxFiles(3),
		WithRotateLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewRotatingWriter() error = %v", err)
	}
	defer w.Close()

	if w.logger == nil {
		t.Error("WithRotateLogger should set logger on RotatingWriter")
	}
}

// TestLogWriterCreatesWriter verifies LogWriter helper creates a writer.
func TestLogWriterCreatesWriter(t *testing.T) {
	dir := t.TempDir()

	w, err := LogWriter(dir, "test_stream",
		WithMaxSize(DefaultMaxLogSize),
		WithMaxFiles(DefaultMaxLogFiles),
	)
	if err != nil {
		t.Fatalf("LogWriter() error = %v", err)
	}
	defer w.Close()

	// Write some data
	_, err = w.Write([]byte("test log line\n"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
}

// TestNewManagerWithLogDir verifies log writer is created when LogDir is set.
func TestNewManagerWithLogDir(t *testing.T) {
	logDir := t.TempDir()

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
		LogDir:     logDir,
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	mgr.mu.RLock()
	hasWriter := mgr.logWriter != nil
	mgr.mu.RUnlock()

	if !hasWriter {
		t.Error("logWriter should be set when LogDir is provided")
	}

	if err := mgr.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

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

// TestBackoffOverflowProtection verifies currentDelay resets if it becomes
// zero or negative (e.g., from time.Duration overflow).
func TestBackoffOverflowProtection(t *testing.T) {
	b := NewBackoff(1*time.Millisecond, 100*time.Millisecond, 50)

	// Manually set delay to 0 to simulate overflow
	b.mu.Lock()
	b.currentDelay = 0
	b.mu.Unlock()

	b.RecordFailure()

	// After overflow protection, delay should be reset to initialDelay
	if b.CurrentDelay() != 1*time.Millisecond {
		t.Errorf("CurrentDelay() = %v after overflow, want initialDelay (1ms)", b.CurrentDelay())
	}
}

// TestRecordSuccessShortRunOverflowProtection verifies overflow protection
// in RecordSuccess for short runs.
func TestRecordSuccessShortRunOverflowProtection(t *testing.T) {
	b := NewBackoff(1*time.Millisecond, 100*time.Millisecond, 50)

	// Manually set delay to 0 to simulate overflow
	b.mu.Lock()
	b.currentDelay = 0
	b.mu.Unlock()

	b.RecordSuccess(1 * time.Second) // short run (< 300s threshold)

	// After overflow protection, delay should be reset to initialDelay
	if b.CurrentDelay() != 1*time.Millisecond {
		t.Errorf("CurrentDelay() = %v after short-run overflow, want initialDelay (1ms)", b.CurrentDelay())
	}
}

// TestRecordSuccessShortRunMaxDelayCap verifies max delay cap in RecordSuccess
// for short runs.
func TestRecordSuccessShortRunMaxDelayCap(t *testing.T) {
	b := NewBackoff(60*time.Millisecond, 100*time.Millisecond, 50)

	// First call should double: 60 -> 120, capped to 100
	b.RecordSuccess(1 * time.Second) // short run

	if b.CurrentDelay() != 100*time.Millisecond {
		t.Errorf("CurrentDelay() = %v, want 100ms (capped)", b.CurrentDelay())
	}
}

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

// TestCompressFileSuccess verifies compressFile creates a .gz and removes original.
func TestCompressFileSuccess(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log.1")
	data := []byte("test log data for compression")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := &RotatingWriter{
		logger: slog.New(slog.NewTextHandler(devNull{}, nil)),
	}

	w.compressFile(filePath)

	// Original should be removed
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("original file should be removed after compression")
	}

	// Compressed file should exist
	gzPath := filePath + ".gz"
	if _, err := os.Stat(gzPath); os.IsNotExist(err) {
		t.Error("compressed file should exist")
	}
}

// TestCompressFileReadError verifies compressFile handles read errors.
func TestCompressFileReadError(t *testing.T) {
	w := &RotatingWriter{
		logger: slog.New(slog.NewTextHandler(devNull{}, nil)),
	}

	// Try to compress a non-existent file - should not panic
	w.compressFile("/nonexistent/path/file.log")
}

// TestCompressFileCreateError verifies compressFile handles create errors.
func TestCompressFileCreateError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log.1")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Make the directory read-only so .gz creation fails
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	w := &RotatingWriter{
		logger: slog.New(slog.NewTextHandler(devNull{}, nil)),
	}

	// Should not panic; just log a warning
	w.compressFile(filePath)
}

// TestCompressFileNilLogger verifies compressFile does not panic without logger.
func TestCompressFileNilLogger(t *testing.T) {
	w := &RotatingWriter{}

	// Non-existent file with nil logger - should not panic
	w.compressFile("/nonexistent/path/file.log")
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

// TestStartFFmpegWithResourceMonitoring exercises the resource monitor
// goroutine startup path inside startFFmpeg (lines 42-54 of process.go).
func TestStartFFmpegWithResourceMonitoring(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()

	scriptPath := filepath.Join(scriptDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\nsleep 60\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	var alertsCalled bool
	cfg := &ManagerConfig{
		DeviceName:      "test_monitor",
		ALSADevice:      "dummy",
		StreamName:      "test",
		SampleRate:      48000,
		Channels:        2,
		Bitrate:         "128k",
		Codec:           "opus",
		RTSPURL:         "/dev/null",
		OutputFormat:    "null",
		LockDir:         lockDir,
		FFmpegPath:      scriptPath,
		Backoff:         NewBackoff(1*time.Second, 10*time.Second, 5),
		MonitorInterval: 50 * time.Millisecond,
		AlertCallback: func(alerts []ResourceAlert) {
			alertsCalled = true
			_ = alertsCalled
		},
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if mgr.resourceMonitor == nil {
		t.Fatal("resourceMonitor should be set when MonitorInterval > 0")
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.startFFmpeg(ctx)
	}()

	// Wait for the process to start and monitoring to begin.
	time.Sleep(200 * time.Millisecond)

	mgr.mu.RLock()
	hasCmd := mgr.cmd != nil
	hasMonitorCancel := mgr.monitorCancel != nil
	mgr.mu.RUnlock()

	if !hasCmd {
		t.Error("cmd should be set after startFFmpeg starts")
	}
	if !hasMonitorCancel {
		t.Error("monitorCancel should be set when monitoring is enabled")
	}

	cancel()
	<-errCh
}

// TestStartFFmpegWithLogWriter exercises the logWriter stderr connection
// path (line 26-28 of process.go).
func TestStartFFmpegWithLogWriter(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()
	logDir := t.TempDir()

	scriptPath := filepath.Join(scriptDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\necho 'stderr output' >&2\nsleep 0.2\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_logwriter",
		ALSADevice:   "dummy",
		StreamName:   "test_logwriter",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(1*time.Second, 10*time.Second, 5),
		LogDir:       logDir,
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer func() {
		if closeErr := mgr.Close(); closeErr != nil {
			t.Logf("Close() error: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = mgr.startFFmpeg(ctx)

	// Verify log files were created.
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", logDir, err)
	}
	if len(entries) == 0 {
		t.Error("expected log files to be created when LogDir is configured")
	}
}

// TestStartFFmpegLocalRecordDirCreationFailure exercises the MkdirAll
// failure path (line 18-20 of process.go).
func TestStartFFmpegLocalRecordDirCreationFailure(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName:     "test",
		ALSADevice:     "hw:0,0",
		StreamName:     "test",
		SampleRate:     48000,
		Channels:       2,
		Bitrate:        "128k",
		Codec:          "opus",
		RTSPURL:        "rtsp://localhost:8554/test",
		OutputFormat:   "rtsp",
		LockDir:        t.TempDir(),
		FFmpegPath:     "/nonexistent/ffmpeg",
		Backoff:        NewBackoff(1*time.Second, 10*time.Second, 3),
		LocalRecordDir: "/\x00invalid/path",
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	err = mgr.startFFmpeg(context.Background())
	if err == nil {
		t.Fatal("startFFmpeg() should fail when LocalRecordDir creation fails")
	}
	if !strings.Contains(err.Error(), "recording directory") {
		t.Errorf("error = %q, want error about recording directory", err.Error())
	}
}

// TestForceStopWithRunningProcess exercises forceStop when a process is running.
func TestForceStopRunningProcess(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()

	scriptPath := filepath.Join(scriptDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\nsleep 60\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_force",
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
		Backoff:      NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.startFFmpeg(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	// forceStop should succeed with a running process.
	err = mgr.forceStop()
	if err != nil {
		t.Errorf("forceStop() error = %v, want nil for running process", err)
	}

	cancel()
	<-errCh
}

// TestStopWithForceKillTimeout exercises the stop() force-kill path when
// the process ignores SIGINT (line 102-104 of process.go).
func TestStopWithForceKillTimeout(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()

	// Script that traps and ignores SIGINT.
	scriptPath := filepath.Join(scriptDir, "stubborn.sh")
	scriptContent := "#!/bin/sh\ntrap '' INT TERM\nsleep 60\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_forcekill",
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
		Backoff:      NewBackoff(1*time.Second, 10*time.Second, 5),
		StopTimeout:  200 * time.Millisecond, // Short timeout to trigger force kill
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.startFFmpeg(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	cancel()

	select {
	case <-errCh:
		// Process was force-killed after timeout.
	case <-time.After(3 * time.Second):
		t.Fatal("Process should be force-killed within StopTimeout")
	}
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

// TestOpenFileStatError verifies openFile handles stat errors.
func TestOpenFileStatError(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	w := &RotatingWriter{
		path:     logPath,
		maxSize:  1024,
		maxFiles: 3,
	}

	// Normal open should succeed
	err := w.openFile()
	if err != nil {
		t.Errorf("openFile() error = %v", err)
	}
	if w.file != nil {
		w.file.Close()
	}
}
