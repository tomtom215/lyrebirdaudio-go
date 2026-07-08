// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"testing"
)

// TestRunDiagnoseBlocksWhenFFmpegMissing verifies M-cli1: diagnose exits
// non-zero when a streaming-blocking component (the required ffmpeg binary) is
// absent, so provisioning automation can detect a genuinely unusable host.
// Soft, environmental gaps alone (no device, ALSA absent, server down) keep it
// at exit 0 — those are covered by the existing diagnose tests.
func TestRunDiagnoseBlocksWhenFFmpegMissing(t *testing.T) {
	// Empty PATH → ffmpeg not found → blocking issue.
	t.Setenv("PATH", t.TempDir())

	if err := runDiagnose([]string{}); err == nil {
		t.Error("runDiagnose() with missing ffmpeg: expected non-nil error, got nil")
	}
}
