package audio

import (
	"os"
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

// TestGetDeviceInfo verifies reading device information from /proc/asound/cardN.
func TestGetDeviceInfo(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	tests := []struct {
		name        string
		cardNumber  int
		wantName    string
		wantUSBID   string
		wantVendor  string
		wantProduct string
		wantErr     bool
	}{
		{
			name:        "card0 - Blue Yeti",
			cardNumber:  0,
			wantName:    "YetiStereoMicrophone",
			wantUSBID:   "0d8c:0014",
			wantVendor:  "0d8c",
			wantProduct: "0014",
			wantErr:     false,
		},
		{
			name:        "card1 - USB Audio",
			cardNumber:  1,
			wantName:    "USB Audio Device",
			wantUSBID:   "1234:5678",
			wantVendor:  "1234",
			wantProduct: "5678",
			wantErr:     false,
		},
		{
			name:       "nonexistent card",
			cardNumber: 99,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev, err := GetDeviceInfo(testdataPath, tt.cardNumber)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetDeviceInfo() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("GetDeviceInfo() error = %v", err)
			}

			if dev.CardNumber != tt.cardNumber {
				t.Errorf("CardNumber = %d, want %d", dev.CardNumber, tt.cardNumber)
			}
			if dev.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", dev.Name, tt.wantName)
			}
			if dev.USBID != tt.wantUSBID {
				t.Errorf("USBID = %q, want %q", dev.USBID, tt.wantUSBID)
			}
			if dev.VendorID != tt.wantVendor {
				t.Errorf("VendorID = %q, want %q", dev.VendorID, tt.wantVendor)
			}
			if dev.ProductID != tt.wantProduct {
				t.Errorf("ProductID = %q, want %q", dev.ProductID, tt.wantProduct)
			}
		})
	}
}

// TestParseUSBID verifies USB ID parsing.
func TestParseUSBID(t *testing.T) {
	tests := []struct {
		name        string
		usbID       string
		wantVendor  string
		wantProduct string
		wantErr     bool
	}{
		{
			name:        "valid USB ID",
			usbID:       "0d8c:0014",
			wantVendor:  "0d8c",
			wantProduct: "0014",
			wantErr:     false,
		},
		{
			name:        "uppercase hex",
			usbID:       "0D8C:0014",
			wantVendor:  "0D8C",
			wantProduct: "0014",
			wantErr:     false,
		},
		{
			name:    "invalid format - no colon",
			usbID:   "0d8c0014",
			wantErr: true,
		},
		{
			name:    "invalid format - too short",
			usbID:   "0d8c:14",
			wantErr: true,
		},
		{
			name:    "empty string",
			usbID:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vendor, product, err := ParseUSBID(tt.usbID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseUSBID() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseUSBID() error = %v", err)
			}

			if vendor != tt.wantVendor {
				t.Errorf("vendor = %q, want %q", vendor, tt.wantVendor)
			}
			if product != tt.wantProduct {
				t.Errorf("product = %q, want %q", product, tt.wantProduct)
			}
		})
	}
}

// TestDeviceSanitizedNames verifies sanitized name generation for config lookup.
func TestDeviceSanitizedNames(t *testing.T) {
	tests := []struct {
		name             string
		deviceName       string
		wantFriendlyName string
	}{
		{
			name:             "Blue Yeti",
			deviceName:       "YetiStereoMicrophone",
			wantFriendlyName: "YetiStereoMicrophone",
		},
		{
			name:             "with spaces",
			deviceName:       "USB Audio Device",
			wantFriendlyName: "USB_Audio_Device",
		},
		{
			name:             "with special chars",
			deviceName:       "C-Media USB Audio",
			wantFriendlyName: "C_Media_USB_Audio",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := &Device{
				Name: tt.deviceName,
			}

			friendlyName := dev.FriendlyName()
			if friendlyName != tt.wantFriendlyName {
				t.Errorf("FriendlyName() = %q, want %q", friendlyName, tt.wantFriendlyName)
			}
		})
	}
}

// Helper functions for tests
func mkdir(path string) error {
	return os.MkdirAll(path, 0755)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
