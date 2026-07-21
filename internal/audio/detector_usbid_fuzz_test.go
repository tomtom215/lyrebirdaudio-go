// SPDX-License-Identifier: MIT

package audio

import (
	"strings"
	"testing"
)

// FuzzParseUSBID hammers the /proc/asound usbid parser with arbitrary input.
// The usbid file is read from the kernel on every device poll; a malformed or
// exotic value (broken hardware descriptors, future kernel format changes)
// must fail cleanly, never panic, and never validate a non-hex ID.
func FuzzParseUSBID(f *testing.F) {
	seeds := []string{
		"0d8c:0014",
		"0D8C:ABCD",
		"",
		":",
		"0d8c:",
		":0014",
		"0d8c:0014:extra",
		"0d8c\x00:0014",
		"12345:0014",
		"123:0014",
		"0d8c:0014\n",
		" 0d8c : 0014 ",
		"g d8c:0014",
		"０ｄ８ｃ:0014", // full-width unicode digits
		strings.Repeat("0", 1024) + ":" + strings.Repeat("f", 1024),
		"-d8c:0014",
		"0x8c:0x14",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	isHex4 := func(s string) bool {
		if len(s) != 4 {
			return false
		}
		for i := 0; i < len(s); i++ {
			c := s[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}

	f.Fuzz(func(t *testing.T, input string) {
		vendor, product, err := ParseUSBID(input)
		if err != nil {
			if vendor != "" || product != "" {
				t.Errorf("ParseUSBID(%q) returned non-empty IDs (%q, %q) alongside error", input, vendor, product)
			}
			return
		}
		// On success both halves must be exactly 4 hex digits — these strings
		// feed identity keys and file names downstream.
		if !isHex4(vendor) || !isHex4(product) {
			t.Errorf("ParseUSBID(%q) accepted non-4-hex IDs: vendor=%q product=%q", input, vendor, product)
		}
		// Round-trip: a normalized re-parse must agree with itself.
		v2, p2, err2 := ParseUSBID(vendor + ":" + product)
		if err2 != nil || v2 != vendor || p2 != product {
			t.Errorf("ParseUSBID round-trip failed for %q: got (%q, %q, %v)", input, v2, p2, err2)
		}
	})
}

// FuzzStableNameDeterministic pins the invariant behind the daemon's stream
// registry: for a device that has a USB ID, StableName must be deterministic
// (no timestamped identity) no matter what the raw device name contains.
func FuzzStableNameDeterministic(f *testing.F) {
	seeds := []string{"Blue Yeti", "", "!!!", "麦克风", "../etc/passwd", "-name",
		"$(reboot)", strings.Repeat("x", 2000), "unknown_device_42", "\x01\x02"}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		dev := Device{Name: name, VendorID: "0d8c", ProductID: "0014"}
		first := dev.StableName()
		second := dev.StableName()
		if first != second {
			t.Errorf("StableName(%q) not deterministic: %q vs %q", name, first, second)
		}
		if strings.HasPrefix(first, "unknown_device_") {
			t.Errorf("StableName(%q) = %q: timestamped identity leaked through despite USB ID", name, first)
		}
		if first == "" {
			t.Errorf("StableName(%q) returned empty identity", name)
		}
	})
}
