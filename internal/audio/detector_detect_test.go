package audio

import (
	"path/filepath"
	"testing"
)

// TestDetectDevices verifies USB audio device detection from /proc/asound.
// This is CRITICAL for stream management - must match bash implementation exactly.
//
// Reference: lyrebird-mic-check.sh lines 1902-1927, get_device_info() lines 551-615
func TestDetectDevices(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	devices, err := DetectDevices(testdataPath)
	if err != nil {
		t.Fatalf("DetectDevices() error = %v", err)
	}

	// Should find 2 USB devices (card0 and card1)
	if len(devices) != 2 {
		t.Errorf("DetectDevices() found %d devices, want 2", len(devices))
	}

	// Verify card0 (Blue Yeti)
	if len(devices) > 0 {
		dev := devices[0]
		if dev.CardNumber != 0 {
			t.Errorf("devices[0].CardNumber = %d, want 0", dev.CardNumber)
		}
		if dev.Name != "YetiStereoMicrophone" {
			t.Errorf("devices[0].Name = %q, want %q", dev.Name, "YetiStereoMicrophone")
		}
		if dev.USBID != "0d8c:0014" {
			t.Errorf("devices[0].USBID = %q, want %q", dev.USBID, "0d8c:0014")
		}
		if dev.VendorID != "0d8c" {
			t.Errorf("devices[0].VendorID = %q, want %q", dev.VendorID, "0d8c")
		}
		if dev.ProductID != "0014" {
			t.Errorf("devices[0].ProductID = %q, want %q", dev.ProductID, "0014")
		}
	}

	// Verify card1 (USB Audio Device)
	if len(devices) > 1 {
		dev := devices[1]
		if dev.CardNumber != 1 {
			t.Errorf("devices[1].CardNumber = %d, want 1", dev.CardNumber)
		}
		if dev.Name != "USB Audio Device" {
			t.Errorf("devices[1].Name = %q, want %q", dev.Name, "USB Audio Device")
		}
		if dev.USBID != "1234:5678" {
			t.Errorf("devices[1].USBID = %q, want %q", dev.USBID, "1234:5678")
		}
	}
}

// TestDetectDevicesNonUSB verifies that non-USB devices are skipped.
func TestDetectDevicesNonUSB(t *testing.T) {
	// Create test directory with non-USB card
	testDir := t.TempDir()
	cardDir := filepath.Join(testDir, "card0")
	if err := mkdir(cardDir); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create id file but NO usbid file (not a USB device)
	idPath := filepath.Join(cardDir, "id")
	if err := writeFile(idPath, "OnboardAudio"); err != nil {
		t.Fatalf("Failed to write id file: %v", err)
	}

	devices, err := DetectDevices(testDir)
	if err != nil {
		t.Fatalf("DetectDevices() error = %v", err)
	}

	// Should find 0 devices (card0 is not USB)
	if len(devices) != 0 {
		t.Errorf("DetectDevices() found %d devices, want 0 (non-USB should be skipped)", len(devices))
	}
}

// TestDetectDevicesEmpty verifies handling of empty /proc/asound directory.
func TestDetectDevicesEmpty(t *testing.T) {
	testDir := t.TempDir()

	devices, err := DetectDevices(testDir)
	if err != nil {
		t.Fatalf("DetectDevices() error = %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("DetectDevices() found %d devices, want 0", len(devices))
	}
}

// TestDetectDevicesMissingDirectory verifies handling of missing /proc/asound.
func TestDetectDevicesMissingDirectory(t *testing.T) {
	nonExistent := "/nonexistent/path/to/asound"

	devices, err := DetectDevices(nonExistent)
	if err == nil {
		t.Error("DetectDevices() with missing directory should return error")
	}
	if devices != nil {
		t.Errorf("DetectDevices() with error should return nil devices, got %v", devices)
	}
}

// TestDetectDevicesGlobError tests handling when glob fails.
func TestDetectDevicesGlobError(t *testing.T) {
	// This is hard to test since filepath.Glob rarely fails
	// But we can at least call it and ensure it doesn't panic
	testDir := t.TempDir()

	devices, err := DetectDevices(testDir)
	if err != nil {
		t.Fatalf("DetectDevices() error = %v", err)
	}

	// Empty directory should return empty slice
	if len(devices) != 0 {
		t.Errorf("DetectDevices() found %d devices in empty dir, want 0", len(devices))
	}
}

// TestDetectDevicesSkipsInvalidCards tests that invalid card dirs are skipped.
func TestDetectDevicesSkipsInvalidCards(t *testing.T) {
	testDir := t.TempDir()

	// Create valid USB card
	cardDir0 := filepath.Join(testDir, "card0")
	if err := mkdir(cardDir0); err != nil {
		t.Fatalf("Failed to create card0: %v", err)
	}
	if err := writeFile(filepath.Join(cardDir0, "usbid"), "1234:5678"); err != nil {
		t.Fatalf("Failed to write usbid: %v", err)
	}
	if err := writeFile(filepath.Join(cardDir0, "id"), "ValidUSB"); err != nil {
		t.Fatalf("Failed to write id: %v", err)
	}

	// Create card with invalid usbid (should be skipped)
	cardDir1 := filepath.Join(testDir, "card1")
	if err := mkdir(cardDir1); err != nil {
		t.Fatalf("Failed to create card1: %v", err)
	}
	if err := writeFile(filepath.Join(cardDir1, "usbid"), "invalid"); err != nil {
		t.Fatalf("Failed to write bad usbid: %v", err)
	}
	if err := writeFile(filepath.Join(cardDir1, "id"), "InvalidUSB"); err != nil {
		t.Fatalf("Failed to write id: %v", err)
	}

	// Create non-USB card (should be skipped)
	cardDir2 := filepath.Join(testDir, "card2")
	if err := mkdir(cardDir2); err != nil {
		t.Fatalf("Failed to create card2: %v", err)
	}
	if err := writeFile(filepath.Join(cardDir2, "id"), "OnboardAudio"); err != nil {
		t.Fatalf("Failed to write id: %v", err)
	}
	// No usbid file

	// Create invalid card directory name (should be skipped)
	cardDirInvalid := filepath.Join(testDir, "cardXYZ")
	if err := mkdir(cardDirInvalid); err != nil {
		t.Fatalf("Failed to create cardXYZ: %v", err)
	}

	devices, err := DetectDevices(testDir)
	if err != nil {
		t.Fatalf("DetectDevices() error = %v", err)
	}

	// Should only find card0 (valid USB device)
	if len(devices) != 1 {
		t.Errorf("DetectDevices() found %d devices, want 1 (only valid USB)", len(devices))
	}

	if len(devices) > 0 && devices[0].CardNumber != 0 {
		t.Errorf("First device CardNumber = %d, want 0", devices[0].CardNumber)
	}
}
