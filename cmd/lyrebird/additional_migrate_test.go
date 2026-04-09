// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunMigrateToFlagParsing verifies --to flag with equals form.
func TestRunMigrateToFlagParsing(t *testing.T) {
	bashConfig := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")
	if _, err := os.Stat(bashConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpDir := t.TempDir()
	yamlConfig := filepath.Join(tmpDir, "config.yaml")

	// Test --to with equals form
	args := []string{
		"--from=" + bashConfig,
		"--to=" + yamlConfig,
	}

	err := runMigrate(args)
	if err != nil {
		t.Fatalf("runMigrate() unexpected error: %v", err)
	}

	if _, err := os.Stat(yamlConfig); os.IsNotExist(err) {
		t.Error("output file was not created")
	}
}

// TestRunValidateWithDevices verifies validate output when devices are configured.
func TestRunValidateWithDevices(t *testing.T) {
	validConfig := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(validConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Test with equals form
	err := runValidate([]string{"--config=" + validConfig})
	if err != nil {
		t.Errorf("runValidate() unexpected error: %v", err)
	}
}
