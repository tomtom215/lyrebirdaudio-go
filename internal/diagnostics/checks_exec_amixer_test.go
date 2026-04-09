// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckAudioCapabilitiesAmixerFound verifies the StatusOK path when amixer
// is available and the info command succeeds.
func TestCheckAudioCapabilitiesAmixerFound(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake amixer: exit 0 with minimal output.
	script := "#!/bin/sh\necho 'ALSA mixer info'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "amixer"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake amixer: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkAudioCapabilities(context.Background())

	if result.Name != "Audio Capabilities" {
		t.Errorf("Name = %q, want %q", result.Name, "Audio Capabilities")
	}
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when amixer succeeds; msg: %s", result.Status, result.Message)
	}
	if result.Details == "" {
		t.Error("expected Details to be populated from amixer info output")
	}
}

// TestCheckAudioCapabilitiesAmixerFails verifies the StatusWarning path when amixer
// is installed but the info command fails.
func TestCheckAudioCapabilitiesAmixerFails(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake amixer: exit 1 (command fails).
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "amixer"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake amixer: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkAudioCapabilities(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when amixer info fails; msg: %s", result.Status, result.Message)
	}
	if result.Message != "ALSA mixer check failed" {
		t.Errorf("Message = %q, want %q", result.Message, "ALSA mixer check failed")
	}
}
