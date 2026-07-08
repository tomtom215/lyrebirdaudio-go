package main

import (
	"strings"
	"testing"
)

// TestRunRouting verifies command dispatch with deterministic, environment-
// independent outcomes: help/version succeed, an unknown command is rejected,
// and arg-validation errors fire before any filesystem or device access.
func TestRunRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{name: "no arguments shows help", args: []string{}, wantErr: false},
		{name: "help command", args: []string{"help"}, wantErr: false},
		{name: "version command", args: []string{"version"}, wantErr: false},
		{name: "unknown command", args: []string{"unknown-command"}, wantErr: true, errMsg: "unknown command"},
		{name: "migrate without --from flag", args: []string{"migrate"}, wantErr: true, errMsg: "--from path is required"},
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
			} else if err != nil {
				t.Errorf("run() unexpected error: %v", err)
			}
		})
	}
}

// TestRunDispatchesKnownCommands verifies every command routes to a real
// handler rather than falling through to the "unknown command" default. It
// deliberately does NOT assert on the handler's success/failure: those outcomes
// depend on the environment (presence of /proc/asound, ffmpeg, a config file, a
// running server), and asserting them couples the routing test to the host.
func TestRunDispatchesKnownCommands(t *testing.T) {
	commands := []string{
		"devices", "detect", "status", "test", "diagnose", "check-system", "validate",
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			// run() must not panic and, whatever the env-dependent outcome, must
			// not report the command as unrouted.
			if err := run([]string{cmd}); err != nil && strings.Contains(err.Error(), "unknown command") {
				t.Errorf("command %q was not routed to a handler: %v", cmd, err)
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
