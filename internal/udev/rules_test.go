package udev

import (
	"strings"
	"testing"
)

// TestGenerateRule verifies udev rule generation with exact format.
//
// This is CRITICAL for persistent device mapping. The format must be
// byte-for-byte identical to the bash version.
//
// Reference: usb-audio-mapper.sh generate_udev_rule() function
func TestGenerateRule(t *testing.T) {
	tests := []struct {
		name     string
		portPath string
		busNum   int
		devNum   int
		want     string
	}{
		{
			name:     "Blue Yeti on port 1-1.4",
			portPath: "1-1.4",
			busNum:   1,
			devNum:   5,
			want:     `SUBSYSTEM=="sound", KERNEL=="controlC[0-9]*", ATTRS{busnum}=="1", ATTRS{devnum}=="5", SYMLINK+="snd/by-usb-port/1-1.4"`,
		},
		{
			name:     "USB Audio on port 1-1.5",
			portPath: "1-1.5",
			busNum:   1,
			devNum:   6,
			want:     `SUBSYSTEM=="sound", KERNEL=="controlC[0-9]*", ATTRS{busnum}=="1", ATTRS{devnum}=="6", SYMLINK+="snd/by-usb-port/1-1.5"`,
		},
		{
			name:     "Device on bus 2",
			portPath: "2-3.1",
			busNum:   2,
			devNum:   10,
			want:     `SUBSYSTEM=="sound", KERNEL=="controlC[0-9]*", ATTRS{busnum}=="2", ATTRS{devnum}=="10", SYMLINK+="snd/by-usb-port/2-3.1"`,
		},
		{
			name:     "Deeply nested port",
			portPath: "1-1.4.3.2",
			busNum:   1,
			devNum:   15,
			want:     `SUBSYSTEM=="sound", KERNEL=="controlC[0-9]*", ATTRS{busnum}=="1", ATTRS{devnum}=="15", SYMLINK+="snd/by-usb-port/1-1.4.3.2"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRule(tt.portPath, tt.busNum, tt.devNum)

			if got != tt.want {
				t.Errorf("GenerateRule() mismatch:\ngot:  %q\nwant: %q", got, tt.want)
			}

			// Verify no trailing whitespace or newline
			if strings.TrimSpace(got) != got {
				t.Error("GenerateRule() has trailing/leading whitespace")
			}
		})
	}
}

// TestGenerateRuleInvalidInputs verifies error handling for invalid inputs.
func TestGenerateRuleInvalidInputs(t *testing.T) {
	tests := []struct {
		name     string
		portPath string
		busNum   int
		devNum   int
		wantErr  bool
	}{
		{"invalid port path - empty", "", 1, 5, true},
		{"invalid port path - no dash", "11", 1, 5, true},
		{"invalid port path - ends with dot", "1-1.", 1, 5, true},
		{"invalid bus number - negative", "1-1.4", -1, 5, true},
		{"invalid dev number - negative", "1-1.4", 1, -1, true},
		{"invalid bus number - zero", "1-1.4", 0, 5, true},
		{"invalid dev number - zero", "1-1.4", 1, 0, true},
		{"valid minimal case", "1-1", 1, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use GenerateRuleWithValidation variant that returns error
			rule, err := GenerateRuleWithValidation(tt.portPath, tt.busNum, tt.devNum)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateRuleWithValidation() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("GenerateRuleWithValidation() unexpected error: %v", err)
				}
				if rule == "" {
					t.Error("GenerateRuleWithValidation() returned empty rule for valid input")
				}
			}
		})
	}
}

// TestGenerateRulesFile verifies complete udev rules file generation.
//
// The rules file must:
// - Start with header comment explaining purpose
// - Include timestamp for tracking
// - Have one rule per device
// - End with newline
func TestGenerateRulesFile(t *testing.T) {
	devices := []struct {
		portPath string
		busNum   int
		devNum   int
	}{
		{"1-1.4", 1, 5},
		{"1-1.5", 1, 6},
		{"2-3.1", 2, 10},
	}

	content := GenerateRulesFile(devices)

	// Verify header comment exists
	if !strings.HasPrefix(content, "#") {
		t.Error("GenerateRulesFile() should start with header comment")
	}

	// Verify all devices have rules
	for _, dev := range devices {
		expected := GenerateRule(dev.portPath, dev.busNum, dev.devNum)
		if !strings.Contains(content, expected) {
			t.Errorf("GenerateRulesFile() missing rule for %s:\n%s", dev.portPath, expected)
		}
	}

	// Verify ends with newline
	if !strings.HasSuffix(content, "\n") {
		t.Error("GenerateRulesFile() should end with newline")
	}

	// Verify no empty lines between rules (compact format)
	lines := strings.Split(content, "\n")
	var emptyCount int
	for i, line := range lines {
		if line == "" && i > 0 && i < len(lines)-1 {
			emptyCount++
		}
	}
	if emptyCount > 1 {
		t.Errorf("GenerateRulesFile() has %d empty lines, want at most 1", emptyCount)
	}
}

// TestWriteRulesFile verifies file writing with proper permissions.
//
// udev rules files must be:
// - Owned by root:root (0:0)
// - Mode 0644 (readable by all, writable by root)
// - Located at /etc/udev/rules.d/99-usb-soundcards.rules
func TestWriteRulesFile(t *testing.T) {
	// This is a unit test - we test the generation logic, not actual file writing
	// Actual file writing is tested in integration tests

	devices := []struct {
		portPath string
		busNum   int
		devNum   int
	}{
		{"1-1.4", 1, 5},
	}

	// Verify content generation
	content := GenerateRulesFile(devices)
	if content == "" {
		t.Error("GenerateRulesFile() returned empty content")
	}

	// Verify correct path constant
	expectedPath := "/etc/udev/rules.d/99-usb-soundcards.rules"
	if RulesFilePath != expectedPath {
		t.Errorf("RulesFilePath = %q, want %q", RulesFilePath, expectedPath)
	}
}

// TestRuleFormatByteForByte verifies EXACT byte-for-byte format match.
//
// This test ensures we maintain 100% compatibility with the bash version.
func TestRuleFormatByteForByte(t *testing.T) {
	// Test specific format requirements
	rule := GenerateRule("1-1.4", 1, 5)

	// CRITICAL: Verify exact format components
	requirements := []struct {
		name  string
		check func(string) bool
		desc  string
	}{
		{
			"double equals for comparisons",
			func(r string) bool { return strings.Count(r, `=="`) == 4 }, // SUBSYSTEM, KERNEL, 2x ATTRS (SYMLINK uses +=)
			"must use == for comparisons (except SYMLINK which uses +=)",
		},
		{
			"SYMLINK uses +=",
			func(r string) bool { return strings.Contains(r, `SYMLINK+="`) },
			"must use += for SYMLINK (append operation)",
		},
		{
			"correct KERNEL pattern",
			func(r string) bool { return strings.Contains(r, `KERNEL=="controlC[0-9]*"`) },
			"must match controlC[0-9]* pattern",
		},
		{
			"ATTRS uses curly braces",
			func(r string) bool {
				return strings.Contains(r, `ATTRS{busnum}`) && strings.Contains(r, `ATTRS{devnum}`)
			},
			"must use ATTRS{} syntax",
		},
		{
			"comma-space separation",
			func(r string) bool {
				parts := strings.Split(r, ", ")
				return len(parts) == 5 // 5 parts separated by ", "
			},
			"must separate with ', ' (comma-space)",
		},
	}

	for _, req := range requirements {
		t.Run(req.name, func(t *testing.T) {
			if !req.check(rule) {
				t.Errorf("Format requirement failed: %s\nRule: %s", req.desc, rule)
			}
		})
	}
}

// TestDeviceInfo verifies the DeviceInfo struct for rule generation.
func TestDeviceInfo(t *testing.T) {
	device := DeviceInfo{
		PortPath: "1-1.4",
		BusNum:   1,
		DevNum:   5,
		Product:  "Yeti Stereo Microphone",
		Serial:   "REV8_12345",
	}

	// Verify rule generation from struct
	rule := device.GenerateRule()
	expected := `SUBSYSTEM=="sound", KERNEL=="controlC[0-9]*", ATTRS{busnum}=="1", ATTRS{devnum}=="5", SYMLINK+="snd/by-usb-port/1-1.4"`

	if rule != expected {
		t.Errorf("DeviceInfo.GenerateRule() mismatch:\ngot:  %q\nwant: %q", rule, expected)
	}
}

// BenchmarkGenerateRule measures performance of rule generation.
func BenchmarkGenerateRule(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateRule("1-1.4", 1, 5)
	}
}

// BenchmarkGenerateRulesFile measures performance of full file generation.
func BenchmarkGenerateRulesFile(b *testing.B) {
	devices := make([]struct {
		portPath string
		busNum   int
		devNum   int
	}, 10)

	for i := range devices {
		devices[i] = struct {
			portPath string
			busNum   int
			devNum   int
		}{
			portPath: "1-1.4",
			busNum:   1,
			devNum:   i + 1,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateRulesFile(devices)
	}
}
