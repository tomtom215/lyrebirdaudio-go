// SPDX-License-Identifier: MIT

package stream

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
