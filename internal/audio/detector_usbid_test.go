package audio

import (
	"os"
	"path/filepath"
	"testing"
)

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
		{"non-hex vendor (4 chars)", "0d8g:0014", true},
		{"non-hex product (4 chars)", "0d8c:00zz", true},
		{"non-hex both (4 chars)", "gggg:hhhh", true},
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

// TestFindDeviceIDPathIn tests the injectable findDeviceIDPathIn function.
func TestFindDeviceIDPathIn(t *testing.T) {
	t.Run("directory does not exist", func(t *testing.T) {
		result := findDeviceIDPathIn("/nonexistent/by-id", 0)
		if result != "" {
			t.Errorf("findDeviceIDPathIn() = %q, want empty string for missing dir", result)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		byIDDir := t.TempDir()
		result := findDeviceIDPathIn(byIDDir, 0)
		if result != "" {
			t.Errorf("findDeviceIDPathIn() = %q, want empty string for empty dir", result)
		}
	})

	t.Run("symlink points to controlC0", func(t *testing.T) {
		byIDDir := t.TempDir()

		// Create the target device file
		devDir := t.TempDir()
		controlFile := filepath.Join(devDir, "controlC0")
		if err := writeFile(controlFile, ""); err != nil {
			t.Fatalf("Failed to create control file: %v", err)
		}

		// Create a symlink in byIDDir pointing to controlC0
		linkName := "usb-Blue_Microphones_Yeti-00"
		linkPath := filepath.Join(byIDDir, linkName)
		if err := os.Symlink(controlFile, linkPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}

		result := findDeviceIDPathIn(byIDDir, 0)
		if result != linkName {
			t.Errorf("findDeviceIDPathIn() = %q, want %q", result, linkName)
		}
	})

	t.Run("symlink points to different card", func(t *testing.T) {
		byIDDir := t.TempDir()

		devDir := t.TempDir()
		// Symlink points to controlC1 but we're looking for card 0
		controlFile := filepath.Join(devDir, "controlC1")
		if err := writeFile(controlFile, ""); err != nil {
			t.Fatalf("Failed to create control file: %v", err)
		}

		linkPath := filepath.Join(byIDDir, "usb-SomeDevice-00")
		if err := os.Symlink(controlFile, linkPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}

		result := findDeviceIDPathIn(byIDDir, 0)
		if result != "" {
			t.Errorf("findDeviceIDPathIn() = %q, want empty string (wrong card)", result)
		}
	})

	t.Run("multiple symlinks, one matches", func(t *testing.T) {
		byIDDir := t.TempDir()
		devDir := t.TempDir()

		// controlC0 for card 0
		ctrl0 := filepath.Join(devDir, "controlC0")
		if err := writeFile(ctrl0, ""); err != nil {
			t.Fatalf("Failed to create controlC0: %v", err)
		}
		// controlC1 for card 1
		ctrl1 := filepath.Join(devDir, "controlC1")
		if err := writeFile(ctrl1, ""); err != nil {
			t.Fatalf("Failed to create controlC1: %v", err)
		}

		link0Name := "usb-Blue_Yeti-00"
		link1Name := "usb-Other_Device-00"

		if err := os.Symlink(ctrl0, filepath.Join(byIDDir, link0Name)); err != nil {
			t.Fatalf("Failed to create symlink for card0: %v", err)
		}
		if err := os.Symlink(ctrl1, filepath.Join(byIDDir, link1Name)); err != nil {
			t.Fatalf("Failed to create symlink for card1: %v", err)
		}

		result := findDeviceIDPathIn(byIDDir, 0)
		if result != link0Name {
			t.Errorf("findDeviceIDPathIn() = %q, want %q", result, link0Name)
		}

		result1 := findDeviceIDPathIn(byIDDir, 1)
		if result1 != link1Name {
			t.Errorf("findDeviceIDPathIn() for card1 = %q, want %q", result1, link1Name)
		}
	})
}
