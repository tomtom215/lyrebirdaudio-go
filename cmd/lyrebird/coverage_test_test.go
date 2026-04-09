// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunTestWithValidConfig verifies the test command with a valid config.
func TestRunTestWithValidConfig(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Test with valid config - will proceed past config check
	err := runTest([]string{"--config=" + configPath})
	// Should succeed (some sub-tests may warn but not error)
	if err != nil {
		t.Errorf("runTest() with valid config unexpected error: %v", err)
	}
}

// TestRunTestWithVerboseFlag verifies the test command with verbose output.
func TestRunTestWithVerboseFlag(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	err := runTest([]string{"--config=" + configPath, "--verbose"})
	if err != nil {
		t.Errorf("runTest() with verbose flag unexpected error: %v", err)
	}
}

// TestRunTestWithShortVerboseFlag verifies the test command with -v flag.
func TestRunTestWithShortVerboseFlag(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	err := runTest([]string{"--config=" + configPath, "-v"})
	if err != nil {
		t.Errorf("runTest() with -v flag unexpected error: %v", err)
	}
}

// TestRunTestInvalidConfig verifies the test command fails gracefully with invalid config.
func TestRunTestInvalidConfig(t *testing.T) {
	err := runTest([]string{"--config=/nonexistent/config.yaml"})
	if err == nil {
		t.Error("runTest() expected error for nonexistent config, got nil")
	}
	if !strings.Contains(err.Error(), "config test failed") {
		t.Errorf("runTest() error = %q, want substring 'config test failed'", err.Error())
	}
}

// TestRunTestConfigWithSpaceFlag verifies --config flag with space separator.
func TestRunTestConfigWithSpaceFlag(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	err := runTest([]string{"--config", configPath})
	if err != nil {
		t.Errorf("runTest() with --config space flag unexpected error: %v", err)
	}
}
