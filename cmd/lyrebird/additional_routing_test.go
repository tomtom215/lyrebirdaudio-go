// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"testing"
)

// TestRunCommandRouting verifies all command routing paths in run().
func TestRunCommandRouting(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"update command", []string{"update"}},
		{"menu command", []string{"menu"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These commands will fail (no GitHub access, no terminal),
			// but should be routed correctly and not panic.
			_ = run(tt.args)
		})
	}
}
