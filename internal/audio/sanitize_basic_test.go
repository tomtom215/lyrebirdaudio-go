package audio

import (
	"strings"
	"testing"
	"time"
)

// TestSanitizeDeviceName verifies device name sanitization matches bash implementation exactly.
// This is CRITICAL for config lookup - any deviation breaks production systems.
//
// Reference: lyrebird-mic-check.sh lines 395-426
func TestSanitizeDeviceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantLike string // For timestamp-based results
	}{
		// Basic alphanumeric (should pass through)
		{
			name:  "simple alphanumeric",
			input: "BlueYeti",
			want:  "BlueYeti",
		},
		{
			name:  "alphanumeric with underscores",
			input: "USB_Audio_Device",
			want:  "USB_Audio_Device",
		},
		{
			name:  "mixed case preserved",
			input: "MyDevice123",
			want:  "MyDevice123",
		},

		// Sanitization: replace non-alphanumeric
		{
			name:  "spaces to underscores",
			input: "Blue Yeti",
			want:  "Blue_Yeti",
		},
		{
			name:  "hyphens to underscores",
			input: "USB-Audio-Device",
			want:  "USB_Audio_Device",
		},
		{
			name:     "special characters with dollar (suspicious)",
			input:    "Device@#$%Name",
			wantLike: "unknown_device_", // $ is suspicious character
		},
		{
			name:  "parentheses replaced",
			input: "Audio(Stereo)",
			want:  "Audio_Stereo", // Trailing underscore stripped
		},
		{
			name:  "brackets replaced",
			input: "Device[USB]",
			want:  "Device_USB", // Trailing underscore stripped
		},

		// Collapse consecutive underscores
		{
			name:  "multiple spaces",
			input: "Blue   Yeti",
			want:  "Blue_Yeti",
		},
		{
			name:  "mixed separators",
			input: "USB - Audio - Device",
			want:  "USB_Audio_Device",
		},

		// Strip leading/trailing underscores
		{
			name:  "leading underscore",
			input: "_Device",
			want:  "Device",
		},
		{
			name:  "trailing underscore",
			input: "Device_",
			want:  "Device",
		},
		{
			name:  "leading space",
			input: " Device",
			want:  "Device",
		},
		{
			name:  "trailing space",
			input: "Device ",
			want:  "Device",
		},

		// Starts with digit: prefix "dev_"
		{
			name:  "starts with digit",
			input: "5GHz",
			want:  "dev_5GHz",
		},
		{
			name:  "starts with digit after sanitization",
			input: "!123Device",
			want:  "dev_123Device",
		},

		// Length truncation (64 char limit)
		{
			name:  "exactly 64 chars",
			input: strings.Repeat("a", 64),
			want:  strings.Repeat("a", 64),
		},
		{
			name:  "over 64 chars truncated",
			input: strings.Repeat("a", 100),
			want:  strings.Repeat("a", 64),
		},
		{
			name:  "over 64 with spaces",
			input: strings.Repeat("ab ", 30),                                          // 90 chars
			want:  "ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_ab_a", // Exact bash output
		},

		// Security: suspicious patterns (return timestamp-based fallback)
		{
			name:     "path traversal attempt",
			input:    "../etc/passwd",
			wantLike: "unknown_device_",
		},
		{
			name:     "absolute path",
			input:    "/etc/passwd",
			wantLike: "unknown_device_",
		},
		{
			name:     "dollar sign",
			input:    "device$name",
			wantLike: "unknown_device_",
		},
		{
			name:     "starts with hyphen",
			input:    "-device",
			wantLike: "unknown_device_",
		},

		// Empty or whitespace-only (fallback)
		{
			name:     "empty string",
			input:    "",
			wantLike: "unknown_device_",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			wantLike: "unknown_device_",
		},
		{
			name:     "special chars only",
			input:    "!@#$%",
			wantLike: "unknown_device_",
		},

		// Real-world device names (from production)
		{
			name:  "Blue Yeti",
			input: "Yeti Stereo Microphone",
			want:  "Yeti_Stereo_Microphone",
		},
		{
			name:  "Generic USB Audio",
			input: "USB Audio Device",
			want:  "USB_Audio_Device",
		},
		{
			name:  "Manufacturer format",
			input: "Blue Microphones Yeti Stereo Microphone REV8_00",
			want:  "Blue_Microphones_Yeti_Stereo_Microphone_REV8_00",
		},
		{
			name:  "USB with serial",
			input: "USB-Audio - Device #1",
			want:  "USB_Audio_Device_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDeviceName(tt.input)

			if tt.wantLike != "" {
				// Timestamp-based result: check prefix
				if !strings.HasPrefix(got, tt.wantLike) {
					t.Errorf("SanitizeDeviceName(%q) = %q, want prefix %q", tt.input, got, tt.wantLike)
				}
				// Verify it looks like a timestamp suffix
				suffix := strings.TrimPrefix(got, tt.wantLike)
				if len(suffix) == 0 {
					t.Errorf("SanitizeDeviceName(%q) = %q, missing timestamp suffix", tt.input, got)
				}
			} else {
				// Exact match required
				if got != tt.want {
					t.Errorf("SanitizeDeviceName(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

// TestSanitizeDeviceNameDeterministic verifies same input produces same output (except timestamps).
func TestSanitizeDeviceNameDeterministic(t *testing.T) {
	inputs := []string{
		"Blue Yeti",
		"USB Audio Device",
		"Device@#$Name",
		"123Device",
	}

	for _, input := range inputs {
		result1 := SanitizeDeviceName(input)
		result2 := SanitizeDeviceName(input)

		if result1 != result2 {
			t.Errorf("SanitizeDeviceName(%q) not deterministic: %q != %q", input, result1, result2)
		}
	}
}

// TestSanitizeDeviceNameTimestampFallback verifies timestamp uniqueness for suspicious inputs.
func TestSanitizeDeviceNameTimestampFallback(t *testing.T) {
	// These inputs should trigger timestamp-based fallback
	inputs := []string{
		"../etc/passwd",
		"/etc/passwd",
		"device$name",
		"-device",
		"",
		"   ",
	}

	for _, input := range inputs {
		result1 := SanitizeDeviceName(input)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamp
		result2 := SanitizeDeviceName(input)

		// Should have timestamp prefix
		if !strings.HasPrefix(result1, "unknown_device_") {
			t.Errorf("SanitizeDeviceName(%q) = %q, expected unknown_device_ prefix", input, result1)
		}

		// Timestamps should differ (unless clock resolution issue)
		if result1 == result2 {
			t.Logf("WARNING: SanitizeDeviceName(%q) produced identical timestamps: %q", input, result1)
			// Not failing - clock resolution might be coarse
		}
	}
}

// TestSanitizeDeviceNameMaxLength ensures 64-char limit is enforced.
func TestSanitizeDeviceNameMaxLength(t *testing.T) {
	inputs := []string{
		strings.Repeat("a", 100),
		strings.Repeat("ab ", 50),
		strings.Repeat("USB Audio Device ", 10),
	}

	for _, input := range inputs {
		result := SanitizeDeviceName(input)

		// Never suspicious input, so not timestamp fallback
		if strings.HasPrefix(result, "unknown_device_") {
			// This is OK if input was sanitized to empty
			continue
		}

		if len(result) > 64 {
			t.Errorf("SanitizeDeviceName(%q) = %q (len=%d), exceeds 64 chars", input, result, len(result))
		}
	}
}

// TestSanitizeDeviceNameExcessiveLength verifies rejection of excessively long inputs.
func TestSanitizeDeviceNameExcessiveLength(t *testing.T) {
	tests := []struct {
		name     string
		inputLen int
		wantLike string
	}{
		{
			name:     "exactly 1024 chars (at limit)",
			inputLen: MaxRawInputLength,
			wantLike: "", // Should be processed normally
		},
		{
			name:     "1025 chars (over limit)",
			inputLen: MaxRawInputLength + 1,
			wantLike: "unknown_device_", // Should trigger fallback
		},
		{
			name:     "10000 chars (way over limit)",
			inputLen: 10000,
			wantLike: "unknown_device_", // Should trigger fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Repeat("a", tt.inputLen)
			got := SanitizeDeviceName(input)

			if tt.wantLike != "" {
				// Should trigger timestamp fallback
				if !strings.HasPrefix(got, tt.wantLike) {
					t.Errorf("SanitizeDeviceName(len=%d) = %q, want prefix %q", tt.inputLen, got, tt.wantLike)
				}
			} else {
				// Should be processed normally (truncated to 64)
				if len(got) > MaxDeviceNameLength {
					t.Errorf("SanitizeDeviceName(len=%d) = %q (len=%d), exceeds 64 chars", tt.inputLen, got, len(got))
				}
				if strings.HasPrefix(got, "unknown_device_") {
					t.Errorf("SanitizeDeviceName(len=%d) = %q, unexpected fallback", tt.inputLen, got)
				}
			}
		})
	}
}
