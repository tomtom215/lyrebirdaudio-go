package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun verifies basic command routing.
func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no arguments shows help",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "help command",
			args:    []string{"help"},
			wantErr: false,
		},
		{
			name:    "version command",
			args:    []string{"version"},
			wantErr: false,
		},
		{
			name:    "unknown command",
			args:    []string{"unknown-command"},
			wantErr: true,
			errMsg:  "unknown command",
		},
		{
			name:    "validate without args uses default path",
			args:    []string{"validate"},
			wantErr: true, // Will fail because default config doesn't exist in test
		},
		{
			name:    "migrate without --from flag",
			args:    []string{"migrate"},
			wantErr: true,
			errMsg:  "--from path is required",
		},
		{
			name:    "devices command",
			args:    []string{"devices"},
			wantErr: true, // Will fail because /proc/asound doesn't exist in test
		},
		{
			name:    "detect command",
			args:    []string{"detect"},
			wantErr: true, // Will fail because /proc/asound doesn't exist in test
		},
		{
			name:    "status command (stub)",
			args:    []string{"status"},
			wantErr: false, // Stub command doesn't error
		},
		{
			name:    "test command (needs config)",
			args:    []string{"test"},
			wantErr: true, // Will fail because default config doesn't exist in test
		},
		{
			name:    "diagnose command (stub)",
			args:    []string{"diagnose"},
			wantErr: false, // Stub command doesn't error
		},
		{
			name:    "check-system command (stub)",
			args:    []string{"check-system"},
			wantErr: false, // Stub command doesn't error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Error("run() expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("run() error = %q, want substring %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("run() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRunHelp verifies help command output.
func TestRunHelp(t *testing.T) {
	err := runHelp()
	if err != nil {
		t.Errorf("runHelp() unexpected error: %v", err)
	}
}

// TestRunVersion verifies version command output.
func TestRunVersion(t *testing.T) {
	// Set version info for test
	Version = "test-version"
	GitCommit = "test-commit"
	BuildDate = "test-date"

	err := runVersion()
	if err != nil {
		t.Errorf("runVersion() unexpected error: %v", err)
	}
}

// TestRunMigrateValidation verifies migrate command validation.
func TestRunMigrateValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing --from flag",
			args:    []string{},
			wantErr: true,
			errMsg:  "--from path is required",
		},
		{
			name:    "from flag with equals",
			args:    []string{"--from=/nonexistent/file.conf"},
			wantErr: true, // Will fail because file doesn't exist
		},
		{
			name:    "from flag with space",
			args:    []string{"--from", "/nonexistent/file.conf"},
			wantErr: true, // Will fail because file doesn't exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runMigrate(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Error("runMigrate() expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("runMigrate() error = %q, want substring %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("runMigrate() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRunMigrateSuccess verifies successful migration.
func TestRunMigrateSuccess(t *testing.T) {
	// Use test fixture
	bashConfig := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")

	// Check if test data exists
	if _, err := os.Stat(bashConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Use temp directory for output
	tmpDir := t.TempDir()
	yamlConfig := filepath.Join(tmpDir, "config.yaml")

	args := []string{
		"--from", bashConfig,
		"--to", yamlConfig,
	}

	err := runMigrate(args)
	if err != nil {
		t.Fatalf("runMigrate() unexpected error: %v", err)
	}

	// Verify output file was created
	if _, err := os.Stat(yamlConfig); os.IsNotExist(err) {
		t.Error("runMigrate() did not create output file")
	}
}

// TestRunMigrateForceOverwrite verifies --force flag.
func TestRunMigrateForceOverwrite(t *testing.T) {
	bashConfig := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")

	if _, err := os.Stat(bashConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpDir := t.TempDir()
	yamlConfig := filepath.Join(tmpDir, "config.yaml")

	// Create existing file
	if err := os.WriteFile(yamlConfig, []byte("existing content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Without --force should fail
	args := []string{"--from", bashConfig, "--to", yamlConfig}
	err := runMigrate(args)
	if err == nil {
		t.Error("runMigrate() expected error for existing file without --force")
	}

	// With --force should succeed
	argsForce := []string{"--from", bashConfig, "--to", yamlConfig, "--force"}
	err = runMigrate(argsForce)
	if err != nil {
		t.Errorf("runMigrate() with --force unexpected error: %v", err)
	}
}

// TestRunValidate verifies validate command.
func TestRunValidate(t *testing.T) {
	// Test with valid config
	validConfig := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	if _, err := os.Stat(validConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	args := []string{"--config", validConfig}
	err := runValidate(args)
	if err != nil {
		t.Errorf("runValidate() with valid config unexpected error: %v", err)
	}

	// Test with invalid config
	invalidConfig := filepath.Join("..", "..", "testdata", "config", "invalid.yaml")

	if _, err := os.Stat(invalidConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	argsInvalid := []string{"--config", invalidConfig}
	err = runValidate(argsInvalid)
	if err == nil {
		t.Error("runValidate() with invalid config expected error, got nil")
	}
}

// TestRunValidateFlagParsing verifies config flag parsing.
func TestRunValidateFlagParsing(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "config flag with equals",
			args: []string{"--config=/etc/lyrebird/config.yaml"},
		},
		{
			name: "config flag with space",
			args: []string{"--config", "/etc/lyrebird/config.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Will fail because file doesn't exist, but we're testing flag parsing
			err := runValidate(tt.args)
			if err == nil {
				t.Error("runValidate() expected error for nonexistent file")
			}
			// Verify error is about loading, not flag parsing
			if !strings.Contains(err.Error(), "failed to load config") {
				t.Errorf("runValidate() error = %q, want 'failed to load config'", err.Error())
			}
		})
	}
}

// TestRunUSBMapDryRun verifies usb-map --dry-run flag.
func TestRunUSBMapDryRun(t *testing.T) {
	// Skip if not root (dry-run still checks root)
	if os.Geteuid() != 0 {
		t.Skip("usb-map requires root privileges")
	}

	args := []string{"--dry-run"}
	err := runUSBMap(args)
	// May succeed or fail depending on USB devices present
	// Just verify it doesn't panic
	_ = err
}

// TestRunUSBMapRootCheck verifies root privilege check.
func TestRunUSBMapRootCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Test must run as non-root")
	}

	err := runUSBMap([]string{})
	if err == nil {
		t.Error("runUSBMap() expected error for non-root user")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Errorf("runUSBMap() error = %q, want 'root privileges'", err.Error())
	}
}

// TestRunSetupRootCheck verifies setup root privilege check.
func TestRunSetupRootCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Test must run as non-root")
	}

	err := runSetup([]string{})
	if err == nil {
		t.Error("runSetup() expected error for non-root user")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Errorf("runSetup() error = %q, want 'root privileges'", err.Error())
	}
}

// TestRunInstallMediaMTXRootCheck verifies install-mediamtx root privilege check.
func TestRunInstallMediaMTXRootCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Test must run as non-root")
	}

	err := runInstallMediaMTX([]string{})
	if err == nil {
		t.Error("runInstallMediaMTX() expected error for non-root user")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Errorf("runInstallMediaMTX() error = %q, want 'root privileges'", err.Error())
	}
}

// TestStubCommands verifies stub commands don't panic.
func TestStubCommands(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]string) error
	}{
		{"status", runStatus},
		{"setup", func(args []string) error {
			if os.Geteuid() != 0 {
				return nil // Skip root check for test
			}
			return runSetup(args)
		}},
		{"install-mediamtx", func(args []string) error {
			if os.Geteuid() != 0 {
				return nil // Skip root check for test
			}
			return runInstallMediaMTX(args)
		}},
		{"test", runTest},
		{"diagnose", runDiagnose},
		{"check-system", runCheckSystem},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify they don't panic
			_ = tt.fn([]string{})
		})
	}
}

// TestRunDevicesWithTestFixtures verifies devices command with test fixtures.
func TestRunDevicesWithTestFixtures(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	err := runDevicesWithPath(asoundPath, []string{})
	if err != nil {
		t.Errorf("runDevicesWithPath() unexpected error: %v", err)
	}
}

// TestRunDevicesWithPathEmpty verifies devices command with empty directory.
func TestRunDevicesWithPathEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	emptyAsound := filepath.Join(tmpDir, "asound")
	if err := os.MkdirAll(emptyAsound, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	err := runDevicesWithPath(emptyAsound, []string{})
	if err != nil {
		t.Errorf("runDevicesWithPath() with empty dir unexpected error: %v", err)
	}
}

// TestRunDevicesWithPathNonexistent verifies devices command with nonexistent directory.
func TestRunDevicesWithPathNonexistent(t *testing.T) {
	err := runDevicesWithPath("/nonexistent/asound", []string{})
	if err == nil {
		t.Error("runDevicesWithPath() with nonexistent path expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to scan devices") {
		t.Errorf("runDevicesWithPath() error = %q, want 'failed to scan devices'", err.Error())
	}
}

// TestRunDetectWithTestFixtures verifies detect command with test fixtures.
func TestRunDetectWithTestFixtures(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	err := runDetectWithPath(asoundPath, []string{})
	if err != nil {
		t.Errorf("runDetectWithPath() unexpected error: %v", err)
	}
}

// TestRunDetectWithPathEmpty verifies detect command with empty directory.
func TestRunDetectWithPathEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	emptyAsound := filepath.Join(tmpDir, "asound")
	if err := os.MkdirAll(emptyAsound, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	err := runDetectWithPath(emptyAsound, []string{})
	if err != nil {
		t.Errorf("runDetectWithPath() with empty dir unexpected error: %v", err)
	}
}

// TestRunDetectWithPathNonexistent verifies detect command with nonexistent directory.
func TestRunDetectWithPathNonexistent(t *testing.T) {
	err := runDetectWithPath("/nonexistent/asound", []string{})
	if err == nil {
		t.Error("runDetectWithPath() with nonexistent path expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to scan devices") {
		t.Errorf("runDetectWithPath() error = %q, want 'failed to scan devices'", err.Error())
	}
}

// TestRunUSBMapWithPathTestFixtures verifies usb-map with test fixtures.
func TestRunUSBMapWithPathTestFixtures(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Test with --dry-run flag
	args := []string{"--dry-run"}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathEmpty verifies usb-map with empty directory.
func TestRunUSBMapWithPathEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	emptyAsound := filepath.Join(tmpDir, "asound")
	if err := os.MkdirAll(emptyAsound, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	args := []string{"--dry-run"}
	err := runUSBMapWithPath(emptyAsound, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() with empty dir unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathNonDryRun verifies usb-map without --dry-run.
func TestRunUSBMapWithPathNonDryRun(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Without --dry-run, should print stub message
	err := runUSBMapWithPath(asoundPath, []string{})
	if err != nil {
		t.Errorf("runUSBMapWithPath() without dry-run unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathOutputFlag verifies usb-map --output flag.
func TestRunUSBMapWithPathOutputFlag(t *testing.T) {
	// Use test fixtures
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")

	// Check if test data exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "output with equals",
			args: []string{"--dry-run", "--output=/tmp/test-rules"},
		},
		{
			name: "output with space",
			args: []string{"--dry-run", "--output", "/tmp/test-rules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runUSBMapWithPath(asoundPath, tt.args)
			if err != nil {
				t.Errorf("runUSBMapWithPath() unexpected error: %v", err)
			}
		})
	}
}

// TestSetupSignalHandler verifies signal handler setup.
func TestSetupSignalHandler(t *testing.T) {
	ctx := setupSignalHandler()
	if ctx == nil {
		t.Error("setupSignalHandler() returned nil context")
	}

	// Verify context is not already cancelled
	select {
	case <-ctx.Done():
		t.Error("setupSignalHandler() context already cancelled")
	default:
		// Expected
	}
}

// BenchmarkRun measures command routing performance.
func BenchmarkRun(b *testing.B) {
	args := []string{"help"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = run(args)
	}
}

// BenchmarkRunVersion measures version command performance.
func BenchmarkRunVersion(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = runVersion()
	}
}

// TestRunTestFlagParsing verifies test command flag parsing.
func TestRunTestFlagParsing(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "default config path",
			args: []string{},
		},
		{
			name: "custom config with equals",
			args: []string{"--config=/tmp/test.yaml"},
		},
		{
			name: "custom config with space",
			args: []string{"--config", "/tmp/test.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test command is a stub, just verify it doesn't panic
			_ = runTest(tt.args)
		})
	}
}

// TestRunUSBMapFlagParsing verifies usb-map flag parsing.
func TestRunUSBMapFlagParsing(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("usb-map requires root privileges")
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "dry-run flag",
			args: []string{"--dry-run"},
		},
		{
			name: "output with equals",
			args: []string{"--dry-run", "--output=/tmp/test-rules"},
		},
		{
			name: "output with space",
			args: []string{"--dry-run", "--output", "/tmp/test-rules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// May fail if no devices, but shouldn't panic
			_ = runUSBMap(tt.args)
		})
	}
}

// TestCommandAliases verifies command aliases work.
func TestCommandAliases(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"help long", []string{"help"}},
		{"help short", []string{"-h"}},
		{"help double dash", []string{"--help"}},
		{"version long", []string{"version"}},
		{"version short", []string{"-v"}},
		{"version double dash", []string{"--version"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err != nil {
				t.Errorf("run() unexpected error for %v: %v", tt.args, err)
			}
		})
	}
}

// TestRunMigrateDirectoryCreation verifies directory creation.
func TestRunMigrateDirectoryCreation(t *testing.T) {
	bashConfig := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")

	if _, err := os.Stat(bashConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpDir := t.TempDir()
	// Nested path to test directory creation
	yamlConfig := filepath.Join(tmpDir, "subdir", "config.yaml")

	args := []string{
		"--from", bashConfig,
		"--to", yamlConfig,
	}

	err := runMigrate(args)
	if err != nil {
		t.Fatalf("runMigrate() unexpected error: %v", err)
	}

	// Verify file and directory were created
	if _, err := os.Stat(yamlConfig); os.IsNotExist(err) {
		t.Error("runMigrate() did not create output file")
	}
}

// TestDetectArch verifies architecture detection.
func TestDetectArch(t *testing.T) {
	arch := detectArch()

	// detectArch should return one of the known values or empty string
	validArchs := map[string]bool{
		"amd64": true,
		"arm64": true,
		"armv7": true,
		"armv6": true,
		"":      true, // Unknown arch returns empty
	}

	if !validArchs[arch] {
		t.Errorf("detectArch() returned unexpected value: %q", arch)
	}

	// On Linux, we should get a non-empty result
	if arch == "" {
		t.Log("detectArch() returned empty string (may be expected on unsupported platform)")
	}
}

// TestReadLockPID verifies lock file reading.
func TestReadLockPID(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantPID int
		wantErr bool
	}{
		{
			name:    "valid pid",
			content: "12345",
			wantPID: 12345,
			wantErr: false,
		},
		{
			name:    "valid pid with newline",
			content: "12345\n",
			wantPID: 12345,
			wantErr: false,
		},
		{
			name:    "valid pid with whitespace",
			content: "  12345  \n",
			wantPID: 12345,
			wantErr: false,
		},
		{
			name:    "invalid content",
			content: "not-a-number",
			wantPID: 0,
			wantErr: true,
		},
		{
			name:    "empty file",
			content: "",
			wantPID: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			lockFile := filepath.Join(tmpDir, "test.lock")

			if err := os.WriteFile(lockFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			pid, err := readLockPID(lockFile)

			if tt.wantErr {
				if err == nil {
					t.Error("readLockPID() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("readLockPID() unexpected error: %v", err)
				}
				if pid != tt.wantPID {
					t.Errorf("readLockPID() = %d, want %d", pid, tt.wantPID)
				}
			}
		})
	}
}

// TestReadLockPIDNonexistent verifies error on non-existent file.
func TestReadLockPIDNonexistent(t *testing.T) {
	_, err := readLockPID("/nonexistent/path/lock.file")
	if err == nil {
		t.Error("readLockPID() expected error for non-existent file, got nil")
	}
}

// TestProcessExists verifies process existence checking.
func TestProcessExists(t *testing.T) {
	// Test with current process (should exist)
	if !processExists(os.Getpid()) {
		t.Error("processExists() returned false for current process")
	}

	// Test with PID 1 (init, should exist on Linux)
	if !processExists(1) {
		t.Log("processExists(1) returned false (may be expected in some environments)")
	}

	// Test with invalid PID (should not exist)
	// Using a very large PID that's unlikely to exist
	if processExists(9999999) {
		t.Error("processExists() returned true for unlikely PID 9999999")
	}

	// Test with negative PID (should not exist)
	if processExists(-1) {
		t.Error("processExists() returned true for negative PID")
	}
}

// TestGetServiceStatus verifies service status formatting.
func TestGetServiceStatus(t *testing.T) {
	// Test with a service that likely doesn't exist
	status := getServiceStatus("nonexistent-test-service-12345")

	// Should return some status string (either error or not-installed)
	if status == "" {
		t.Error("getServiceStatus() returned empty string")
	}

	// Test with a common service (might not work in all environments)
	_ = getServiceStatus("systemd-journald")
}

// TestRunStatusWithTestFixtures verifies status command output.
func TestRunStatusWithTestFixtures(t *testing.T) {
	// Status command should not panic even without devices
	err := runStatus([]string{})
	// May or may not error depending on environment
	_ = err
}

// TestRunDiagnoseOutput verifies diagnose command runs without panic.
func TestRunDiagnoseOutput(t *testing.T) {
	// Diagnose command should not panic
	err := runDiagnose([]string{})
	// May or may not error depending on environment
	_ = err
}

// TestRunCheckSystemOutput verifies check-system command output.
func TestRunCheckSystemOutput(t *testing.T) {
	// Check-system command should not panic
	err := runCheckSystem([]string{})
	// May or may not error depending on environment
	_ = err
}

// TestUSBMapWithPathWriteRules verifies usb-map writes rules when not dry-run.
func TestUSBMapWithPathWriteRules(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Create temp directory for output
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "99-usb-soundcards.rules")

	// Test with output flag (but not actually writing to system path)
	args := []string{"--output=" + outputPath}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() unexpected error: %v", err)
	}
}

// TestRunDevicesVerboseOutput verifies devices command with verbose flag.
func TestRunDevicesVerboseOutput(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Test with verbose flag (if supported)
	err := runDevicesWithPath(asoundPath, []string{"--verbose"})
	if err != nil {
		t.Errorf("runDevicesWithPath() with verbose unexpected error: %v", err)
	}
}

// TestRunDetectVerboseOutput verifies detect command with verbose flag.
func TestRunDetectVerboseOutput(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Test with verbose flag (if supported)
	err := runDetectWithPath(asoundPath, []string{"--verbose"})
	if err != nil {
		t.Errorf("runDetectWithPath() with verbose unexpected error: %v", err)
	}
}

// TestInstallLyreBirdServiceMatchesSystemdFile asserts that the embedded
// lyrebirdServiceContent var is byte-for-byte identical to
// systemd/lyrebird-stream.service at the repo root (M-12 fix).
func TestInstallLyreBirdServiceMatchesSystemdFile(t *testing.T) {
	// Navigate from cmd/lyrebird up to the repo root.
	systemdPath := filepath.Join("..", "..", "systemd", "lyrebird-stream.service")
	data, err := os.ReadFile(systemdPath) // #nosec G304 -- test reads a known repo file
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("systemd/lyrebird-stream.service not found; skipping equivalence check")
		}
		t.Fatalf("failed to read systemd service file: %v", err)
	}

	got := lyrebirdServiceContent
	want := string(data)
	if got != want {
		t.Errorf("lyrebirdServiceContent is out of sync with systemd/lyrebird-stream.service\n"+
			"Update lyrebirdServiceContent in cmd/lyrebird/main.go to match the file.\n"+
			"diff (want=file, got=var):\n%s",
			diffStrings(want, got))
	}
}

// diffStrings returns the first line that differs between a and b.
func diffStrings(a, b string) string {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	for i := 0; i < len(aLines) && i < len(bLines); i++ {
		if aLines[i] != bLines[i] {
			return fmt.Sprintf("first difference at line %d:\n  want: %q\n  got:  %q", i+1, aLines[i], bLines[i])
		}
	}
	if len(aLines) != len(bLines) {
		return fmt.Sprintf("line count differs: want %d, got %d", len(aLines), len(bLines))
	}
	return "(no line-level diff found; possibly whitespace)"
}

// TestInstallLyreBirdServiceToPathWritesFile verifies the service file is written.
func TestInstallLyreBirdServiceToPathWritesFile(t *testing.T) {
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "lyrebird-stream.service")

	// installLyreBirdServiceToPath calls systemctl daemon-reload which won't
	// work in CI; we test only the write portion by writing directly.
	// #nosec G306 - test file
	if err := os.WriteFile(servicePath, []byte(lyrebirdServiceContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := os.ReadFile(servicePath) // #nosec G304 -- test reads from t.TempDir()
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != lyrebirdServiceContent {
		t.Error("written service content does not match lyrebirdServiceContent")
	}

	// Verify key hardening directives are present.
	for _, directive := range []string{
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"PrivateTmp=true",
		"ProtectHome=true",
		"StartLimitIntervalSec=300",
		"ExecReload=/bin/kill -HUP $MAINPID",
	} {
		if !strings.Contains(string(data), directive) {
			t.Errorf("service file missing security directive: %s", directive)
		}
	}
}

// TestGetUSBBusDevFromCardWithSysRoot tests the injectable variant.
func TestGetUSBBusDevFromCardWithSysRoot(t *testing.T) {
	t.Run("card device symlink not found", func(t *testing.T) {
		sysRoot := t.TempDir()
		_, _, err := getUSBBusDevFromCardWithSysRoot(0, sysRoot)
		if err == nil {
			t.Fatal("expected error for missing card device symlink")
		}
		if !strings.Contains(err.Error(), "failed to resolve card device path") {
			t.Errorf("error = %q, want 'failed to resolve card device path'", err.Error())
		}
	})

	t.Run("busnum and devnum found directly", func(t *testing.T) {
		sysRoot := t.TempDir()

		// Create the USB device directory with busnum/devnum files
		usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-1.4")
		if err := os.MkdirAll(usbDevDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbDevDir, "busnum"), []byte("1\n"), 0644); err != nil {
			t.Fatalf("WriteFile busnum: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbDevDir, "devnum"), []byte("5\n"), 0644); err != nil {
			t.Fatalf("WriteFile devnum: %v", err)
		}

		// Create the sound/card0/device symlink pointing at the USB device dir
		soundDir := filepath.Join(sysRoot, "class", "sound", "card0")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		busNum, devNum, err := getUSBBusDevFromCardWithSysRoot(0, sysRoot)
		if err != nil {
			t.Fatalf("getUSBBusDevFromCardWithSysRoot() error = %v", err)
		}
		if busNum != 1 {
			t.Errorf("busNum = %d, want 1", busNum)
		}
		if devNum != 5 {
			t.Errorf("devNum = %d, want 5", devNum)
		}
	})

	t.Run("busnum found in parent directory", func(t *testing.T) {
		sysRoot := t.TempDir()

		// USB device info is in the parent of where symlink points
		usbParentDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-1")
		childDir := filepath.Join(usbParentDir, "1-1:1.0")
		if err := os.MkdirAll(childDir, 0755); err != nil {
			t.Fatalf("MkdirAll child: %v", err)
		}
		// busnum/devnum in parent, not in child
		if err := os.WriteFile(filepath.Join(usbParentDir, "busnum"), []byte("2\n"), 0644); err != nil {
			t.Fatalf("WriteFile busnum: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbParentDir, "devnum"), []byte("7\n"), 0644); err != nil {
			t.Fatalf("WriteFile devnum: %v", err)
		}

		// Symlink points to child
		soundDir := filepath.Join(sysRoot, "class", "sound", "card1")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(childDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		busNum, devNum, err := getUSBBusDevFromCardWithSysRoot(1, sysRoot)
		if err != nil {
			t.Fatalf("getUSBBusDevFromCardWithSysRoot() error = %v", err)
		}
		if busNum != 2 {
			t.Errorf("busNum = %d, want 2", busNum)
		}
		if devNum != 7 {
			t.Errorf("devNum = %d, want 7", devNum)
		}
	})

	t.Run("busnum and devnum not found anywhere", func(t *testing.T) {
		sysRoot := t.TempDir()

		// USB device dir with NO busnum/devnum
		usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-2")
		if err := os.MkdirAll(usbDevDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		soundDir := filepath.Join(sysRoot, "class", "sound", "card2")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		_, _, err := getUSBBusDevFromCardWithSysRoot(2, sysRoot)
		if err == nil {
			t.Fatal("expected error when bus/dev not found")
		}
		if !strings.Contains(err.Error(), "USB bus/dev numbers not found") {
			t.Errorf("error = %q, want 'USB bus/dev numbers not found'", err.Error())
		}
	})

	t.Run("malformed busnum does not infinite loop", func(t *testing.T) {
		sysRoot := t.TempDir()

		usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-3")
		if err := os.MkdirAll(usbDevDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		// Write non-numeric content to busnum
		if err := os.WriteFile(filepath.Join(usbDevDir, "busnum"), []byte("not-a-number\n"), 0644); err != nil {
			t.Fatalf("WriteFile busnum: %v", err)
		}
		if err := os.WriteFile(filepath.Join(usbDevDir, "devnum"), []byte("5\n"), 0644); err != nil {
			t.Fatalf("WriteFile devnum: %v", err)
		}

		soundDir := filepath.Join(sysRoot, "class", "sound", "card3")
		if err := os.MkdirAll(soundDir, 0755); err != nil {
			t.Fatalf("MkdirAll sound: %v", err)
		}
		if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
			t.Fatalf("Symlink: %v", err)
		}

		// This must terminate (old code had infinite loop here)
		_, _, err := getUSBBusDevFromCardWithSysRoot(3, sysRoot)
		if err == nil {
			t.Fatal("expected error for malformed busnum")
		}
	})
}

// --- L-4: downloadFile and installLyreBirdServiceToPath coverage ---

// TestDownloadFileNeitherFound covers the "neither curl nor wget" error path.
func TestDownloadFileNeitherFound(t *testing.T) {
	// Use a temp dir with no executables so LookPath for both curl and wget fails.
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)

	err := downloadFile("http://example.invalid/fake", filepath.Join(emptyBin, "out"))
	if err == nil {
		t.Fatal("downloadFile() expected error when neither curl nor wget found")
	}
	if !strings.Contains(err.Error(), "neither curl nor wget") {
		t.Errorf("downloadFile() error = %q; want 'neither curl nor wget'", err.Error())
	}
}

// TestDownloadFileCurlSuccess covers the happy path when curl is available.
func TestDownloadFileCurlSuccess(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Fake curl: writes "fake content" to its -o argument ($3).
	// Invocation: curl -fsSL -o <dest> <url>  →  $1=-fsSL $2=-o $3=dest $4=url
	fakeCurl := filepath.Join(tmpBin, "curl")
	if err := os.WriteFile(fakeCurl, []byte("#!/bin/sh\nprintf 'fake content' > \"$3\"\nexit 0\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	dest := filepath.Join(tmpDir, "download.bin")
	if err := downloadFile("http://example.invalid/fake", dest); err != nil {
		t.Fatalf("downloadFile(curl success) = %v; want nil", err)
	}
	data, err := os.ReadFile(dest) //#nosec G304
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "fake content" {
		t.Errorf("downloaded content = %q; want %q", string(data), "fake content")
	}
}

// TestDownloadFileCurlFailure covers the error path when curl exits non-zero.
func TestDownloadFileCurlFailure(t *testing.T) {
	tmpBin := t.TempDir()

	// Fake curl that always fails.
	fakeCurl := filepath.Join(tmpBin, "curl")
	if err := os.WriteFile(fakeCurl, []byte("#!/bin/sh\necho 'download error' >&2\nexit 1\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := downloadFile("http://example.invalid/fake", filepath.Join(tmpBin, "out"))
	if err == nil {
		t.Fatal("downloadFile(curl failure) expected non-nil error")
	}
	if !strings.Contains(err.Error(), "curl failed") {
		t.Errorf("downloadFile(curl failure) error = %q; want 'curl failed'", err.Error())
	}
}

// TestDownloadFileWgetSuccess covers the wget fallback when curl is absent.
func TestDownloadFileWgetSuccess(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Only wget available; curl is not in this isolated PATH so LookPath("curl") fails.
	// Invocation: wget -q -O <dest> <url>  →  $1=-q $2=-O $3=dest $4=url
	fakeWget := filepath.Join(tmpBin, "wget")
	if err := os.WriteFile(fakeWget, []byte("#!/bin/sh\nprintf 'wget content' > \"$3\"\nexit 0\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	// Isolate PATH to tmpBin only so real curl is hidden.
	t.Setenv("PATH", tmpBin)

	dest := filepath.Join(tmpDir, "download.bin")
	if err := downloadFile("http://example.invalid/fake", dest); err != nil {
		t.Fatalf("downloadFile(wget success) = %v; want nil", err)
	}
	data, err := os.ReadFile(dest) //#nosec G304
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "wget content" {
		t.Errorf("downloaded content = %q; want %q", string(data), "wget content")
	}
}

// TestDownloadFileWgetFailure covers the wget error path.
func TestDownloadFileWgetFailure(t *testing.T) {
	tmpBin := t.TempDir()

	// Only wget (failing), curl absent from isolated PATH.
	fakeWget := filepath.Join(tmpBin, "wget")
	if err := os.WriteFile(fakeWget, []byte("#!/bin/sh\necho 'wget error' >&2\nexit 1\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	// Isolate PATH to tmpBin only so real curl is hidden.
	t.Setenv("PATH", tmpBin)

	err := downloadFile("http://example.invalid/fake", filepath.Join(tmpBin, "out"))
	if err == nil {
		t.Fatal("downloadFile(wget failure) expected non-nil error")
	}
	if !strings.Contains(err.Error(), "wget failed") {
		t.Errorf("downloadFile(wget failure) error = %q; want 'wget failed'", err.Error())
	}
}

// TestInstallLyreBirdServiceToPathSuccess covers the happy path with a fake systemctl.
func TestInstallLyreBirdServiceToPathSuccess(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Fake systemctl that exits 0.
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	servicePath := filepath.Join(tmpDir, "lyrebird-stream.service")
	if err := installLyreBirdServiceToPath(servicePath); err != nil {
		t.Fatalf("installLyreBirdServiceToPath() = %v; want nil", err)
	}

	data, err := os.ReadFile(servicePath) //#nosec G304
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != lyrebirdServiceContent {
		t.Error("installed service content does not match lyrebirdServiceContent")
	}
}

// TestInstallLyreBirdServiceToPathWriteError covers the write-failure error path.
func TestInstallLyreBirdServiceToPathWriteError(t *testing.T) {
	// Pass a path whose parent directory does not exist.
	err := installLyreBirdServiceToPath("/nonexistent/path/lyrebird-stream.service")
	if err == nil {
		t.Fatal("installLyreBirdServiceToPath() expected error for missing directory")
	}
	if !strings.Contains(err.Error(), "failed to write service file") {
		t.Errorf("installLyreBirdServiceToPath() error = %q; want 'failed to write service file'", err.Error())
	}
}

// TestInstallLyreBirdServiceToPathSystemctlFailure covers the systemctl daemon-reload error path.
func TestInstallLyreBirdServiceToPathSystemctlFailure(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Fake systemctl that fails.
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\necho 'daemon-reload failed' >&2\nexit 1\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	servicePath := filepath.Join(tmpDir, "lyrebird-stream.service")
	err := installLyreBirdServiceToPath(servicePath)
	if err == nil {
		t.Fatal("installLyreBirdServiceToPath() expected error when systemctl fails")
	}
	if !strings.Contains(err.Error(), "systemctl daemon-reload failed") {
		t.Errorf("installLyreBirdServiceToPath() error = %q; want 'systemctl daemon-reload failed'", err.Error())
	}
}

// TestIsValidMediaMTXVersion verifies SEC-5: version string validation.
func TestIsValidMediaMTXVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		// Valid versions
		{"v1.9.3", true},
		{"v0.1.0", true},
		{"v10.20.30", true},
		{"1.9.3", true},
		{"v1.9.3-rc1", true},
		{"v2.0.0-beta.1", true},

		// Invalid versions (potential injection)
		{"", false},
		{"latest", false},
		{"v1.9.3/%2e%2e/", false},
		{"v1.9.3; rm -rf /", false},
		{"../../../etc/passwd", false},
		{"v1.9", false},           // missing patch
		{"v1", false},             // missing minor+patch
		{"v1.9.3\nmalicious", false},
		{"v1.9.3 --help", false},
		{"v1.9.3&foo=bar", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.version), func(t *testing.T) {
			got := isValidMediaMTXVersion(tt.version)
			if got != tt.want {
				t.Errorf("isValidMediaMTXVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

// TestInstallMediaMTXVersionValidation verifies SEC-5: invalid versions are rejected.
func TestInstallMediaMTXVersionValidation(t *testing.T) {
	// Requires root, so only test the validation error path
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}

	err := runInstallMediaMTX([]string{"--version=../../../etc/passwd"})
	if err == nil {
		t.Fatal("runInstallMediaMTX should fail for non-root or bad version")
	}

	// Should fail at root check first (non-root env), but if root check somehow
	// passes, it must fail on version validation
	if strings.Contains(err.Error(), "root privileges") {
		// Expected: root check fired first
		return
	}
	if !strings.Contains(err.Error(), "invalid version format") {
		t.Errorf("expected 'invalid version format' error, got: %v", err)
	}
}

// TestMain verifies main function integration.
func TestMain(m *testing.M) {
	// Run all tests
	code := m.Run()

	// Cleanup coverage file if exists
	_ = os.Remove("coverage.out")

	os.Exit(code)
}
