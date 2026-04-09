// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

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
