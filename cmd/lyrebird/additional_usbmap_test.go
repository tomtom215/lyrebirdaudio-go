// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunUSBMapWithPathNoReloadFlag verifies the --no-reload flag.
func TestRunUSBMapWithPathNoReloadFlag(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	args := []string{"--dry-run", "--no-reload"}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() with --no-reload unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathOutputSpaceForm verifies output flag with space separator.
func TestRunUSBMapWithPathOutputSpaceForm(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-rules")

	args := []string{"--dry-run", "--output", outputPath}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() with --output space form unexpected error: %v", err)
	}
}

// TestGetUSBBusDevFromCardWithSysRootDevnumReadError tests devnum read failure.
func TestGetUSBBusDevFromCardWithSysRootDevnumReadError(t *testing.T) {
	sysRoot := t.TempDir()

	usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-4")
	if err := os.MkdirAll(usbDevDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create busnum but make devnum a directory (so ReadFile fails)
	if err := os.WriteFile(filepath.Join(usbDevDir, "busnum"), []byte("1\n"), 0644); err != nil {
		t.Fatalf("WriteFile busnum: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(usbDevDir, "devnum"), 0755); err != nil {
		t.Fatalf("MkdirAll devnum: %v", err)
	}

	soundDir := filepath.Join(sysRoot, "class", "sound", "card4")
	if err := os.MkdirAll(soundDir, 0755); err != nil {
		t.Fatalf("MkdirAll sound: %v", err)
	}
	if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, _, err := getUSBBusDevFromCardWithSysRoot(4, sysRoot)
	if err == nil {
		t.Fatal("expected error for devnum read failure")
	}
	if !strings.Contains(err.Error(), "failed to read devnum") {
		t.Errorf("error = %q, want 'failed to read devnum'", err.Error())
	}
}

// TestGetUSBBusDevFromCardWithSysRootBusnumReadError tests busnum read failure.
func TestGetUSBBusDevFromCardWithSysRootBusnumReadError(t *testing.T) {
	sysRoot := t.TempDir()

	usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-5")
	if err := os.MkdirAll(usbDevDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create busnum as a directory (so ReadFile fails) and devnum as file
	if err := os.MkdirAll(filepath.Join(usbDevDir, "busnum"), 0755); err != nil {
		t.Fatalf("MkdirAll busnum: %v", err)
	}
	if err := os.WriteFile(filepath.Join(usbDevDir, "devnum"), []byte("5\n"), 0644); err != nil {
		t.Fatalf("WriteFile devnum: %v", err)
	}

	soundDir := filepath.Join(sysRoot, "class", "sound", "card5")
	if err := os.MkdirAll(soundDir, 0755); err != nil {
		t.Fatalf("MkdirAll sound: %v", err)
	}
	if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, _, err := getUSBBusDevFromCardWithSysRoot(5, sysRoot)
	if err == nil {
		t.Fatal("expected error for busnum read failure")
	}
	if !strings.Contains(err.Error(), "failed to read busnum") {
		t.Errorf("error = %q, want 'failed to read busnum'", err.Error())
	}
}

// TestRunUSBMapWithPathNonexistentAsound verifies error for bad asound path.
func TestRunUSBMapWithPathNonexistentAsound(t *testing.T) {
	err := runUSBMapWithPath("/nonexistent/asound", []string{"--dry-run"})
	if err == nil {
		t.Error("runUSBMapWithPath() expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "failed to detect devices") {
		t.Errorf("error = %q, want 'failed to detect devices'", err.Error())
	}
}

// TestRunUSBMapWithPathEmptyDevices verifies usb-map with no devices.
func TestRunUSBMapWithPathEmptyDevices(t *testing.T) {
	tmpDir := t.TempDir()
	emptyAsound := filepath.Join(tmpDir, "asound")
	if err := os.MkdirAll(emptyAsound, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := runUSBMapWithPath(emptyAsound, []string{})
	if err != nil {
		t.Errorf("runUSBMapWithPath() with empty dir unexpected error: %v", err)
	}
}
