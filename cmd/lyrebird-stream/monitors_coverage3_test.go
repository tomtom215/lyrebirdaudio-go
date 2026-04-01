// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// makeImmutable uses chattr +i to set the immutable flag on a file,
// preventing removal even as root. It skips the test if chattr is unavailable
// or if the filesystem does not support the immutable attribute.
func makeImmutable(t *testing.T, path string) {
	t.Helper()
	if err := exec.Command("chattr", "+i", path).Run(); err != nil { //#nosec G204 -- test-only helper with fixed args
		t.Skipf("chattr +i failed (filesystem may not support immutable flag): %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("chattr", "-i", path).Run() //#nosec G204 -- test cleanup
	})
}

// TestStartStallDetectorDefaultMaxStallChecks covers monitors.go:211-213 —
// the `maxStallChecks = 3` branch when cfg.Monitor.MaxStallChecks <= 0.
// The context is pre-cancelled so the for loop exits immediately after
// executing the setup code (including the default assignment) before the
// first tick.
func TestStartStallDetectorDefaultMaxStallChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so for loop exits on first iteration

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sup := supervisor.New(supervisor.Config{})
	var mu sync.RWMutex
	services := map[string]bool{}
	hashes := map[string]string{}

	cfg := config.DefaultConfig()
	cfg.Monitor.MaxStallChecks = 0 // triggers maxStallChecks = 3 branch

	// startStallDetector executes `maxStallChecks = 3` before entering the
	// for loop, then exits immediately via ctx.Done().
	startStallDetector(ctx, logger, cfg, sup, &mu, services, hashes)
	// Reaching here without panic means the default-assignment branch executed.
}

// TestStartWatchdogNotifyFailed covers maintenance.go:65-67 — the
// `logger.Warn("watchdog notify failed")` branch when sdNotify returns an error.
// WATCHDOG_USEC is set to 10000 (10ms interval), NOTIFY_SOCKET points to a
// non-existent socket so the notify fails on every tick.
func TestStartWatchdogNotifyFailed(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "10000")                             // interval = 5ms
	t.Setenv("NOTIFY_SOCKET", "/nonexistent/watchdog/notify.sock") // sdNotify will fail

	var sb syncBuffer
	logger := slog.New(slog.NewTextHandler(&sb, nil))
	ctx, cancel := context.WithCancel(context.Background())

	startWatchdog(ctx, logger)

	// Allow at least one tick (5ms interval) for the notify to fail.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for the watchdog goroutine to exit.
	time.Sleep(20 * time.Millisecond)

	if !sb.Contains("watchdog notify failed") {
		t.Errorf("expected 'watchdog notify failed' log, got: %s", sb.String())
	}
}

// TestCleanupSegmentsRemoveAgeError covers maintenance.go:147-149 — the
// `logger.Warn("segment retention: failed to delete old segment")` path when
// os.Remove fails on an age-expired file. The segment file is made immutable
// via chattr +i so os.Remove fails even as root. The test is skipped if the
// filesystem does not support the immutable attribute.
func TestCleanupSegmentsRemoveAgeError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a segment file with an old modification time.
	segFile := filepath.Join(tmpDir, "segment-old.wav")
	if err := os.WriteFile(segFile, []byte("data"), 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	oldTime := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(segFile, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Set immutable flag so os.Remove fails even as root.
	makeImmutable(t, segFile)

	var sb syncBuffer
	logger := slog.New(slog.NewTextHandler(&sb, nil))
	streamCfg := config.StreamConfig{
		LocalRecordDir: tmpDir,
		SegmentMaxAge:  1 * time.Hour, // files older than 1h are candidates
	}

	cleanupSegments(logger, streamCfg)

	if !sb.Contains("failed to delete old segment") {
		t.Errorf("expected 'failed to delete old segment' warning, got: %s", sb.String())
	}
}

// TestCleanupSegmentsRemoveSizeError covers maintenance.go:175-177 — the
// `logger.Warn("segment retention: failed to delete segment for size budget")`
// path when os.Remove fails during size-budget enforcement. The file is made
// immutable via chattr +i so os.Remove fails even as root.
func TestCleanupSegmentsRemoveSizeError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a segment file to exceed the budget.
	segFile := filepath.Join(tmpDir, "segment-big.wav")
	payload := make([]byte, 1024) // 1 KB
	if err := os.WriteFile(segFile, payload, 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Set immutable flag so os.Remove fails even as root.
	makeImmutable(t, segFile)

	var sb syncBuffer
	logger := slog.New(slog.NewTextHandler(&sb, nil))
	streamCfg := config.StreamConfig{
		LocalRecordDir:       tmpDir,
		SegmentMaxTotalBytes: 1, // budget = 1 byte → file exceeds it
	}

	cleanupSegments(logger, streamCfg)

	if !sb.Contains("failed to delete segment for size budget") {
		t.Errorf("expected 'failed to delete segment for size budget' warning, got: %s", sb.String())
	}
}
