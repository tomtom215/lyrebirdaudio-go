// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func newWatchdogSocket(t *testing.T) *net.UnixConn {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "notify.sock")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("ListenUnixgram: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	t.Setenv("NOTIFY_SOCKET", socketPath)
	t.Setenv("WATCHDOG_USEC", "100000") // 100ms -> ping every 50ms
	return conn
}

// TestStartWatchdogWithheldWhenUnhealthy verifies that an unhealthy liveness
// probe causes the keepalive to be withheld, so systemd can restart a wedged
// daemon instead of it being kept alive unconditionally.
func TestStartWatchdogWithheldWhenUnhealthy(t *testing.T) {
	conn := newWatchdogSocket(t)
	var sb syncBuffer // serialises the watchdog goroutine's writes and our reads
	logger := slog.New(slog.NewTextHandler(&sb, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startWatchdog(ctx, logger, func(context.Context) bool { return false })

	buf := make([]byte, 256)
	if err := conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if n, _ := conn.Read(buf); n > 0 {
		t.Errorf("watchdog sent a ping (%q) despite an unhealthy probe; want none", buf[:n])
	}
	if !sb.Contains("withholding keepalive") {
		t.Errorf("expected a 'withholding keepalive' log, got: %s", sb.String())
	}
}

// TestStartWatchdogPingsWhenHealthy verifies that a healthy probe still pings.
func TestStartWatchdogPingsWhenHealthy(t *testing.T) {
	conn := newWatchdogSocket(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startWatchdog(ctx, logger, func(context.Context) bool { return true })

	buf := make([]byte, 256)
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("expected a watchdog ping with a healthy probe, got err=%v n=%d", err, n)
	}
	if string(buf[:n]) != "WATCHDOG=1" {
		t.Errorf("ping = %q, want WATCHDOG=1", buf[:n])
	}
}
