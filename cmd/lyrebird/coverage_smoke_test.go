// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

// TestRunCheckSystemSmoke verifies check-system runs without panic.
func TestRunCheckSystemSmoke(t *testing.T) {
	// Should not panic regardless of environment
	err := runCheckSystem([]string{})
	if err != nil {
		t.Errorf("runCheckSystem() unexpected error: %v", err)
	}
}
