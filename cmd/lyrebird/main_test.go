package main

import (
	"os"
	"testing"
)

// TestMain verifies main function integration.
func TestMain(m *testing.M) {
	// Run all tests
	code := m.Run()

	// Cleanup coverage file if exists
	_ = os.Remove("coverage.out")

	os.Exit(code)
}
