// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckMediaMTXServiceFound verifies paths when mediamtx is installed.
func TestCheckMediaMTXServiceFound(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake mediamtx binary (just needs to exist in PATH).
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "mediamtx"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake mediamtx: %v", err)
	}
	// Fake systemctl: reports mediamtx as "active".
	systemctlScript := "#!/bin/sh\necho 'active'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "systemctl"), []byte(systemctlScript), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake systemctl: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkMediaMTXService(context.Background())

	if result.Name != "MediaMTX Service" {
		t.Errorf("Name = %q, want %q", result.Name, "MediaMTX Service")
	}
	// mediamtx binary found + systemctl reports active → StatusOK.
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when mediamtx active; msg: %s", result.Status, result.Message)
	}
}

// TestCheckMediaMTXServiceInactive verifies the StatusWarning path when mediamtx
// is installed but the systemd service is not running.
func TestCheckMediaMTXServiceInactive(t *testing.T) {
	tmpBin := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "mediamtx"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake mediamtx: %v", err)
	}
	// Fake systemctl: exits 1 (service not running) so cmd.Output() returns error.
	systemctlScript := "#!/bin/sh\necho 'inactive'\nexit 3\n"                                               // systemctl exit 3 = inactive
	if err := os.WriteFile(filepath.Join(tmpBin, "systemctl"), []byte(systemctlScript), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake systemctl: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkMediaMTXService(context.Background())

	// systemctl exits non-zero → StatusWarning "not running".
	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when systemctl fails; msg: %s", result.Status, result.Message)
	}
}
