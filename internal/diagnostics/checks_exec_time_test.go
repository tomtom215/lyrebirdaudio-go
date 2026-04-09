// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckTimeSynchronizationSuccessPath verifies that checkTimeSynchronization
// calls evaluateTimeSyncOutput when timedatectl exits successfully.
// Creates a fake timedatectl script in a temp directory and prepends it to PATH.
func TestCheckTimeSynchronizationSuccessPath(t *testing.T) {
	// Build a fake timedatectl that exits 0 and prints timedatectl-like output.
	tmpBin := t.TempDir()
	script := "#!/bin/sh\nprintf 'NTPSynchronized=yes\\nNTPService=active\\nSystemNTPService=systemd-timesyncd\\n'\n"
	scriptPath := filepath.Join(tmpBin, "timedatectl")
	if err := os.WriteFile(scriptPath, []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake timedatectl: %v", err)
	}

	// Prepend our fake binary directory to PATH; t.Setenv restores on cleanup.
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkTimeSynchronization(context.Background())

	if result.Name != "Time Sync" {
		t.Errorf("Name = %q, want %q", result.Name, "Time Sync")
	}
	// evaluateTimeSyncOutput should be called; result should be StatusOK or StatusWarning.
	if result.Status == StatusError {
		t.Errorf("unexpected StatusError: %s", result.Message)
	}
	if result.Message == "Time sync check skipped (timedatectl not available)" {
		t.Error("expected evaluateTimeSyncOutput to be called, got skipped message")
	}
}
