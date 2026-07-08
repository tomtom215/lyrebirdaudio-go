// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunDiagnoseUnknownFlagDoesNotPanic verifies that unknown flags are handled.
func TestRunDiagnoseUnknownFlagDoesNotPanic(t *testing.T) {
	err := runDiagnose([]string{"--verbose", "--unknown"})
	// Should complete without panic
	_ = err
}

// TestRunCheckSystemWithFakeTools verifies check-system with mocked tools.
func TestRunCheckSystemWithFakeTools(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake ffmpeg
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho 'ffmpeg version 6.0'\n"), 0750); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create fake groups command
	fakeGroups := filepath.Join(tmpBin, "groups")
	if err := os.WriteFile(fakeGroups, []byte("#!/bin/sh\necho 'user audio video'\n"), 0750); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runCheckSystem([]string{})
	if err != nil {
		t.Errorf("runCheckSystem() unexpected error: %v", err)
	}
}

// TestRunDiagnoseWithFakeTools verifies diagnose with mocked system tools.
func TestRunDiagnoseWithFakeTools(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake ffmpeg that reports version
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then echo 'ffmpeg version 6.1.1'; fi\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake arecord
	fakeArecord := filepath.Join(tmpBin, "arecord")
	if err := os.WriteFile(fakeArecord, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake systemctl
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\necho 'inactive'\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake mediamtx
	fakeMediamtx := filepath.Join(tmpBin, "mediamtx")
	if err := os.WriteFile(fakeMediamtx, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runDiagnose([]string{})
	if err != nil {
		t.Errorf("runDiagnose() unexpected error: %v", err)
	}
}

// TestRunDiagnoseWithoutMediamtx verifies diagnose when mediamtx is absent.
func TestRunDiagnoseWithoutMediamtx(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake ffmpeg
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho 'ffmpeg version 6.0'\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake arecord
	fakeArecord := filepath.Join(tmpBin, "arecord")
	if err := os.WriteFile(fakeArecord, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake systemctl that reports inactive for mediamtx
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\necho 'inactive'\nexit 3\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// No mediamtx in PATH
	t.Setenv("PATH", tmpBin)

	err := runDiagnose([]string{})
	if err != nil {
		t.Errorf("runDiagnose() unexpected error: %v", err)
	}
}

// TestRunCheckSystemWithMissingFFmpeg verifies check-system exits non-zero when
// the required ffmpeg binary is absent (M-cli1): a missing required tool is a
// deterministic incompatibility that automation must be able to detect.
func TestRunCheckSystemWithMissingFFmpeg(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake groups (no audio group)
	fakeGroups := filepath.Join(tmpBin, "groups")
	if err := os.WriteFile(fakeGroups, []byte("#!/bin/sh\necho 'user video'\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// No ffmpeg in PATH
	t.Setenv("PATH", tmpBin)

	err := runCheckSystem([]string{})
	if err == nil {
		t.Error("runCheckSystem() with missing ffmpeg: expected non-nil error, got nil")
	}
}

// TestRunCheckSystemGroupsFails verifies check-system when groups command fails.
func TestRunCheckSystemGroupsFails(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake groups that fails
	fakeGroups := filepath.Join(tmpBin, "groups")
	if err := os.WriteFile(fakeGroups, []byte("#!/bin/sh\nexit 1\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake ffmpeg
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runCheckSystem([]string{})
	if err != nil {
		t.Errorf("runCheckSystem() unexpected error: %v", err)
	}
}
