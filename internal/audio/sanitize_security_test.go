package audio

import (
	"strings"
	"testing"
)

// TestSanitizeDeviceNameNoPathTraversal ensures no path traversal in output.
func TestSanitizeDeviceNameNoPathTraversal(t *testing.T) {
	malicious := []string{
		"../../../etc/passwd",
		"./config",
		"/etc/shadow",
		"device/../etc",
	}

	for _, input := range malicious {
		result := SanitizeDeviceName(input)

		// Output must not contain path separators or ".."
		if strings.Contains(result, "/") {
			t.Errorf("SanitizeDeviceName(%q) = %q, contains path separator", input, result)
		}
		if strings.Contains(result, "..") {
			t.Errorf("SanitizeDeviceName(%q) = %q, contains path traversal", input, result)
		}
	}
}

// TestSanitizeDeviceNameControlChars verifies rejection of control characters.
func TestSanitizeDeviceNameControlChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLike string
	}{
		{
			name:     "null byte",
			input:    "Device\x00Name",
			wantLike: "unknown_device_",
		},
		{
			name:     "bell character",
			input:    "Device\x07Name",
			wantLike: "unknown_device_",
		},
		{
			name:     "backspace",
			input:    "Device\x08Name",
			wantLike: "unknown_device_",
		},
		{
			name:     "escape character",
			input:    "Device\x1bName",
			wantLike: "unknown_device_",
		},
		{
			name:     "DEL character",
			input:    "Device\x7fName",
			wantLike: "unknown_device_",
		},
		{
			name:     "tab is allowed",
			input:    "Device\tName",
			wantLike: "", // Tab is allowed - converted to underscore
		},
		{
			name:     "newline is allowed",
			input:    "Device\nName",
			wantLike: "", // Newline is allowed - converted to underscore
		},
		{
			name:     "carriage return is allowed",
			input:    "Device\rName",
			wantLike: "", // CR is allowed - converted to underscore
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDeviceName(tt.input)

			if tt.wantLike != "" {
				// Should trigger timestamp fallback
				if !strings.HasPrefix(got, tt.wantLike) {
					t.Errorf("SanitizeDeviceName(%q) = %q, want prefix %q", tt.input, got, tt.wantLike)
				}
			} else {
				// Should be processed normally
				if strings.HasPrefix(got, "unknown_device_") {
					t.Errorf("SanitizeDeviceName(%q) = %q, unexpected fallback", tt.input, got)
				}
				// Result should contain only safe characters
				for i := 0; i < len(got); i++ {
					c := got[i]
					if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
						t.Errorf("SanitizeDeviceName(%q) = %q, contains unsafe char: %q", tt.input, got, c)
					}
				}
			}
		})
	}
}

// TestContainsControlChars tests the control character detection helper.
func TestContainsControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"clean string", "Hello World", false},
		{"with tab", "Hello\tWorld", false},     // Tab is allowed
		{"with newline", "Hello\nWorld", false}, // Newline is allowed
		{"with CR", "Hello\rWorld", false},      // CR is allowed
		{"with null", "Hello\x00World", true},
		{"with bell", "Hello\x07World", true},
		{"with backspace", "Hello\x08World", true},
		{"with escape", "Hello\x1bWorld", true},
		{"with DEL", "Hello\x7fWorld", true},
		{"with form feed", "Hello\x0cWorld", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsControlChars(tt.input)
			if got != tt.want {
				t.Errorf("containsControlChars(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
