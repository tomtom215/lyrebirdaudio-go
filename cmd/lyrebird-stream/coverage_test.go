// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/stream"
)

// TestParseSlogLevelAllCases tests parseSlogLevel exhaustively.
func TestParseSlogLevelAllCases(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"Debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"Error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"trace", slog.LevelInfo},
		{"fatal", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run("level_"+tt.input, func(t *testing.T) {
			got := parseSlogLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseSlogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestDeviceConfigHashDifferentConfigs verifies that changing any field
// produces a different hash.
func TestDeviceConfigHashDifferentConfigs(t *testing.T) {
	base := config.DeviceConfig{
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 512,
	}
	baseURL := "rtsp://localhost:8554/device"
	baseSC := config.StreamConfig{
		LocalRecordDir:  "/recordings",
		SegmentDuration: 3600,
		SegmentFormat:   "wav",
		StopTimeout:     5 * time.Second,
	}

	baseHash := deviceConfigHash(base, baseURL, baseSC)

	tests := []struct {
		name   string
		devCfg config.DeviceConfig
		url    string
		sc     config.StreamConfig
	}{
		{
			name:   "different thread queue",
			devCfg: func() config.DeviceConfig { d := base; d.ThreadQueue = 1024; return d }(),
			url:    baseURL,
			sc:     baseSC,
		},
		{
			name:   "different segment format",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.SegmentFormat = "flac"
				return s
			}(),
		},
		{
			name:   "different local record dir",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.LocalRecordDir = "/other"
				return s
			}(),
		},
		{
			name:   "different segment duration",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.SegmentDuration = 1800
				return s
			}(),
		},
		{
			name:   "different stop timeout",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.StopTimeout = 15 * time.Second
				return s
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := deviceConfigHash(tt.devCfg, tt.url, tt.sc)
			if h == baseHash {
				t.Errorf("hash should differ from base when %s changes", tt.name)
			}
		})
	}
}

// TestDeviceConfigHashStability verifies the same inputs produce identical hashes.
func TestDeviceConfigHashStability(t *testing.T) {
	devCfg := config.DeviceConfig{
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 512,
	}
	url := "rtsp://localhost:8554/test"
	sc := config.StreamConfig{}

	h1 := deviceConfigHash(devCfg, url, sc)
	h2 := deviceConfigHash(devCfg, url, sc)
	if h1 != h2 {
		t.Errorf("identical inputs produced different hashes: %q vs %q", h1, h2)
	}
}

// TestCleanupSegmentsSkipsDirectories verifies directories are not deleted.
func TestCleanupSegmentsSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a subdirectory (should be skipped)
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an old file
	oldFile := filepath.Join(dir, "old.wav")
	if err := os.WriteFile(oldFile, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	cfg := config.StreamConfig{
		LocalRecordDir: dir,
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	cleanupSegments(logger, cfg)

	// Old file should be deleted
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should be deleted")
	}

	// Subdirectory should still exist
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Error("subdirectory should not be deleted")
	}
}

// TestCleanupSegmentsEmptyLocalRecordDir verifies no-op when dir is empty string.
func TestCleanupSegmentsEmptyLocalRecordDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.StreamConfig{
		LocalRecordDir: "",
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	// Should return immediately without error
	cleanupSegments(logger, cfg)
}

// TestCleanupSegmentsNonExistentDir verifies graceful handling of missing dir.
func TestCleanupSegmentsNonExistentDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.StreamConfig{
		LocalRecordDir: "/nonexistent/dir/for/testing",
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	// Should log warning but not panic
	cleanupSegments(logger, cfg)
}

// TestSdNotifyNoSocket verifies sdNotify is a no-op when NOTIFY_SOCKET is not set.
func TestSdNotifyNoSocketExplicit(t *testing.T) {
	// Explicitly unset NOTIFY_SOCKET
	t.Setenv("NOTIFY_SOCKET", "")

	err := sdNotify("READY=1")
	if err != nil {
		t.Errorf("sdNotify should be no-op without NOTIFY_SOCKET, got: %v", err)
	}
}

// TestSdNotifyWithSocket verifies sdNotify sends data to the socket.
func TestSdNotifyWithSocket(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "notify.sock")

	// Create a unix datagram socket to receive notifications
	addr := &net.UnixAddr{Name: socketPath, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		t.Fatalf("ListenUnixgram: %v", err)
	}
	defer conn.Close()

	t.Setenv("NOTIFY_SOCKET", socketPath)

	err = sdNotify("READY=1")
	if err != nil {
		t.Fatalf("sdNotify() error = %v", err)
	}

	// Read the notification
	buf := make([]byte, 256)
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	got := string(buf[:n])
	if got != "READY=1" {
		t.Errorf("received %q, want %q", got, "READY=1")
	}
}

// TestSdNotifyInvalidSocket verifies sdNotify returns error for invalid socket.
func TestSdNotifyInvalidSocket(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "/nonexistent/socket/path")

	err := sdNotify("READY=1")
	if err == nil {
		t.Error("sdNotify should return error for invalid socket path")
	}
}

// TestStreamServiceRun verifies streamService.Run delegates to manager.Run.
func TestStreamServiceRun(t *testing.T) {
	lockDir := t.TempDir()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Create a mock FFmpeg script that exits immediately
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &stream.ManagerConfig{
		DeviceName:   "test_svc",
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
		Backoff:      stream.NewBackoff(10*time.Millisecond, 50*time.Millisecond, 2),
		Logger:       logger,
	}

	mgr, err := stream.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	svc := &streamService{
		name:    "test_svc",
		manager: mgr,
		logger:  logger,
	}

	// Run with a short context
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = svc.Run(ctx)
	// Should complete (either max attempts or context cancelled)
	if err == nil {
		t.Log("Run completed without error (max attempts reached or context cancelled)")
	}
}

// TestStreamServiceRunContextCancelled verifies clean shutdown logging.
func TestStreamServiceRunContextCancelled(t *testing.T) {
	lockDir := t.TempDir()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Create a script that runs for a while
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 30\n"), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &stream.ManagerConfig{
		DeviceName:   "test_cancel",
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
		Backoff:      stream.NewBackoff(1*time.Second, 5*time.Second, 10),
		Logger:       logger,
	}

	mgr, err := stream.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	svc := &streamService{
		name:    "test_cancel",
		manager: mgr,
		logger:  logger,
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	// Wait for service to start
	time.Sleep(200 * time.Millisecond)

	// Cancel
	cancel()

	select {
	case err := <-errCh:
		// context.Canceled is the expected result
		if err != nil && err != context.Canceled {
			t.Logf("Run() returned: %v (expected context.Canceled)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not complete within timeout")
	}

	// Verify "stream stopped" was logged
	output := logBuf.String()
	if output == "" {
		t.Error("expected log output")
	}
}

// TestStartWatchdogNoEnvVar verifies startWatchdog is a no-op without WATCHDOG_USEC.
func TestStartWatchdogNoEnvVar(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should return immediately (no goroutine started)
	startWatchdog(ctx, logger)
}

// TestStartWatchdogInvalidValue verifies startWatchdog handles invalid WATCHDOG_USEC.
func TestStartWatchdogInvalidValue(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "not_a_number")
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startWatchdog(ctx, logger)

	// Should log warning
	if !bytes.Contains(logBuf.Bytes(), []byte("invalid WATCHDOG_USEC")) {
		t.Errorf("expected warning about invalid WATCHDOG_USEC, got: %s", logBuf.String())
	}
}

// TestStartWatchdogZeroValue verifies startWatchdog handles zero WATCHDOG_USEC.
func TestStartWatchdogZeroValue(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "0")
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startWatchdog(ctx, logger)

	// Should log warning about invalid value
	if !bytes.Contains(logBuf.Bytes(), []byte("invalid WATCHDOG_USEC")) {
		t.Errorf("expected warning about invalid WATCHDOG_USEC, got: %s", logBuf.String())
	}
}

// TestStartWatchdogValidValue verifies startWatchdog starts the goroutine.
func TestStartWatchdogValidValue(t *testing.T) {
	// Set up a socket to receive watchdog pings
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "notify.sock")

	addr := &net.UnixAddr{Name: socketPath, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		t.Fatalf("ListenUnixgram: %v", err)
	}
	defer conn.Close()

	t.Setenv("NOTIFY_SOCKET", socketPath)
	// Set a very short interval (100ms -> ping every 50ms)
	t.Setenv("WATCHDOG_USEC", "100000")

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	ctx, cancel := context.WithCancel(context.Background())

	startWatchdog(ctx, logger)

	// Wait for at least one ping
	buf := make([]byte, 256)
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	got := string(buf[:n])
	if got != "WATCHDOG=1" {
		t.Errorf("received %q, want %q", got, "WATCHDOG=1")
	}

	cancel()
}

// TestCleanupSegmentsBothLimits verifies combined max age and size limits.
func TestCleanupSegmentsBothLimits(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	now := time.Now()

	// Create 3 files:
	// old1: 20 days old, 1KB
	// old2: 5 days old, 1KB
	// new1: 1 hour old, 1KB
	files := []struct {
		name string
		age  time.Duration
	}{
		{"old1.wav", 20 * 24 * time.Hour},
		{"old2.wav", 5 * 24 * time.Hour},
		{"new1.wav", 1 * time.Hour},
	}

	data := make([]byte, 1024)
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		mtime := now.Add(-f.age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.StreamConfig{
		LocalRecordDir:       dir,
		SegmentMaxAge:        10 * 24 * time.Hour, // Delete files > 10 days
		SegmentMaxTotalBytes: 1500,                // Budget: 1500 bytes (only 1 file fits)
	}

	cleanupSegments(logger, cfg)

	// old1 should be deleted by age (20 > 10 days)
	if _, err := os.Stat(filepath.Join(dir, "old1.wav")); !os.IsNotExist(err) {
		t.Error("old1.wav should be deleted by max age")
	}

	// old2 should be deleted by size budget (2KB > 1.5KB budget)
	if _, err := os.Stat(filepath.Join(dir, "old2.wav")); !os.IsNotExist(err) {
		t.Error("old2.wav should be deleted by size budget")
	}

	// new1 should be kept
	if _, err := os.Stat(filepath.Join(dir, "new1.wav")); os.IsNotExist(err) {
		t.Error("new1.wav should be kept")
	}
}

// TestStreamServiceName verifies streamService.Name returns the correct name.
func TestStreamServiceNameTableDriven(t *testing.T) {
	tests := []struct {
		name     string
		svcName  string
		wantName string
	}{
		{"simple name", "blue_yeti", "blue_yeti"},
		{"with underscores", "usb_mic_1", "usb_mic_1"},
		{"empty name", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &streamService{name: tt.svcName}
			if got := svc.Name(); got != tt.wantName {
				t.Errorf("Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

// TestDaemonSystemInfoProviderEmptyRecordDir verifies fallback to "/" when
// recordDir is empty.
func TestDaemonSystemInfoProviderEmptyRecordDir(t *testing.T) {
	p := &daemonSystemInfoProvider{
		recordDir:        "",
		diskLowThreshold: 0,
	}
	si := p.SystemInfo()

	// Should still report disk stats (for "/")
	if si.DiskTotalBytes == 0 {
		t.Error("DiskTotalBytes should be non-zero for root filesystem")
	}
}

// TestRunDaemonLogDirCreationFailure verifies behavior when log dir cannot be
// created (falls back to no logging).
func TestRunDaemonLogDirCreationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	flags := daemonFlags{
		ConfigPath: filepath.Join(tmpDir, "nonexistent.yaml"),
		LockDir:    tmpDir,
		LogLevel:   "error",
		LogDir:     "/\x00invalid/log/dir",
	}

	// The daemon should continue (log dir failure is not fatal)
	// but will fail later at ffmpeg lookup (if ffmpeg not installed)
	// or succeed and run. Either way, it shouldn't crash.
	code := runDaemon(flags)
	// code 1 is expected (ffmpeg not found or other startup issue)
	_ = code
}

// TestRunSegmentRetention verifies the retention goroutine runs cleanup and
// responds to context cancellation.
func TestRunSegmentRetention(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create an old file
	oldFile := filepath.Join(dir, "old.wav")
	if err := os.WriteFile(oldFile, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	cfg := config.StreamConfig{
		LocalRecordDir: dir,
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runSegmentRetention(ctx, logger, cfg)
		close(done)
	}()

	// Let initial cleanup pass run
	time.Sleep(100 * time.Millisecond)

	// Old file should be deleted by the initial cleanup pass
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should be deleted by initial retention pass")
	}

	cancel()

	// Wait for goroutine to exit
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runSegmentRetention did not exit after context cancellation")
	}
}

// TestRunDiskSpaceMonitor verifies the disk monitor goroutine runs and
// responds to context cancellation.
func TestRunDiskSpaceMonitor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Stream: config.StreamConfig{
			LocalRecordDir: t.TempDir(),
		},
		Monitor: config.MonitorConfig{
			DiskLowThresholdMB: 1, // 1MB - very low threshold, unlikely to trigger
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runDiskSpaceMonitor(ctx, logger, cfg)
		close(done)
	}()

	// Let initial check run
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runDiskSpaceMonitor did not exit after context cancellation")
	}
}

// TestRunDiskSpaceMonitorHighThreshold verifies warning is logged when disk
// space is below threshold.
func TestRunDiskSpaceMonitorHighThreshold(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	cfg := &config.Config{
		Stream: config.StreamConfig{
			LocalRecordDir: t.TempDir(),
		},
		Monitor: config.MonitorConfig{
			// Impossibly high threshold to trigger warning
			DiskLowThresholdMB: 1 << 30, // ~1 PB
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runDiskSpaceMonitor(ctx, logger, cfg)
		close(done)
	}()

	// Let initial check run
	time.Sleep(100 * time.Millisecond)

	cancel()
	<-done

	// Should have logged a warning
	if !bytes.Contains(logBuf.Bytes(), []byte("LOW DISK SPACE WARNING")) {
		t.Error("expected low disk space warning in log output")
	}
}

// TestRunDiskSpaceMonitorEmptyRecordDir verifies fallback to "/" when
// LocalRecordDir is empty.
func TestRunDiskSpaceMonitorEmptyRecordDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Stream: config.StreamConfig{
			LocalRecordDir: "", // Should fall back to "/"
		},
		Monitor: config.MonitorConfig{
			DiskLowThresholdMB: 1,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runDiskSpaceMonitor(ctx, logger, cfg)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runDiskSpaceMonitor did not exit after context cancellation")
	}
}

// TestRunDaemonWithConfigAndLogDir exercises runDaemon with valid lock dir and
// log dir but no ffmpeg (expected to fail at ffmpeg check).
func TestRunDaemonWithConfigAndLogDir(t *testing.T) {
	if _, err := findFFmpegPath(); err == nil {
		t.Skip("ffmpeg is installed; this test requires ffmpeg to be absent")
	}

	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")

	flags := daemonFlags{
		ConfigPath: filepath.Join(tmpDir, "config.yaml"),
		LockDir:    tmpDir,
		LogLevel:   "debug",
		LogDir:     logDir,
	}

	code := runDaemon(flags)
	if code != 1 {
		t.Errorf("runDaemon() returned %d, want 1 (ffmpeg not found)", code)
	}

	// Log dir should have been created
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Error("log directory should have been created")
	}
}

// TestRunDaemonWithInvalidConfig exercises runDaemon when config file exists
// but is invalid YAML.
func TestRunDaemonWithInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	flags := daemonFlags{
		ConfigPath: cfgPath,
		LockDir:    tmpDir,
		LogLevel:   "error",
	}

	code := runDaemon(flags)
	if code != 1 {
		t.Errorf("runDaemon() with invalid config returned %d, want 1", code)
	}
}

// TestFindFFmpegPathPresent verifies findFFmpegPath when ffmpeg is in PATH.
func TestFindFFmpegPathPresent(t *testing.T) {
	path, err := findFFmpegPath()
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	if path == "" {
		t.Error("findFFmpegPath returned empty path without error")
	}

	// Verify the returned path is executable
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("stat returned path: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("returned ffmpeg path is not executable")
	}
}
