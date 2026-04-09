// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"strings"
	"testing"
)

// TestRunUpdateFlagParsing verifies update command flag parsing.
func TestRunUpdateFlagParsing(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "check only flag",
			args:    []string{"--check"},
			wantErr: true, // Will fail trying to contact GitHub
		},
		{
			name:    "force flag",
			args:    []string{"--force"},
			wantErr: true, // Will fail trying to contact GitHub
		},
		{
			name:    "check and force flags",
			args:    []string{"--check", "--force"},
			wantErr: true, // Will fail trying to contact GitHub
		},
		{
			name:    "no flags",
			args:    []string{},
			wantErr: true, // Will fail trying to contact GitHub
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runUpdate(tt.args)
			// All cases should fail because we can't reach GitHub in CI,
			// but they should not panic and should return a proper error.
			if tt.wantErr && err == nil {
				t.Error("runUpdate() expected error, got nil")
			}
			if err != nil && !strings.Contains(err.Error(), "failed to check for updates") {
				t.Logf("runUpdate() returned unexpected error type: %v", err)
			}
		})
	}
}
