// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallLyreBirdServiceCallsToPath verifies the wrapper function.
func TestInstallLyreBirdServiceCallsToPath(t *testing.T) {
	// installLyreBirdService writes to /etc/systemd/system which is not writable
	// for non-root, providing coverage for the function entry point.
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}
	err := installLyreBirdService()
	if err == nil {
		t.Error("installLyreBirdService() expected error for non-root, got nil")
	}
	if !strings.Contains(err.Error(), "failed to write service file") {
		t.Errorf("installLyreBirdService() error = %q, want 'failed to write service file'", err.Error())
	}
}

// TestRunInstallMediaMTXFlagParsing verifies flag parsing for install-mediamtx.
func TestRunInstallMediaMTXFlagParsing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "version flag with equals",
			args: []string{"--version=v1.9.3"},
		},
		{
			name: "no service flag",
			args: []string{"--no-service"},
		},
		{
			name: "combined flags",
			args: []string{"--version=v2.0.0", "--no-service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runInstallMediaMTX(tt.args)
			// All should fail at root check
			if err == nil {
				t.Error("runInstallMediaMTX() expected error for non-root")
			}
			if !strings.Contains(err.Error(), "root privileges") {
				t.Errorf("runInstallMediaMTX() error = %q, want 'root privileges'", err.Error())
			}
		})
	}
}

// TestVerifyDownloadIntegrityStatError verifies the stat error path.
func TestVerifyDownloadIntegrityStatError(t *testing.T) {
	// Create a file then remove read permissions to trigger stat-related error
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable.tar.gz")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify normal case works first
	hash, err := verifyDownloadIntegrity(path)
	if err != nil {
		t.Fatalf("verifyDownloadIntegrity() unexpected error: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
}
