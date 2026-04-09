// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckPrerequisitesAllPresent verifies the StatusOK path when all required
// tools are available. Uses fake binaries in a temp PATH.
func TestCheckPrerequisitesAllPresent(t *testing.T) {
	tmpBin := t.TempDir()
	// Create fake executables for all required and optional tools.
	for _, name := range []string{"ffmpeg", "arecord", "aplay", "udevadm", "systemctl"} {
		script := "#!/bin/sh\nexit 0\n"
		if err := os.WriteFile(filepath.Join(tmpBin, name), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
			t.Fatalf("write fake %s: %v", name, err)
		}
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkPrerequisites(context.Background())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when all tools present; msg: %s", result.Status, result.Message)
	}
	if result.Message != "All required tools available" {
		t.Errorf("Message = %q, want %q", result.Message, "All required tools available")
	}
}

// TestCheckPrerequisitesMissingOptional verifies the StatusWarning path when
// all required tools are present but some optional tools are missing.
func TestCheckPrerequisitesMissingOptional(t *testing.T) {
	tmpBin := t.TempDir()
	// Only create the required tool (ffmpeg); omit optional ones.
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	// Use ONLY our temp dir so optional tools (arecord etc.) are not found.
	t.Setenv("PATH", tmpBin)

	runner := NewRunner(DefaultOptions())
	result := runner.checkPrerequisites(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when optional tools missing; msg: %s", result.Status, result.Message)
	}
}
