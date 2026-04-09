package main

import (
	"strings"
	"testing"
)

// TestRun verifies basic command routing.
func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no arguments shows help",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "help command",
			args:    []string{"help"},
			wantErr: false,
		},
		{
			name:    "version command",
			args:    []string{"version"},
			wantErr: false,
		},
		{
			name:    "unknown command",
			args:    []string{"unknown-command"},
			wantErr: true,
			errMsg:  "unknown command",
		},
		{
			name:    "validate without args uses default path",
			args:    []string{"validate"},
			wantErr: true, // Will fail because default config doesn't exist in test
		},
		{
			name:    "migrate without --from flag",
			args:    []string{"migrate"},
			wantErr: true,
			errMsg:  "--from path is required",
		},
		{
			name:    "devices command",
			args:    []string{"devices"},
			wantErr: true, // Will fail because /proc/asound doesn't exist in test
		},
		{
			name:    "detect command",
			args:    []string{"detect"},
			wantErr: true, // Will fail because /proc/asound doesn't exist in test
		},
		{
			name:    "status command (stub)",
			args:    []string{"status"},
			wantErr: false, // Stub command doesn't error
		},
		{
			name:    "test command (needs config)",
			args:    []string{"test"},
			wantErr: true, // Will fail because default config doesn't exist in test
		},
		{
			name:    "diagnose command (stub)",
			args:    []string{"diagnose"},
			wantErr: false, // Stub command doesn't error
		},
		{
			name:    "check-system command (stub)",
			args:    []string{"check-system"},
			wantErr: false, // Stub command doesn't error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Error("run() expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("run() error = %q, want substring %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("run() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRunHelp verifies help command output.
func TestRunHelp(t *testing.T) {
	err := runHelp()
	if err != nil {
		t.Errorf("runHelp() unexpected error: %v", err)
	}
}

// TestRunVersion verifies version command output.
func TestRunVersion(t *testing.T) {
	// Set version info for test
	Version = "test-version"
	GitCommit = "test-commit"
	BuildDate = "test-date"

	err := runVersion()
	if err != nil {
		t.Errorf("runVersion() unexpected error: %v", err)
	}
}
