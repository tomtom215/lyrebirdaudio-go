package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
