// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// syncBuffer is a bytes.Buffer with a mutex so that concurrent goroutine
// writes (slog handler) and test reads do not race.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) Contains(s string) bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return bytes.Contains(sb.buf.Bytes(), []byte(s))
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// TestStartWatchdogInvalidUsec covers maintenance.go:50-53 — the
// `logger.Warn("invalid WATCHDOG_USEC, watchdog disabled")` branch.
// WATCHDOG_USEC is set to a non-numeric string so fmt.Sscanf returns an error,
// triggering the warning and early return.
func TestStartWatchdogInvalidUsec(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "not-a-number")

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startWatchdog(ctx, logger, nil)

	if !bytes.Contains(logBuf.Bytes(), []byte("invalid WATCHDOG_USEC")) {
		t.Errorf("expected 'invalid WATCHDOG_USEC' warning, got: %s", logBuf.String())
	}
}

// TestStartWatchdogZeroUsec covers maintenance.go:50-53 — the `usec <= 0`
// branch of the WATCHDOG_USEC validation. WATCHDOG_USEC=0 parses as 0 which
// fails the usec <= 0 guard.
func TestStartWatchdogZeroUsec(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "0")

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startWatchdog(ctx, logger, nil)

	if !bytes.Contains(logBuf.Bytes(), []byte("invalid WATCHDOG_USEC")) {
		t.Errorf("expected 'invalid WATCHDOG_USEC' warning for usec=0, got: %s", logBuf.String())
	}
}

// TestDaemonSystemInfoDiskLowWarning covers services.go:93-95 — the
// `si.DiskLowWarning = true` branch in SystemInfo. By setting diskLowThreshold
// to the maximum uint64, freeBytes is always less than the threshold.
func TestDaemonSystemInfoDiskLowWarning(t *testing.T) {
	p := &daemonSystemInfoProvider{
		recordDir:        "/",
		diskLowThreshold: math.MaxUint64 / 2, // always larger than available free bytes
	}

	si := p.SystemInfo(context.Background())
	if !si.DiskLowWarning {
		t.Error("SystemInfo() DiskLowWarning = false, want true (threshold set to MaxUint64/2)")
	}
}

// TestRunDiskSpaceMonitorStatfsError covers maintenance.go:202-205 — the
// `logger.Warn("disk space check failed")` error branch in runDiskSpaceMonitor.
// LocalRecordDir is set to a non-existent path so syscall.Statfs fails with
// ENOENT on the initial checkDisk() call. The pre-cancelled context exits
// the ticker loop immediately.
func TestRunDiskSpaceMonitorStatfsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so the ticker loop exits immediately after first check

	cfg := config.DefaultConfig()
	cfg.Stream.LocalRecordDir = "/nonexistent-path-for-statfs-test-xyzzy"

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	runDiskSpaceMonitor(ctx, logger, cfg)

	if !bytes.Contains(logBuf.Bytes(), []byte("disk space check failed")) {
		t.Errorf("expected 'disk space check failed' warning, got: %s", logBuf.String())
	}
}

// TestStartHealthEndpointListenErrorWarning covers main.go:344 — the
// `logger.Warn("health endpoint error")` branch in the goroutine spawned by
// startHealthEndpoint. The port is held by our listener so
// ListenAndServeReady fails immediately with EADDRINUSE (not context.Canceled).
// A goroutine cancels the context after 100ms to let the select exit without
// waiting the full 2-second time.After timeout.
// A syncBuffer serialises concurrent goroutine writes and test reads.
func TestStartHealthEndpointListenErrorWarning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()

	var sb syncBuffer
	logger := slog.New(slog.NewTextHandler(&sb, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := supervisor.New(supervisor.Config{})
	cfg := config.DefaultConfig()
	cfg.Monitor.HealthAddr = addr

	// Cancel context after 100ms — enough time for the goroutine inside
	// startHealthEndpoint to attempt binding (fails fast) and call logger.Warn.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	startHealthEndpoint(ctx, logger, cfg, sup)

	// Give the internal goroutine a brief moment to complete its logger.Warn call.
	time.Sleep(50 * time.Millisecond)

	if !sb.Contains("health endpoint error") {
		t.Errorf("expected 'health endpoint error' log, got: %s", sb.String())
	}
}

// TestStartHealthEndpointTimeoutWarning covers main.go:350-351 — the
// `case <-time.After(2 * time.Second)` branch in startHealthEndpoint.
// The port is held (healthReady never fires), and the context is live,
// so startHealthEndpoint blocks until the 2-second timer fires.
// We accept the 2s latency to get deterministic coverage of this path.
func TestStartHealthEndpointTimeoutWarning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()

	var sb syncBuffer
	logger := slog.New(slog.NewTextHandler(&sb, nil))
	// Use a long-lived context so time.After(2s) fires before ctx.Done().
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sup := supervisor.New(supervisor.Config{})
	cfg := config.DefaultConfig()
	cfg.Monitor.HealthAddr = addr

	// This call blocks ~2 seconds until the time.After case fires.
	// The internal goroutine logs "health endpoint error" immediately (fast),
	// but the select waits for time.After(2s) since ctx is not cancelled.
	startHealthEndpoint(ctx, logger, cfg, sup)

	if !sb.Contains("health endpoint did not start within 2s") {
		t.Errorf("expected '2s timeout' log, got: %s", sb.String())
	}
}

// TestStartHealthEndpointNoLogger covers main.go:344 using io.Discard to
// verify the goroutine path executes without any log-buffer races.
func TestStartHealthEndpointNoLogger(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())

	sup := supervisor.New(supervisor.Config{})
	cfg := config.DefaultConfig()
	cfg.Monitor.HealthAddr = addr

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	startHealthEndpoint(ctx, logger, cfg, sup)
	// Reaching here without panic or race means the goroutine ran correctly.
}

// TestLoadConfigurationKoanfNoFileEnvValidationFail covers daemon_config.go:54-56 —
// the `return kc, config.DefaultConfig(), nil` path when no config file exists
// and the env-only kc.Load() fails because an env var sets an invalid value
// (LYREBIRD_DEFAULT_SAMPLE_RATE=-1 → Validate() rejects negative sample rate).
func TestLoadConfigurationKoanfNoFileEnvValidationFail(t *testing.T) {
	// Set an env var that maps to default.sample_rate = -1, which fails Validate().
	t.Setenv("LYREBIRD_DEFAULT_SAMPLE_RATE", "-1")

	kc, cfg, err := loadConfigurationKoanf("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("loadConfigurationKoanf() unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("loadConfigurationKoanf() returned nil config, want DefaultConfig()")
	}
	// kc is non-nil (env-only KoanfConfig was created) but Load() fallback to default.
	_ = kc
}
