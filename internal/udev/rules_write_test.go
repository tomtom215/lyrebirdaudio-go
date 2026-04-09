package udev

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

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
