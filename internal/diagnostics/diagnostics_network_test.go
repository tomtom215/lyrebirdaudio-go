//go:build linux

package diagnostics

import (
	"testing"
)

func TestIsPortOpen(t *testing.T) {
	// Test with invalid address
	if isPortOpen("invalid:address:999") {
		t.Error("expected isPortOpen to return false for invalid address")
	}

	// Test with non-routable address (should timeout/fail)
	if isPortOpen("192.0.2.1:9999") {
		t.Error("expected isPortOpen to return false for non-routable address")
	}
}

func TestIsPortOpenWithValidAddress(t *testing.T) {
	// Test with localhost on a typically closed port
	result := isPortOpen("127.0.0.1:1")
	if result {
		t.Log("Port 1 appears open (unexpected)")
	}

	// Test with explicit localhost port
	result = isPortOpen("localhost:65534")
	if result {
		t.Log("Port 65534 appears open (unexpected)")
	}
}
