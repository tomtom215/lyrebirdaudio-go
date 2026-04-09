package main

import (
	"os"
	"strings"
	"testing"
)

// TestRunSetupRootCheck verifies setup root privilege check.
func TestRunSetupRootCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Test must run as non-root")
	}

	err := runSetup([]string{})
	if err == nil {
		t.Error("runSetup() expected error for non-root user")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Errorf("runSetup() error = %q, want 'root privileges'", err.Error())
	}
}

// TestRunInstallMediaMTXRootCheck verifies install-mediamtx root privilege check.
func TestRunInstallMediaMTXRootCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Test must run as non-root")
	}

	err := runInstallMediaMTX([]string{})
	if err == nil {
		t.Error("runInstallMediaMTX() expected error for non-root user")
	}
	if !strings.Contains(err.Error(), "root privileges") {
		t.Errorf("runInstallMediaMTX() error = %q, want 'root privileges'", err.Error())
	}
}

// TestStubCommands verifies stub commands don't panic.
func TestStubCommands(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]string) error
	}{
		{"status", runStatus},
		{"setup", func(args []string) error {
			if os.Geteuid() != 0 {
				return nil // Skip root check for test
			}
			return runSetup(args)
		}},
		{"install-mediamtx", func(args []string) error {
			if os.Geteuid() != 0 {
				return nil // Skip root check for test
			}
			return runInstallMediaMTX(args)
		}},
		{"test", runTest},
		{"diagnose", runDiagnose},
		{"check-system", runCheckSystem},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify they don't panic
			_ = tt.fn([]string{})
		})
	}
}

// TestCommandAliases verifies command aliases work.
func TestCommandAliases(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"help long", []string{"help"}},
		{"help short", []string{"-h"}},
		{"help double dash", []string{"--help"}},
		{"version long", []string{"version"}},
		{"version short", []string{"-v"}},
		{"version double dash", []string{"--version"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err != nil {
				t.Errorf("run() unexpected error for %v: %v", tt.args, err)
			}
		})
	}
}
