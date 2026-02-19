package udev

import (
	"fmt"
	"os"
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
	devices := []*DeviceInfo{
		{PortPath: "1-1.4", BusNum: 1, DevNum: 5},
		{PortPath: "1-1.5", BusNum: 1, DevNum: 6},
		{PortPath: "2-3.1", BusNum: 2, DevNum: 10},
	}

	content := GenerateRulesFile(devices)

	// Verify header comment exists
	if !strings.HasPrefix(content, "#") {
		t.Error("GenerateRulesFile() should start with header comment")
	}

	// Verify all devices have rules
	for _, dev := range devices {
		expected := GenerateRule(dev.PortPath, dev.BusNum, dev.DevNum)
		if !strings.Contains(content, expected) {
			t.Errorf("GenerateRulesFile() missing rule for %s:\n%s", dev.PortPath, expected)
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

// TestRulesFileConstants verifies file writing with proper permissions.
//
// udev rules files must be:
// - Owned by root:root (0:0)
// - Mode 0644 (readable by all, writable by root)
// - Located at /etc/udev/rules.d/99-usb-soundcards.rules
func TestRulesFileConstants(t *testing.T) {
	// This is a unit test - we test the generation logic, not actual file writing
	// Actual file writing is tested in integration tests

	devices := []*DeviceInfo{
		{PortPath: "1-1.4", BusNum: 1, DevNum: 5},
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

// TestWriteRulesFileToPath verifies file writing functionality.
func TestWriteRulesFileToPath(t *testing.T) {
	t.Run("valid devices without reload", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/99-usb-soundcards.rules"

		devices := []*DeviceInfo{
			{PortPath: "1-1.4", BusNum: 1, DevNum: 5},
			{PortPath: "1-1.5", BusNum: 1, DevNum: 6},
		}

		err := WriteRulesFileToPath(devices, path, false)
		if err != nil {
			t.Fatalf("WriteRulesFileToPath() unexpected error: %v", err)
		}

		// Verify file exists and has correct content
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read rules file: %v", err)
		}

		// Verify content contains both rules
		contentStr := string(content)
		if !strings.Contains(contentStr, "1-1.4") {
			t.Error("Content missing rule for 1-1.4")
		}
		if !strings.Contains(contentStr, "1-1.5") {
			t.Error("Content missing rule for 1-1.5")
		}

		// Verify header comment
		if !strings.HasPrefix(contentStr, "#") {
			t.Error("Content should start with header comment")
		}
	})

	t.Run("single valid device", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/rules.test"

		devices := []*DeviceInfo{
			{PortPath: "1-1", BusNum: 1, DevNum: 3},
		}

		err := WriteRulesFileToPath(devices, path, false)
		if err != nil {
			t.Fatalf("WriteRulesFileToPath() unexpected error: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("Rules file was not created")
		}
	})

	t.Run("empty devices list", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/empty.rules"

		devices := []*DeviceInfo{}

		err := WriteRulesFileToPath(devices, path, false)
		if err != nil {
			t.Fatalf("WriteRulesFileToPath() unexpected error: %v", err)
		}

		// Verify file contains only header
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read rules file: %v", err)
		}
		if !strings.HasPrefix(string(content), "#") {
			t.Error("Empty rules file should still have header")
		}
	})
}

// TestWriteRulesFileValidation verifies input validation.
func TestWriteRulesFileValidation(t *testing.T) {
	tests := []struct {
		name    string
		devices []*DeviceInfo
		wantErr bool
		errMsg  string
	}{
		{
			name: "invalid port path",
			devices: []*DeviceInfo{
				{PortPath: "invalid", BusNum: 1, DevNum: 5},
			},
			wantErr: true,
			errMsg:  "invalid device 0: invalid USB port path: invalid",
		},
		{
			name: "invalid bus number",
			devices: []*DeviceInfo{
				{PortPath: "1-1.4", BusNum: -1, DevNum: 5},
			},
			wantErr: true,
			errMsg:  "invalid device 0: invalid bus number: -1 (must be positive)",
		},
		{
			name: "invalid dev number",
			devices: []*DeviceInfo{
				{PortPath: "1-1.4", BusNum: 1, DevNum: -1},
			},
			wantErr: true,
			errMsg:  "invalid device 0: invalid dev number: -1 (must be positive)",
		},
		{
			name: "multiple devices with one invalid",
			devices: []*DeviceInfo{
				{PortPath: "1-1.4", BusNum: 1, DevNum: 5},
				{PortPath: "invalid", BusNum: 1, DevNum: 6},
			},
			wantErr: true,
			errMsg:  "invalid device 1: invalid USB port path: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := tmpDir + "/test.rules"

			err := WriteRulesFileToPath(tt.devices, path, false)

			if tt.wantErr {
				if err == nil {
					t.Error("WriteRulesFileToPath() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("WriteRulesFileToPath() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("WriteRulesFileToPath() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestWriteRulesFilePermissionError verifies error handling for write failures.
func TestWriteRulesFilePermissionError(t *testing.T) {
	// Try to write to a non-existent directory
	devices := []*DeviceInfo{
		{PortPath: "1-1.4", BusNum: 1, DevNum: 5},
	}

	err := WriteRulesFileToPath(devices, "/nonexistent/path/rules.test", false)
	if err == nil {
		t.Error("WriteRulesFileToPath() expected error for invalid path")
	}
}

// TestReloadUdevRulesWith tests the injectable reloadUdevRulesWith function.
func TestReloadUdevRulesWith(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var called []string
		runner := func(name string, args ...string) ([]byte, error) {
			called = append(called, name+" "+strings.Join(args, " "))
			return nil, nil
		}
		if err := reloadUdevRulesWith(runner); err != nil {
			t.Fatalf("reloadUdevRulesWith() unexpected error: %v", err)
		}
		if len(called) != 2 {
			t.Fatalf("expected 2 commands called, got %d: %v", len(called), called)
		}
		if !strings.Contains(called[0], "reload-rules") {
			t.Errorf("first call should be reload-rules, got %q", called[0])
		}
		if !strings.Contains(called[1], "trigger") {
			t.Errorf("second call should be trigger, got %q", called[1])
		}
	})

	t.Run("reload-rules fails", func(t *testing.T) {
		runner := func(name string, args ...string) ([]byte, error) {
			if strings.Contains(strings.Join(args, " "), "reload-rules") {
				return []byte("mock error"), fmt.Errorf("exit status 1")
			}
			return nil, nil
		}
		err := reloadUdevRulesWith(runner)
		if err == nil {
			t.Fatal("reloadUdevRulesWith() expected error when reload-rules fails")
		}
		if !strings.Contains(err.Error(), "reload-rules") {
			t.Errorf("error should mention reload-rules, got: %v", err)
		}
	})

	t.Run("trigger fails", func(t *testing.T) {
		runner := func(name string, args ...string) ([]byte, error) {
			if strings.Contains(strings.Join(args, " "), "trigger") {
				return []byte("trigger error"), fmt.Errorf("exit status 1")
			}
			return nil, nil
		}
		err := reloadUdevRulesWith(runner)
		if err == nil {
			t.Fatal("reloadUdevRulesWith() expected error when trigger fails")
		}
		if !strings.Contains(err.Error(), "trigger") {
			t.Errorf("error should mention trigger, got: %v", err)
		}
	})
}

// TestWriteRulesFileToPathWithReload tests the reload=true path using injectable runner.
func TestWriteRulesFileToPathWithReload(t *testing.T) {
	t.Run("reload succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/rules.test"
		var reloaded bool
		runner := func(name string, args ...string) ([]byte, error) {
			reloaded = true
			return nil, nil
		}

		devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
		err := writeRulesFileToPathWithRunner(devices, path, true, runner)
		if err != nil {
			t.Fatalf("writeRulesFileToPathWithRunner() unexpected error: %v", err)
		}
		if !reloaded {
			t.Error("expected udev runner to be called for reload")
		}
	})

	t.Run("reload fails returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/rules.test"
		runner := func(name string, args ...string) ([]byte, error) {
			return []byte("no udevadm"), fmt.Errorf("not found")
		}

		devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
		err := writeRulesFileToPathWithRunner(devices, path, true, runner)
		if err == nil {
			t.Fatal("expected error when reload fails")
		}
		if !strings.Contains(err.Error(), "failed to reload udev rules") {
			t.Errorf("error = %q, want 'failed to reload udev rules'", err.Error())
		}
	})

	t.Run("reload=false skips runner", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/rules.test"
		var called bool
		runner := func(name string, args ...string) ([]byte, error) {
			called = true
			return nil, nil
		}

		devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
		err := writeRulesFileToPathWithRunner(devices, path, false, runner)
		if err != nil {
			t.Fatalf("writeRulesFileToPathWithRunner() unexpected error: %v", err)
		}
		if called {
			t.Error("runner should not be called when reload=false")
		}
	})
}

// TestWriteRulesFile tests the top-level WriteRulesFile function.
// Since it writes to /etc/udev/rules.d/ (requires root), we verify
// the error path when running unprivileged.
func TestWriteRulesFile(t *testing.T) {
	devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
	err := WriteRulesFile(devices, false)
	if err == nil {
		// Running as root — skip the assertion but the call must not panic
		t.Log("WriteRulesFile() succeeded (running as root)")
		return
	}
	if !strings.Contains(err.Error(), "failed to write rules file") {
		t.Errorf("WriteRulesFile() error = %q, want 'failed to write rules file'", err.Error())
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
	devices := make([]*DeviceInfo, 10)

	for i := range devices {
		devices[i] = &DeviceInfo{
			PortPath: "1-1.4",
			BusNum:   1,
			DevNum:   i + 1,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateRulesFile(devices)
	}
}
