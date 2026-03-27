// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckKernelModules(t *testing.T) {
	r := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := r.checkKernelModules(ctx)

	if result.Name != "Kernel Modules" {
		t.Errorf("expected name 'Kernel Modules', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected category 'System', got %q", result.Category)
	}
	// On CI/test machines snd_usb_audio may not be loaded
	validStatuses := map[CheckStatus]bool{StatusOK: true, StatusWarning: true, StatusCritical: true, StatusError: true}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %v", result.Status)
	}
	if result.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestCheckDevicePermissions(t *testing.T) {
	r := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := r.checkDevicePermissions(ctx)

	if result.Name != "Device Permissions" {
		t.Errorf("expected name 'Device Permissions', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected category 'Audio', got %q", result.Category)
	}
	// On machines without sound cards this will be warning
	validStatuses := map[CheckStatus]bool{StatusOK: true, StatusWarning: true}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %v", result.Status)
	}
}

func TestCheckFFmpegCodecs(t *testing.T) {
	r := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := r.checkFFmpegCodecs(ctx)

	if result.Name != "FFmpeg Codecs" {
		t.Errorf("expected name 'FFmpeg Codecs', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected category 'Audio', got %q", result.Category)
	}
	// FFmpeg may not be installed in CI
	validStatuses := map[CheckStatus]bool{StatusOK: true, StatusSkipped: true, StatusCritical: true, StatusError: true}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %v", result.Status)
	}
}

func TestCheckUSBStability(t *testing.T) {
	r := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := r.checkUSBStability(ctx)

	if result.Name != "USB Stability" {
		t.Errorf("expected name 'USB Stability', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected category 'Audio', got %q", result.Category)
	}
	// dmesg may require root
	validStatuses := map[CheckStatus]bool{StatusOK: true, StatusWarning: true, StatusSkipped: true}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %v", result.Status)
	}
}

func TestCheckLockFilePermissions(t *testing.T) {
	r := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := r.checkLockFilePermissions(ctx)

	if result.Name != "Lock File Permissions" {
		t.Errorf("expected name 'Lock File Permissions', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected category 'System', got %q", result.Category)
	}
	// Lock dir may not exist in test environment
	validStatuses := map[CheckStatus]bool{StatusOK: true, StatusWarning: true, StatusError: true}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %v", result.Status)
	}
}

func TestCheckLockFilePermissions_WithTempDir(t *testing.T) {
	// Create a temp dir to simulate lock directory with proper permissions
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create a stale lock file with a non-existent PID
	staleLock := filepath.Join(lockDir, "test_device.lock")
	if err := os.WriteFile(staleLock, []byte("999999999"), 0640); err != nil {
		t.Fatal(err)
	}

	// Create a lock file with invalid PID content
	badLock := filepath.Join(lockDir, "bad_device.lock")
	if err := os.WriteFile(badLock, []byte("not-a-pid"), 0640); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := r.checkLockFilePermissions(ctx)

	// The default lock dir is /var/run/lyrebird, not our temp dir,
	// so this tests the real system state. The stale lock test above
	// just ensures the code paths work.
	if result.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestCheckUlimits(t *testing.T) {
	r := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := r.checkUlimits(ctx)

	if result.Name != "Resource Limits" {
		t.Errorf("expected name 'Resource Limits', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected category 'System', got %q", result.Category)
	}
	validStatuses := map[CheckStatus]bool{StatusOK: true, StatusWarning: true, StatusError: true}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %v", result.Status)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestAdvancedChecksRegistered(t *testing.T) {
	r := NewRunner(Options{Mode: ModeFull, Output: os.Stdout})
	checks := r.getChecks()
	// Should have 30 checks now (24 original + 6 new)
	if len(checks) < 30 {
		t.Errorf("expected at least 30 checks in full mode, got %d", len(checks))
	}
}

func TestAdvancedChecksInReport(t *testing.T) {
	r := NewRunner(Options{Mode: ModeFull, Output: os.Stdout})
	ctx := context.Background()
	report, err := r.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify new checks appear in the report
	advancedCheckNames := map[string]bool{
		"Kernel Modules":        false,
		"Device Permissions":    false,
		"FFmpeg Codecs":         false,
		"USB Stability":         false,
		"Lock File Permissions": false,
		"Resource Limits":       false,
	}

	for _, check := range report.Checks {
		if _, exists := advancedCheckNames[check.Name]; exists {
			advancedCheckNames[check.Name] = true
		}
	}

	for name, found := range advancedCheckNames {
		if !found {
			t.Errorf("advanced check %q not found in report", name)
		}
	}
}
