// SPDX-License-Identifier: MIT

package main

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

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
