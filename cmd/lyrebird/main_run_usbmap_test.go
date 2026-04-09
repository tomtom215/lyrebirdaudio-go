package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunUSBMapDryRun verifies usb-map --dry-run flag.
func TestRunUSBMapDryRun(t *testing.T) {
	// Skip if not root (dry-run still checks root)
	if os.Geteuid() != 0 {
		t.Skip("usb-map requires root privileges")
	}

	args := []string{"--dry-run"}
	err := runUSBMap(args)
	// May succeed or fail depending on USB devices present
	// Just verify it doesn't panic
	_ = err
}

// TestRunUSBMapRootCheck verifies root privilege check.
func TestRunUSBMapRootCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Test must run as non-root")
	}

	err := runUSBMap([]string{})
	if err == nil {
		t.Error("runUSBMap() expected error for non-root user")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Errorf("runUSBMap() error = %q, want 'root privileges'", err.Error())
	}
}

// TestRunUSBMapWithPathTestFixtures verifies usb-map with test fixtures.
func TestRunUSBMapWithPathTestFixtures(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Test with --dry-run flag
	args := []string{"--dry-run"}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathEmpty verifies usb-map with empty directory.
func TestRunUSBMapWithPathEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	emptyAsound := filepath.Join(tmpDir, "asound")
	if err := os.MkdirAll(emptyAsound, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	args := []string{"--dry-run"}
	err := runUSBMapWithPath(emptyAsound, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() with empty dir unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathNonDryRun verifies usb-map without --dry-run.
func TestRunUSBMapWithPathNonDryRun(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Without --dry-run, should print stub message
	err := runUSBMapWithPath(asoundPath, []string{})
	if err != nil {
		t.Errorf("runUSBMapWithPath() without dry-run unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathOutputFlag verifies usb-map --output flag.
func TestRunUSBMapWithPathOutputFlag(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "output with equals",
			args: []string{"--dry-run", "--output=/tmp/test-rules"},
		},
		{
			name: "output with space",
			args: []string{"--dry-run", "--output", "/tmp/test-rules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runUSBMapWithPath(asoundPath, tt.args)
			if err != nil {
				t.Errorf("runUSBMapWithPath() unexpected error: %v", err)
			}
		})
	}
}

// TestRunUSBMapFlagParsing verifies usb-map flag parsing.
func TestRunUSBMapFlagParsing(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("usb-map requires root privileges")
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "dry-run flag",
			args: []string{"--dry-run"},
		},
		{
			name: "output with equals",
			args: []string{"--dry-run", "--output=/tmp/test-rules"},
		},
		{
			name: "output with space",
			args: []string{"--dry-run", "--output", "/tmp/test-rules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// May fail if no devices, but shouldn't panic
			_ = runUSBMap(tt.args)
		})
	}
}

// TestUSBMapWithPathWriteRules verifies usb-map writes rules when not dry-run.
func TestUSBMapWithPathWriteRules(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Create temp directory for output
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "99-usb-soundcards.rules")

	// Test with output flag (but not actually writing to system path)
	args := []string{"--output=" + outputPath}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() unexpected error: %v", err)
	}
}

// TestGetUSBBusDevFromCardWithSysRoot tests the injectable variant.
func TestGetUSBBusDevFromCardWithSysRoot(t *testing.T) {
	t.Run("card device symlink not found", func(t *testing.T) {
		sysRoot := t.TempDir()
		_, _, err := getUSBBusDevFromCardWithSysRoot(0, sysRoot)
		if err == nil {
			t.Fatal("expected error for missing card device symlink")
		}
		if !strings.Contains(err.Error(), "failed to resolve card device path") {
			t.Errorf("error = %q, want 'failed to resolve card device path'", err.Error())
		}
	})

	t.Run("busnum and devnum found directly", func(t *testing.T) {
		sysRoot := t.TempDir()

		// Create the USB device directory with busnum/devnum files
		usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-1.4")
		if err := os.MkdirAll(usbDevDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbDevDir, "busnum"), []byte("1\n"), 0644); err != nil {
			t.Fatalf("WriteFile busnum: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbDevDir, "devnum"), []byte("5\n"), 0644); err != nil {
			t.Fatalf("WriteFile devnum: %v", err)
		}

		// Create the sound/card0/device symlink pointing at the USB device dir
		soundDir := filepath.Join(sysRoot, "class", "sound", "card0")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		busNum, devNum, err := getUSBBusDevFromCardWithSysRoot(0, sysRoot)
		if err != nil {
			t.Fatalf("getUSBBusDevFromCardWithSysRoot() error = %v", err)
		}
		if busNum != 1 {
			t.Errorf("busNum = %d, want 1", busNum)
		}
		if devNum != 5 {
			t.Errorf("devNum = %d, want 5", devNum)
		}
	})

	t.Run("busnum found in parent directory", func(t *testing.T) {
		sysRoot := t.TempDir()

		// USB device info is in the parent of where symlink points
		usbParentDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-1")
		childDir := filepath.Join(usbParentDir, "1-1:1.0")
		if err := os.MkdirAll(childDir, 0755); err != nil {
			t.Fatalf("MkdirAll child: %v", err)
		}
		// busnum/devnum in parent, not in child
		if err := os.WriteFile(filepath.Join(usbParentDir, "busnum"), []byte("2\n"), 0644); err != nil {
			t.Fatalf("WriteFile busnum: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbParentDir, "devnum"), []byte("7\n"), 0644); err != nil {
			t.Fatalf("WriteFile devnum: %v", err)
		}

		// Symlink points to child
		soundDir := filepath.Join(sysRoot, "class", "sound", "card1")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(childDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		busNum, devNum, err := getUSBBusDevFromCardWithSysRoot(1, sysRoot)
		if err != nil {
			t.Fatalf("getUSBBusDevFromCardWithSysRoot() error = %v", err)
		}
		if busNum != 2 {
			t.Errorf("busNum = %d, want 2", busNum)
		}
		if devNum != 7 {
			t.Errorf("devNum = %d, want 7", devNum)
		}
	})

	t.Run("busnum and devnum not found anywhere", func(t *testing.T) {
		sysRoot := t.TempDir()

		// USB device dir with NO busnum/devnum
		usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-2")
		if err := os.MkdirAll(usbDevDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		soundDir := filepath.Join(sysRoot, "class", "sound", "card2")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		_, _, err := getUSBBusDevFromCardWithSysRoot(2, sysRoot)
		if err == nil {
			t.Fatal("expected error when bus/dev not found")
		}
		if !strings.Contains(err.Error(), "USB bus/dev numbers not found") {
			t.Errorf("error = %q, want 'USB bus/dev numbers not found'", err.Error())
		}
	})

	t.Run("malformed busnum does not infinite loop", func(t *testing.T) {
		sysRoot := t.TempDir()

		usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-3")
		if err := os.MkdirAll(usbDevDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		// Write non-numeric content to busnum
		if err := os.WriteFile(filepath.Join(usbDevDir, "busnum"), []byte("not-a-number\n"), 0644); err != nil {
			t.Fatalf("WriteFile busnum: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbDevDir, "devnum"), []byte("5\n"), 0644); err != nil {
			t.Fatalf("WriteFile devnum: %v", err)
		}

		soundDir := filepath.Join(sysRoot, "class", "sound", "card3")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		// This must terminate (old code had infinite loop here)
		_, _, err := getUSBBusDevFromCardWithSysRoot(3, sysRoot)
		if err == nil {
			t.Fatal("expected error for malformed busnum")
		}
	})
}
