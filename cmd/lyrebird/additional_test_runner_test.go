// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunTestWithNonexistentFFmpeg verifies test handles missing ffmpeg.
func TestRunTestWithNonexistentFFmpeg(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Isolate PATH to hide ffmpeg
	tmpBin := t.TempDir()
	t.Setenv("PATH", tmpBin)

	err := runTest([]string{"--config=" + configPath})
	// Should complete without error (ffmpeg missing is a WARNING, not an error)
	if err != nil {
		t.Errorf("runTest() unexpected error: %v", err)
	}
}

// TestRunSetupAutoModeAsNonRoot verifies setup returns root error.
func TestRunSetupAutoModeAsNonRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}

	tests := []struct {
		name string
		args []string
	}{
		{"auto mode", []string{"--auto"}},
		{"short auto", []string{"-y"}},
		{"no args", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runSetup(tt.args)
			if err == nil {
				t.Error("runSetup() expected error for non-root")
			}
			if !strings.Contains(err.Error(), "root privileges") {
				t.Errorf("runSetup() error = %q, want 'root privileges'", err.Error())
			}
		})
	}
}

// TestRunTestWithFakeFFmpeg verifies test command when ffmpeg is available but fails.
func TestRunTestWithFakeFFmpeg(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpBin := t.TempDir()

	// Create fake ffmpeg that fails the test
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho 'codec error' >&2\nexit 1\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runTest([]string{"--config=" + configPath, "--verbose"})
	// Should complete without error (ffmpeg failure is a WARNING)
	if err != nil {
		t.Errorf("runTest() unexpected error: %v", err)
	}
}

// TestRunTestWithFFmpegSuccess verifies test command when ffmpeg succeeds.
func TestRunTestWithFFmpegSuccess(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpBin := t.TempDir()

	// Create fake ffmpeg that succeeds
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runTest([]string{"--config=" + configPath, "-v"})
	if err != nil {
		t.Errorf("runTest() unexpected error: %v", err)
	}
}
