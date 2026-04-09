// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"testing"
)

// TestRunStatusWithNonexistentLockDir verifies status with missing lock dir.
func TestRunStatusWithNonexistentLockDir(t *testing.T) {
	args := []string{"--lock-dir=/nonexistent/lock/dir"}
	err := runStatus(args)
	if err != nil {
		t.Errorf("runStatus() with nonexistent lock dir unexpected error: %v", err)
	}
}

// TestRunStatusFlagParsing verifies all flag parsing combinations.
func TestRunStatusFlagParsing(t *testing.T) {
	lockDir := t.TempDir()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "all flags combined",
			args: []string{
				"--lock-dir=" + lockDir,
				"--config=/nonexistent/config.yaml",
				"--json",
			},
		},
		{
			name: "lock-dir only",
			args: []string{"--lock-dir=" + lockDir},
		},
		{
			name: "json short flag with lock dir",
			args: []string{"--lock-dir=" + lockDir, "-j"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runStatus(tt.args)
			if err != nil {
				t.Errorf("runStatus() unexpected error: %v", err)
			}
		})
	}
}
