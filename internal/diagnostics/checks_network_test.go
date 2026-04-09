// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestCheckNetworkPortsWithListeners(t *testing.T) {
	// Test isPortOpen with an actual listener to cover the success path
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	if !isPortOpen(addr) {
		t.Errorf("expected port %s to be open", addr)
	}

	// Close it and verify it reports closed
	_ = ln.Close()
	// Give the OS a moment to release
	time.Sleep(10 * time.Millisecond)
	if isPortOpen(addr) {
		t.Log("port still appears open right after close (race with OS)")
	}
}

func TestCheckTCPResourcesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkTCPResources(ctx)

	if result.Name != "TCP Resources" {
		t.Errorf("expected Name 'TCP Resources', got %q", result.Name)
	}
	if result.Category != "Network" {
		t.Errorf("expected Category 'Network', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckNetworkPortsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkNetworkPorts(ctx)

	if result.Name != "Network Ports" {
		t.Errorf("expected Name 'Network Ports', got %q", result.Name)
	}
	if result.Category != "Network" {
		t.Errorf("expected Category 'Network', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
}

func TestIsPortOpenWithActiveListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	if !isPortOpen(addr) {
		t.Errorf("isPortOpen(%q) = false, expected true for active listener", addr)
	}
}
