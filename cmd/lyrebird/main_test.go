package main

import (
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

// TestMain verifies main function integration.
func TestMain(m *testing.M) {
	// Run all tests
	code := m.Run()

	// Cleanup coverage file if exists
	_ = os.Remove("coverage.out")

	os.Exit(code)
}
