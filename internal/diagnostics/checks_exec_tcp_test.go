// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckTCPResourcesSkippedPath verifies the "skipped" path when ss fails.
func TestCheckTCPResourcesSkippedPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := NewRunner(DefaultOptions())
	result := runner.checkTCPResources(ctx)

	if result.Name != "TCP Resources" {
		t.Errorf("Name = %q, want %q", result.Name, "TCP Resources")
	}
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when ss is skipped", result.Status)
	}
	if result.Message != "TCP check skipped" {
		t.Errorf("Message = %q, want %q", result.Message, "TCP check skipped")
	}
}

// TestCheckTCPResourcesSuccessPath verifies evaluateTCPResources is called when
// a fake ss binary succeeds and returns output.
func TestCheckTCPResourcesSuccessPath(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ss: exits 0 with minimal output (no TIME_WAIT connections).
	script := "#!/bin/sh\nprintf 'State  Recv-Q  Send-Q  Local Address:Port  Peer Address:Port\\n'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "ss"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ss: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkTCPResources(context.Background())

	if result.Status == StatusOK && result.Message == "TCP check skipped" {
		t.Error("expected evaluateTCPResources to be called, not skipped")
	}
	// With no TIME_WAIT connections, should be StatusOK (not skipped).
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK for no TIME_WAIT; msg: %s", result.Status, result.Message)
	}
}
