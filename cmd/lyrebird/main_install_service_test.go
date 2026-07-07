package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallLyreBirdServiceMatchesSystemdFile asserts that the embedded
// lyrebirdServiceContent var is byte-for-byte identical to
// systemd/lyrebird-stream.service at the repo root (M-12 fix).
func TestInstallLyreBirdServiceMatchesSystemdFile(t *testing.T) {
	// Navigate from cmd/lyrebird up to the repo root.
	systemdPath := filepath.Join("..", "..", "systemd", "lyrebird-stream.service")
	data, err := os.ReadFile(systemdPath) // #nosec G304 -- test reads a known repo file
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("systemd/lyrebird-stream.service not found; skipping equivalence check")
		}
		t.Fatalf("failed to read systemd service file: %v", err)
	}

	got := lyrebirdServiceContent
	want := string(data)
	if got != want {
		t.Errorf("lyrebirdServiceContent is out of sync with systemd/lyrebird-stream.service\n"+
			"Update lyrebirdServiceContent in cmd/lyrebird/main.go to match the file.\n"+
			"diff (want=file, got=var):\n%s",
			diffStrings(want, got))
	}
}

// TestInstallLyreBirdServiceToPathWritesFile verifies the service file is written.
func TestInstallLyreBirdServiceToPathWritesFile(t *testing.T) {
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "lyrebird-stream.service")

	// installLyreBirdServiceToPath calls systemctl daemon-reload which won't
	// work in CI; we test only the write portion by writing directly.
	// #nosec G306 - test file
	if err := os.WriteFile(servicePath, []byte(lyrebirdServiceContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := os.ReadFile(servicePath) // #nosec G304 -- test reads from t.TempDir()
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != lyrebirdServiceContent {
		t.Error("written service content does not match lyrebirdServiceContent")
	}

	// Verify key hardening directives are present.
	for _, directive := range []string{
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"PrivateTmp=true",
		"ProtectHome=true",
		"StartLimitIntervalSec=0",
		"ExecReload=/bin/kill -HUP $MAINPID",
		// char-alsa (not the invalid /dev/snd/* glob) is required for audio
		// capture under DevicePolicy=closed.
		"DeviceAllow=char-alsa rw",
		// RuntimeDirectory/StateDirectory make the unit survive a reboot.
		"RuntimeDirectory=lyrebird",
		"StateDirectory=lyrebird",
	} {
		if !strings.Contains(string(data), directive) {
			t.Errorf("service file missing security directive: %s", directive)
		}
	}
}

// TestInstallLyreBirdServiceToPathSuccess covers the happy path with a fake systemctl.
func TestInstallLyreBirdServiceToPathSuccess(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Fake systemctl that exits 0.
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	servicePath := filepath.Join(tmpDir, "lyrebird-stream.service")
	if err := installLyreBirdServiceToPath(servicePath); err != nil {
		t.Fatalf("installLyreBirdServiceToPath() = %v; want nil", err)
	}

	data, err := os.ReadFile(servicePath) //#nosec G304
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != lyrebirdServiceContent {
		t.Error("installed service content does not match lyrebirdServiceContent")
	}
}

// TestInstallLyreBirdServiceToPathWriteError covers the write-failure error path.
func TestInstallLyreBirdServiceToPathWriteError(t *testing.T) {
	// Pass a path whose parent directory does not exist.
	err := installLyreBirdServiceToPath("/nonexistent/path/lyrebird-stream.service")
	if err == nil {
		t.Fatal("installLyreBirdServiceToPath() expected error for missing directory")
	}
	if !strings.Contains(err.Error(), "failed to write service file") {
		t.Errorf("installLyreBirdServiceToPath() error = %q; want 'failed to write service file'", err.Error())
	}
}

// TestInstallLyreBirdServiceToPathSystemctlFailure covers the systemctl daemon-reload error path.
func TestInstallLyreBirdServiceToPathSystemctlFailure(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Fake systemctl that fails.
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\necho 'daemon-reload failed' >&2\nexit 1\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	servicePath := filepath.Join(tmpDir, "lyrebird-stream.service")
	err := installLyreBirdServiceToPath(servicePath)
	if err == nil {
		t.Fatal("installLyreBirdServiceToPath() expected error when systemctl fails")
	}
	if !strings.Contains(err.Error(), "systemctl daemon-reload failed") {
		t.Errorf("installLyreBirdServiceToPath() error = %q; want 'systemctl daemon-reload failed'", err.Error())
	}
}

// diffStrings returns the first line that differs between a and b.
func diffStrings(a, b string) string {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	for i := 0; i < len(aLines) && i < len(bLines); i++ {
		if aLines[i] != bLines[i] {
			return fmt.Sprintf("first difference at line %d:\n  want: %q\n  got:  %q", i+1, aLines[i], bLines[i])
		}
	}
	if len(aLines) != len(bLines) {
		return fmt.Sprintf("line count differs: want %d, got %d", len(aLines), len(bLines))
	}
	return "(no line-level diff found; possibly whitespace)"
}
