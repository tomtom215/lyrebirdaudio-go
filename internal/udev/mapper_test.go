package udev

import (
	"path/filepath"
	"testing"
)

// TestGetUSBPhysicalPort verifies USB port detection from sysfs.
//
// This is CRITICAL for supporting multiple identical USB devices.
// The bash version had bugs where Device 5 matched the hub instead of the device.
//
// Reference: usb-audio-mapper.sh get_usb_physical_port() lines 221-342
func TestGetUSBPhysicalPort(t *testing.T) {
	sysfsPath := filepath.Join("..", "..", "testdata", "sys", "bus", "usb", "devices")

	tests := []struct {
		name        string
		busNum      int
		devNum      int
		wantPort    string
		wantProduct string
		wantSerial  string
		wantErr     bool
	}{
		{
			name:        "Blue Yeti on port 1-1.4",
			busNum:      1,
			devNum:      5,
			wantPort:    "1-1.4",
			wantProduct: "Yeti Stereo Microphone",
			wantSerial:  "REV8_12345",
			wantErr:     false,
		},
		{
			name:        "USB Audio on port 1-1.5",
			busNum:      1,
			devNum:      6,
			wantPort:    "1-1.5",
			wantProduct: "USB Audio Device",
			wantSerial:  "",
			wantErr:     false,
		},
		{
			name:        "USB Hub on port 1-1 (should not match device on 1-1.4)",
			busNum:      1,
			devNum:      2,
			wantPort:    "1-1",
			wantProduct: "USB Hub",
			wantErr:     false,
		},
		{
			name:    "nonexistent device",
			busNum:  99,
			devNum:  99,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, product, serial, err := GetUSBPhysicalPort(sysfsPath, tt.busNum, tt.devNum)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetUSBPhysicalPort() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("GetUSBPhysicalPort() error = %v", err)
			}

			if port != tt.wantPort {
				t.Errorf("port = %q, want %q", port, tt.wantPort)
			}

			if product != tt.wantProduct {
				t.Errorf("product = %q, want %q", product, tt.wantProduct)
			}

			if serial != tt.wantSerial {
				t.Errorf("serial = %q, want %q", serial, tt.wantSerial)
			}
		})
	}
}

// TestGetUSBPhysicalPortNoBusDevMismatch verifies we don't match wrong devices.
//
// CRITICAL: The bash had a bug where searching for Device 5 could match
// the USB hub instead of the actual device. This test ensures we properly
// validate bus AND device numbers.
//
// Reference: usb-audio-mapper.sh comments about Device 5 matching hub
func TestGetUSBPhysicalPortNoBusDevMismatch(t *testing.T) {
	sysfsPath := filepath.Join("..", "..", "testdata", "sys", "bus", "usb", "devices")

	// Request device at bus=1, dev=5 (Blue Yeti on 1-1.4)
	port, _, _, err := GetUSBPhysicalPort(sysfsPath, 1, 5)
	if err != nil {
		t.Fatalf("GetUSBPhysicalPort() error = %v", err)
	}

	// Should NOT match the hub at 1-1 (bus=1, dev=2)
	if port == "1-1" {
		t.Error("GetUSBPhysicalPort() matched hub instead of device - BUG!")
	}

	// Should match the correct device at 1-1.4
	if port != "1-1.4" {
		t.Errorf("port = %q, want %q", port, "1-1.4")
	}
}

// TestParseUSBPortPath verifies port path pattern matching.
func TestParseUSBPortPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
	}{
		{"simple port", "1-1", true},
		{"nested port", "1-1.4", true},
		{"deeply nested", "1-1.4.3", true},
		{"bus 2", "2-3.1", true},
		{"invalid - no dash", "11", false},
		{"invalid - starts with letter", "a-1", false},
		{"invalid - ends with dot", "1-1.", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok := IsValidUSBPortPath(tt.input)
			if ok != tt.wantOK {
				t.Errorf("IsValidUSBPortPath(%q) = %v, want %v", tt.input, ok, tt.wantOK)
			}
		})
	}
}

// TestSafeBase10 verifies base-10 conversion with leading zero handling.
//
// This is critical to prevent octal interpretation (e.g., "08" is invalid octal).
//
// Reference: usb-audio-mapper.sh safe_base10() function
func TestSafeBase10(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"simple number", "5", 5, false},
		{"leading zeros", "005", 5, false},
		{"zero", "0", 0, false},
		{"large number", "12345", 12345, false},
		{"invalid - letters", "abc", 0, true},
		{"invalid - empty", "", 0, true},
		{"invalid - negative", "-5", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeBase10(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SafeBase10(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("SafeBase10(%q) error = %v", tt.input, err)
			}

			if got != tt.want {
				t.Errorf("SafeBase10(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestGetUSBPhysicalPortEdgeCases tests edge cases and error handling.
func TestGetUSBPhysicalPortEdgeCases(t *testing.T) {
	sysfsPath := filepath.Join("..", "..", "testdata", "sys", "bus", "usb", "devices")

	t.Run("invalid bus number", func(t *testing.T) {
		_, _, _, err := GetUSBPhysicalPort(sysfsPath, -1, 5)
		if err == nil {
			t.Error("Expected error for invalid bus number")
		}
	})

	t.Run("invalid dev number", func(t *testing.T) {
		_, _, _, err := GetUSBPhysicalPort(sysfsPath, 1, -1)
		if err == nil {
			t.Error("Expected error for invalid dev number")
		}
	})

	t.Run("nonexistent sysfs path", func(t *testing.T) {
		_, _, _, err := GetUSBPhysicalPort("/nonexistent/path", 1, 5)
		if err == nil {
			t.Error("Expected error for nonexistent path")
		}
	})
}

// BenchmarkGetUSBPhysicalPort measures performance of port detection.
func BenchmarkGetUSBPhysicalPort(b *testing.B) {
	sysfsPath := filepath.Join("..", "..", "testdata", "sys", "bus", "usb", "devices")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = GetUSBPhysicalPort(sysfsPath, 1, 5)
	}
}
