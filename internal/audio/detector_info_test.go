package audio

import (
	"os"
	"path/filepath"
	"testing"
)

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

// TestGetDeviceInfoWithByID tests getDeviceInfo with an injected byIDDir.
func TestGetDeviceInfoWithByID(t *testing.T) {
	t.Run("device ID resolved via injected byIDDir", func(t *testing.T) {
		asoundDir := t.TempDir()
		byIDDir := t.TempDir()
		devDir := t.TempDir()

		// Create a valid USB card
		cardDir := filepath.Join(asoundDir, "card0")
		if err := mkdir(cardDir); err != nil {
			t.Fatalf("Failed to create card dir: %v", err)
		}
		if err := writeFile(filepath.Join(cardDir, "usbid"), "0d8c:0014"); err != nil {
			t.Fatalf("Failed to write usbid: %v", err)
		}
		if err := writeFile(filepath.Join(cardDir, "id"), "YetiMic"); err != nil {
			t.Fatalf("Failed to write id: %v", err)
		}

		// Set up byIDDir with a symlink pointing to controlC0
		ctrlFile := filepath.Join(devDir, "controlC0")
		if err := writeFile(ctrlFile, ""); err != nil {
			t.Fatalf("Failed to create control file: %v", err)
		}
		linkName := "usb-Blue_Yeti-00"
		if err := os.Symlink(ctrlFile, filepath.Join(byIDDir, linkName)); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}

		dev, err := getDeviceInfo(asoundDir, 0, byIDDir)
		if err != nil {
			t.Fatalf("getDeviceInfo() error = %v", err)
		}
		if dev.DeviceID != linkName {
			t.Errorf("DeviceID = %q, want %q", dev.DeviceID, linkName)
		}
		if dev.Name != "YetiMic" {
			t.Errorf("Name = %q, want %q", dev.Name, "YetiMic")
		}
	})

	t.Run("byIDDir missing returns empty DeviceID", func(t *testing.T) {
		asoundDir := t.TempDir()

		cardDir := filepath.Join(asoundDir, "card2")
		if err := mkdir(cardDir); err != nil {
			t.Fatalf("Failed to create card dir: %v", err)
		}
		if err := writeFile(filepath.Join(cardDir, "usbid"), "1234:5678"); err != nil {
			t.Fatalf("Failed to write usbid: %v", err)
		}
		if err := writeFile(filepath.Join(cardDir, "id"), "TestDevice"); err != nil {
			t.Fatalf("Failed to write id: %v", err)
		}

		dev, err := getDeviceInfo(asoundDir, 2, "/nonexistent/by-id")
		if err != nil {
			t.Fatalf("getDeviceInfo() error = %v", err)
		}
		if dev.DeviceID != "" {
			t.Errorf("DeviceID = %q, want empty string when byIDDir missing", dev.DeviceID)
		}
	})
}
