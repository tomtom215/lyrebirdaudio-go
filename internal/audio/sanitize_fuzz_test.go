package audio

import (
	"regexp"
	"strings"
	"testing"
)

// FuzzSanitizeDeviceName fuzz tests SanitizeDeviceName with arbitrary input.
//
// Invariants verified:
//   - No panics on any input
//   - Output is always non-empty
//   - Output contains only safe characters: [a-zA-Z0-9_]
//   - Output never contains path traversal sequences
//   - Output length never exceeds MaxDeviceNameLength (64) + "dev_" prefix (4) + unknown_device_ fallback length
func FuzzSanitizeDeviceName(f *testing.F) {
	// Seed corpus: representative inputs from existing unit tests
	seeds := []string{
		// Normal device names
		"Blue Yeti",
		"USB Audio Device",
		"BlueYeti",
		"USB_Audio_Device",
		"MyDevice123",
		"Yeti Stereo Microphone",
		"Blue Microphones Yeti Stereo Microphone REV8_00",
		"USB-Audio - Device #1",

		// Edge cases: digit prefix
		"5GHz",
		"123Device",

		// Edge cases: special characters
		"Device@#Name",
		"Audio(Stereo)",
		"Device[USB]",
		"!123Device",

		// Security: suspicious patterns
		"../etc/passwd",
		"/etc/passwd",
		"device$name",
		"-device",
		"../../../etc/shadow",
		"./config",

		// Edge cases: whitespace and empty
		"",
		"   ",
		" Device",
		"Device ",
		"_Device",
		"Device_",

		// Edge cases: long inputs
		strings.Repeat("a", 64),
		strings.Repeat("a", 100),
		strings.Repeat("a", 1025),
		strings.Repeat("ab ", 30),

		// Edge cases: control characters
		"Device\x00Name",
		"Device\x07Name",
		"Device\x1bName",
		"Device\x7fName",
		"Device\tName",
		"Device\nName",
		"Device\rName",

		// Edge cases: unicode
		"Mikrofon\xc3\xbc",
		"\xff\xfe",

		// Edge cases: all underscores / special chars only
		"___",
		"!@#%^&*()",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	// Safe character pattern: only alphanumeric and underscore allowed in output
	safePattern := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

	f.Fuzz(func(t *testing.T, input string) {
		result := SanitizeDeviceName(input)

		// Invariant 1: Output must never be empty
		if result == "" {
			t.Errorf("SanitizeDeviceName(%q) returned empty string", input)
		}

		// Invariant 2: Output must contain only safe characters [a-zA-Z0-9_]
		if !safePattern.MatchString(result) {
			t.Errorf("SanitizeDeviceName(%q) = %q, contains unsafe characters", input, result)
		}

		// Invariant 3: Output must not contain path traversal
		if strings.Contains(result, "..") {
			t.Errorf("SanitizeDeviceName(%q) = %q, contains path traversal", input, result)
		}
		if strings.Contains(result, "/") {
			t.Errorf("SanitizeDeviceName(%q) = %q, contains path separator", input, result)
		}

		// Invariant 4: For non-fallback results, length must not exceed MaxDeviceNameLength + "dev_" prefix
		if !strings.HasPrefix(result, "unknown_device_") {
			maxLen := MaxDeviceNameLength + len("dev_")
			if len(result) > maxLen {
				t.Errorf("SanitizeDeviceName(%q) = %q (len=%d), exceeds max length %d", input, result, len(result), maxLen)
			}
		}

		// Invariant 5: Output must not start with underscore
		if strings.HasPrefix(result, "_") && !strings.HasPrefix(result, "unknown_device_") {
			// unknown_device_ is the only valid prefix containing underscore
			t.Errorf("SanitizeDeviceName(%q) = %q, starts with underscore", input, result)
		}

		// Invariant 6: Output must not end with underscore (unless it's the fallback format)
		if strings.HasSuffix(result, "_") {
			t.Errorf("SanitizeDeviceName(%q) = %q, ends with underscore", input, result)
		}

		// Invariant 7: No consecutive underscores (except in "unknown_device_" prefix)
		cleaned := strings.TrimPrefix(result, "unknown_device_")
		if strings.Contains(cleaned, "__") {
			t.Errorf("SanitizeDeviceName(%q) = %q, contains consecutive underscores", input, result)
		}
	})
}
