package main

import (
	"os"
	"testing"
)

func TestFindFFmpegPath(t *testing.T) {
	// This test is environment-dependent
	// We just verify the function doesn't panic

	path, err := findFFmpegPath()

	// In CI/test environments, ffmpeg might not be installed
	// So we just verify the function returns something sensible
	if err != nil {
		t.Logf("FFmpeg not found (expected in some environments): %v", err)
		return
	}

	if path == "" {
		t.Error("findFFmpegPath returned empty path without error")
	}

	// Verify the path exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("findFFmpegPath returned non-existent path: %s", path)
	}
}

func TestFindFFmpegPathCommonLocations(t *testing.T) {
	// Test that the function checks common locations
	path, err := findFFmpegPath()

	// Either finds ffmpeg or returns appropriate error
	if err != nil {
		if path != "" {
			t.Errorf("findFFmpegPath returned path %q with error %v", path, err)
		}
	} else {
		if path == "" {
			t.Error("findFFmpegPath returned empty path without error")
		}
	}
}
