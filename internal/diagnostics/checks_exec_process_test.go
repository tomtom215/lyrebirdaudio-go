// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckProcessStabilitySkippedPath verifies that checkProcessStability
// handles the case where the journalctl command fails (returns "skipped" status).
// Uses a pre-cancelled context to make exec.CommandContext fail immediately.
func TestCheckProcessStabilitySkippedPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancel so exec.CommandContext fails immediately.

	runner := NewRunner(DefaultOptions())
	result := runner.checkProcessStability(ctx)

	if result.Name != "Process Stability" {
		t.Errorf("Name = %q, want %q", result.Name, "Process Stability")
	}
	// With a cancelled context, journalctl cannot run → StatusOK + "skipped" message.
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when journalctl unavailable", result.Status)
	}
	if result.Message != "Process stability check skipped" {
		t.Errorf("Message = %q, want %q", result.Message, "Process stability check skipped")
	}
}

// TestCheckProcessStabilitySuccessPath verifies that checkProcessStability
// calls evaluateProcessRestarts when journalctl exits successfully.
// Uses a fake journalctl that outputs nothing (no restarts detected).
func TestCheckProcessStabilitySuccessPath(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake journalctl: exit 0, empty output.
	script := "#!/bin/sh\nexit 0\n"
	scriptPath := filepath.Join(tmpBin, "journalctl")
	if err := os.WriteFile(scriptPath, []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake journalctl: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkProcessStability(context.Background())

	if result.Name != "Process Stability" {
		t.Errorf("Name = %q, want %q", result.Name, "Process Stability")
	}
	if result.Message == "Process stability check skipped" {
		t.Error("expected evaluateProcessRestarts to be called, not skipped")
	}
	// Empty output → no restarts detected → StatusOK.
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK for empty journalctl output", result.Status)
	}
}
