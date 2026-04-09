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
)

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
