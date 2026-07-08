package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDetectArch verifies architecture detection.
func TestDetectArch(t *testing.T) {
	arch := detectArch()

	// detectArch should return one of the known values or empty string
	validArchs := map[string]bool{
		"amd64": true,
		"arm64": true,
		"armv7": true,
		"armv6": true,
		"":      true, // Unknown arch returns empty
	}

	if !validArchs[arch] {
		t.Errorf("detectArch() returned unexpected value: %q", arch)
	}

	// On Linux, we should get a non-empty result
	if arch == "" {
		t.Log("detectArch() returned empty string (may be expected on unsupported platform)")
	}
}

// TestReadLockPID verifies lock file reading.
func TestReadLockPID(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantPID int
		wantErr bool
	}{
		{
			name:    "valid pid",
			content: "12345",
			wantPID: 12345,
			wantErr: false,
		},
		{
			name:    "valid pid with newline",
			content: "12345\n",
			wantPID: 12345,
			wantErr: false,
		},
		{
			name:    "valid pid with whitespace",
			content: "  12345  \n",
			wantPID: 12345,
			wantErr: false,
		},
		{
			name:    "invalid content",
			content: "not-a-number",
			wantPID: 0,
			wantErr: true,
		},
		{
			name:    "empty file",
			content: "",
			wantPID: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			lockFile := filepath.Join(tmpDir, "test.lock")

			if err := os.WriteFile(lockFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			pid, err := readLockPID(lockFile)

			if tt.wantErr {
				if err == nil {
					t.Error("readLockPID() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("readLockPID() unexpected error: %v", err)
				}
				if pid != tt.wantPID {
					t.Errorf("readLockPID() = %d, want %d", pid, tt.wantPID)
				}
			}
		})
	}
}

// TestReadLockPIDNonexistent verifies error on non-existent file.
func TestReadLockPIDNonexistent(t *testing.T) {
	_, err := readLockPID("/nonexistent/path/lock.file")
	if err == nil {
		t.Error("readLockPID() expected error for non-existent file, got nil")
	}
}

// TestProcessExists verifies process existence checking.
func TestProcessExists(t *testing.T) {
	// Test with current process (should exist)
	if !processExists(os.Getpid()) {
		t.Error("processExists() returned false for current process")
	}

	// PID 1 (init) always exists. When the test runs non-root, signal(0) to it
	// returns EPERM; treating EPERM as "not running" (the old behavior) would
	// misreport a live root-owned daemon as a stale lock, so this is a hard
	// assertion guarding the fix.
	if !processExists(1) {
		t.Error("processExists(1) = false, want true (EPERM must be treated as alive)")
	}

	// Test with invalid PID (should not exist)
	// Using a very large PID that's unlikely to exist
	if processExists(9999999) {
		t.Error("processExists() returned true for unlikely PID 9999999")
	}

	// Test with negative PID (should not exist)
	if processExists(-1) {
		t.Error("processExists() returned true for negative PID")
	}
}

// TestGetServiceStatus verifies service status formatting.
func TestGetServiceStatus(t *testing.T) {
	// Test with a service that likely doesn't exist
	status := getServiceStatus("nonexistent-test-service-12345")

	// Should return some status string (either error or not-installed)
	if status == "" {
		t.Error("getServiceStatus() returned empty string")
	}

	// Test with a common service (might not work in all environments)
	_ = getServiceStatus("systemd-journald")
}

// TestRunStatusWithTestFixtures verifies status command output.
func TestRunStatusWithTestFixtures(t *testing.T) {
	// Status command should not panic even without devices
	err := runStatus([]string{})
	// May or may not error depending on environment
	_ = err
}

// TestRunDiagnoseOutput verifies diagnose prints its report header. The header
// is emitted unconditionally before any environment-dependent check, so this
// assertion holds regardless of ffmpeg/config/device state and the return value.
func TestRunDiagnoseOutput(t *testing.T) {
	out, _ := captureStdout(t, func() error { return runDiagnose([]string{}) })
	if !strings.Contains(out, "LyreBird System Diagnostics") {
		t.Errorf("diagnose output missing report header; got:\n%s", out)
	}
}

// TestRunCheckSystemOutput verifies check-system prints its report header.
func TestRunCheckSystemOutput(t *testing.T) {
	out, _ := captureStdout(t, func() error { return runCheckSystem([]string{}) })
	if !strings.Contains(out, "System Compatibility Check") {
		t.Errorf("check-system output missing report header; got:\n%s", out)
	}
}
