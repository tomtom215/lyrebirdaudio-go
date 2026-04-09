// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckVersionsWithFakeFFmpeg verifies that checkVersions includes the ffmpeg
// version when the binary is available. Uses a fake ffmpeg that prints version info.
func TestCheckVersionsWithFakeFFmpeg(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ffmpeg that prints a version line.
	script := "#!/bin/sh\necho 'ffmpeg version 6.0 Copyright (c) 2000-2023 the FFmpeg developers'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkVersions(context.Background())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Details == "" {
		t.Error("expected version details to be populated with fake ffmpeg output")
	}
}
