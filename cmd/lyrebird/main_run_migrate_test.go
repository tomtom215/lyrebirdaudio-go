package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
