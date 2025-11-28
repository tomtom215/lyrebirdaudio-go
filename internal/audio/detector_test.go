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

// TestDeviceFullDeviceID verifies FullDeviceID formatting.
func TestDeviceFullDeviceID(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		want     string
	}{
		{
			name:     "standard USB device ID",
			deviceID: "usb-Manufacturer_Model_Serial-00",
			want:     "USB_MANUFACTURER_MODEL_SERIAL_00",
		},
		{
			name:     "device ID without usb- prefix",
			deviceID: "Manufacturer_Model_Serial-00",
			want:     "USB_MANUFACTURER_MODEL_SERIAL_00",
		},
		{
			name:     "device ID with special characters",
			deviceID: "usb-Blue_Microphones_Yeti_Stereo_Microphone_797_2020_10_29_53769-00",
			want:     "USB_BLUE_MICROPHONES_YETI_STEREO_MICROPHONE_797_2020_10_29_53769_00",
		},
		{
			name:     "empty device ID",
			deviceID: "",
			want:     "",
		},
		{
			name:     "device ID with consecutive underscores",
			deviceID: "usb-Manufacturer__Model___Serial-00",
			want:     "USB_MANUFACTURER_MODEL_SERIAL_00",
		},
		{
			name:     "device ID with hyphens",
			deviceID: "usb-My-Device-Name-123",
			want:     "USB_MY_DEVICE_NAME_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := &Device{DeviceID: tt.deviceID}
			got := dev.FullDeviceID()
			if got != tt.want {
				t.Errorf("FullDeviceID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetDeviceInfoMissingUSBID tests handling of missing usbid file.
func TestGetDeviceInfoMissingUSBID(t *testing.T) {
	testDir := t.TempDir()
	cardDir := filepath.Join(testDir, "card0")
	if err := mkdir(cardDir); err != nil {
		t.Fatalf("Failed to create card directory: %v", err)
	}

	// Create id file but NOT usbid file
	idPath := filepath.Join(cardDir, "id")
	if err := writeFile(idPath, "SomeDevice"); err != nil {
		t.Fatalf("Failed to write id file: %v", err)
	}

	// Should return error (not a USB device)
	_, err := GetDeviceInfo(testDir, 0)
	if err == nil {
		t.Error("GetDeviceInfo() should return error for card without usbid")
	}
}

// TestGetDeviceInfoMalformedUSBID tests handling of invalid USB ID format.
func TestGetDeviceInfoMalformedUSBID(t *testing.T) {
	tests := []struct {
		name  string
		usbid string
	}{
		{"no colon", "0d8c0014"},
		{"too short vendor", "0d8:0014"},
		{"too short product", "0d8c:014"},
		{"too long vendor", "0d8c1:0014"},
		{"too long product", "0d8c:00145"},
		{"multiple colons", "0d8c:00:14"},
		{"empty", ""},
		{"only colon", ":"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			cardDir := filepath.Join(testDir, "card0")
			if err := mkdir(cardDir); err != nil {
				t.Fatalf("Failed to create card directory: %v", err)
			}

			// Write malformed usbid
			usbidPath := filepath.Join(cardDir, "usbid")
			if err := writeFile(usbidPath, tt.usbid); err != nil {
				t.Fatalf("Failed to write usbid: %v", err)
			}

			// Write valid id
			idPath := filepath.Join(cardDir, "id")
			if err := writeFile(idPath, "TestDevice"); err != nil {
				t.Fatalf("Failed to write id: %v", err)
			}

			// Should return error for malformed USB ID
			_, err := GetDeviceInfo(testDir, 0)
			if err == nil {
				t.Errorf("GetDeviceInfo() should return error for malformed USB ID %q", tt.usbid)
			}
		})
	}
}

// TestGetDeviceInfoEmptyName tests handling of empty device name.
func TestGetDeviceInfoEmptyName(t *testing.T) {
	testDir := t.TempDir()
	cardDir := filepath.Join(testDir, "card5")
	if err := mkdir(cardDir); err != nil {
		t.Fatalf("Failed to create card directory: %v", err)
	}

	// Write valid usbid
	usbidPath := filepath.Join(cardDir, "usbid")
	if err := writeFile(usbidPath, "1234:5678"); err != nil {
		t.Fatalf("Failed to write usbid: %v", err)
	}

	// Write EMPTY id file
	idPath := filepath.Join(cardDir, "id")
	if err := writeFile(idPath, ""); err != nil {
		t.Fatalf("Failed to write id: %v", err)
	}

	dev, err := GetDeviceInfo(testDir, 5)
	if err != nil {
		t.Fatalf("GetDeviceInfo() error = %v", err)
	}

	// Should fallback to "card5"
	if dev.Name != "card5" {
		t.Errorf("Name = %q, want %q (fallback to card number)", dev.Name, "card5")
	}
}

// TestGetDeviceInfoMissingIDFile tests handling of missing id file.
func TestGetDeviceInfoMissingIDFile(t *testing.T) {
	testDir := t.TempDir()
	cardDir := filepath.Join(testDir, "card3")
	if err := mkdir(cardDir); err != nil {
		t.Fatalf("Failed to create card directory: %v", err)
	}

	// Write valid usbid but NO id file
	usbidPath := filepath.Join(cardDir, "usbid")
	if err := writeFile(usbidPath, "abcd:ef01"); err != nil {
		t.Fatalf("Failed to write usbid: %v", err)
	}

	dev, err := GetDeviceInfo(testDir, 3)
	if err != nil {
		t.Fatalf("GetDeviceInfo() error = %v", err)
	}

	// Should use "unknown" as fallback
	if dev.Name != "unknown" {
		t.Errorf("Name = %q, want \"unknown\" (fallback for missing id)", dev.Name)
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

// TestParseUSBIDWhitespace tests handling of whitespace in USB IDs.
func TestParseUSBIDWhitespace(t *testing.T) {
	tests := []struct {
		name        string
		usbID       string
		wantVendor  string
		wantProduct string
	}{
		{"leading spaces", "  0d8c:0014", "0d8c", "0014"},
		{"trailing spaces", "0d8c:0014  ", "0d8c", "0014"},
		{"spaces around colon", "0d8c : 0014", "0d8c", "0014"},
		{"tab characters", "0d8c\t:\t0014", "0d8c", "0014"},
		{"newline at end", "0d8c:0014\n", "0d8c", "0014"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vendor, product, err := ParseUSBID(tt.usbID)
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

// TestParseUSBIDEdgeCases tests edge cases in USB ID parsing.
func TestParseUSBIDEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		usbID   string
		wantErr bool
	}{
		{"multiple colons", "0d8c:0014:extra", true},
		{"no colon", "0d8c0014", true},
		{"five digit vendor", "0d8c1:0014", true},
		{"five digit product", "0d8c:00145", true},
		{"three digit vendor", "d8c:0014", true},
		{"three digit product", "0d8c:014", true},
		{"empty after colon", "0d8c:", true},
		{"empty before colon", ":0014", true},
		{"only whitespace", "   ", true},
		{"just colon", ":", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseUSBID(tt.usbID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUSBID() error = %v, wantErr %v", err, tt.wantErr)
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
