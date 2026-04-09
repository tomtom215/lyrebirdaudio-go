// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"testing"
)

// TestFindFFmpegPathPresent verifies findFFmpegPath when ffmpeg is in PATH.
func TestFindFFmpegPathPresent(t *testing.T) {
	path, err := findFFmpegPath()
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	if path == "" {
		t.Error("findFFmpegPath returned empty path without error")
	}

	// Verify the returned path is executable
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("stat returned path: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("returned ffmpeg path is not executable")
	}
}
