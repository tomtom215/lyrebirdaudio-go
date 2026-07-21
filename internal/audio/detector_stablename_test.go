// SPDX-License-Identifier: MIT

package audio

import (
	"strings"
	"testing"
)

// TestDeviceStableName verifies that StableName returns a DETERMINISTIC
// identity for every device, including devices whose raw name sanitizes to
// the timestamped "unknown_device_<unix>" fallback (fully non-ASCII product
// strings, symbols-only names, names rejected as suspicious). The daemon keys
// its stream registry on this identity across polls; a time-dependent name
// would register the same physical device as a brand-new stream every poll,
// leaking managers, lock files and FFmpeg processes without bound.
func TestDeviceStableName(t *testing.T) {
	tests := []struct {
		name string
		dev  Device
		want string
	}{
		{
			name: "clean name passes through unchanged",
			dev:  Device{Name: "Blue Yeti", VendorID: "0d8c", ProductID: "0014"},
			want: "Blue_Yeti",
		},
		{
			name: "non-ascii name falls back to usb identity",
			dev:  Device{Name: "麦克风", VendorID: "0d8c", ProductID: "0014"},
			want: "usb_0d8c_0014",
		},
		{
			name: "symbols-only name falls back to usb identity",
			dev:  Device{Name: "!!!", VendorID: "0d8c", ProductID: "0014"},
			want: "usb_0d8c_0014",
		},
		{
			name: "uppercase usb id is normalized",
			dev:  Device{Name: "###", VendorID: "0D8C", ProductID: "ABCD"},
			want: "usb_0d8c_abcd",
		},
		{
			name: "suspicious name falls back to usb identity",
			dev:  Device{Name: "../etc/passwd", VendorID: "1234", ProductID: "abcd"},
			want: "usb_1234_abcd",
		},
		{
			name: "empty name falls back to usb identity",
			dev:  Device{Name: "", VendorID: "1234", ProductID: "5678"},
			want: "usb_1234_5678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dev.StableName()
			if got != tt.want {
				t.Errorf("StableName() = %q, want %q", got, tt.want)
			}
			// The whole point: calling it again must give the same answer.
			if again := tt.dev.StableName(); again != got {
				t.Errorf("StableName() not deterministic: first %q, second %q", got, again)
			}
		})
	}
}

// TestDeviceStableNameWithoutUSBID pins the degraded case: a device with an
// unsanitizable name and NO USB ID has nothing stable to derive an identity
// from, so StableName preserves SanitizeDeviceName's fallback behavior.
func TestDeviceStableNameWithoutUSBID(t *testing.T) {
	dev := Device{Name: "!!!"}
	got := dev.StableName()
	if !strings.HasPrefix(got, "unknown_device_") {
		t.Errorf("StableName() without USB ID = %q, want unknown_device_ fallback", got)
	}
}
