//go:build linux

package diagnostics

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGetChecks(t *testing.T) {
	// Test quick mode
	optsQuick := DefaultOptions()
	optsQuick.Mode = ModeQuick
	runnerQuick := NewRunner(optsQuick)
	quickChecks := runnerQuick.getChecks()

	if len(quickChecks) != 5 {
		t.Errorf("expected 5 quick checks, got %d", len(quickChecks))
	}

	// Test full mode
	optsFull := DefaultOptions()
	optsFull.Mode = ModeFull
	runnerFull := NewRunner(optsFull)
	fullChecks := runnerFull.getChecks()

	if len(fullChecks) != 30 {
		t.Errorf("expected 30 full checks, got %d", len(fullChecks))
	}

	// Test debug mode (same as full)
	optsDebug := DefaultOptions()
	optsDebug.Mode = ModeDebug
	runnerDebug := NewRunner(optsDebug)
	debugChecks := runnerDebug.getChecks()

	if len(debugChecks) != 30 {
		t.Errorf("expected 30 full checks, got %d", len(debugChecks))
	}
}

func TestCheckConfig(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		configPath     string
		createConfig   bool
		expectedStatus CheckStatus
	}{
		{
			name:           "config exists",
			configPath:     tmpDir + "/config.yaml",
			createConfig:   true,
			expectedStatus: StatusOK,
		},
		{
			name:           "config not found",
			configPath:     tmpDir + "/nonexistent.yaml",
			createConfig:   false,
			expectedStatus: StatusWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.createConfig {
				err := os.WriteFile(tt.configPath, []byte("test: true"), 0644)
				if err != nil {
					t.Fatalf("failed to create config file: %v", err)
				}
			}

			opts := DefaultOptions()
			opts.ConfigPath = tt.configPath
			runner := NewRunner(opts)

			result := runner.checkConfig(context.Background())
			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, result.Status)
			}
		})
	}
}

func TestCheckLogFiles(t *testing.T) {
	// Test with non-existent log directory
	opts := DefaultOptions()
	opts.LogDir = "/nonexistent/log/dir"
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected status OK for non-existent log dir, got %s", result.Status)
	}

	// Test with existing log directory
	tmpDir := t.TempDir()
	opts.LogDir = tmpDir
	runner = NewRunner(opts)

	result = runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected status OK for empty log dir, got %s", result.Status)
	}

	// Create some log files
	_ = os.WriteFile(tmpDir+"/test.log", []byte("log content"), 0644)
	_ = os.WriteFile(tmpDir+"/test.log.1", []byte("rotated log"), 0644)

	result = runner.checkLogFiles(context.Background())
	// Status depends on size thresholds
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s for log files", result.Status)
	}
}

func TestCheckPrerequisites(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkPrerequisites(context.Background())
	if result.Name != "Prerequisites" {
		t.Errorf("expected Name 'Prerequisites', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckVersions(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkVersions(ctx)
	if result.Name != "Versions" {
		t.Errorf("expected Name 'Versions', got %q", result.Name)
	}
	if result.Status != StatusOK {
		t.Errorf("expected status OK, got %s", result.Status)
	}
}

func TestCheckSystemInfo(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkSystemInfo(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected status OK, got %s", result.Status)
	}
	if result.Name != "System Info" {
		t.Errorf("expected Name 'System Info', got %q", result.Name)
	}
}

func TestCheckUSBAudio(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUSBAudio(context.Background())
	if result.Name != "USB Audio" {
		t.Errorf("expected Name 'USB Audio', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckUdevRules(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUdevRules(context.Background())
	if result.Name != "udev Rules" {
		t.Errorf("expected Name 'udev Rules', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckLockDir(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkLockDir(context.Background())
	if result.Name != "Lock Directory" {
		t.Errorf("expected Name 'Lock Directory', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkDiskSpace(context.Background())
	if result.Name != "Disk Space" {
		t.Errorf("expected Name 'Disk Space', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckMemory(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkMemory(context.Background())
	if result.Name != "Memory" {
		t.Errorf("expected Name 'Memory', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckNetworkPorts(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkNetworkPorts(ctx)
	if result.Name != "Network Ports" {
		t.Errorf("expected Name 'Network Ports', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}
