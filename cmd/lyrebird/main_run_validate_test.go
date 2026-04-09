package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
