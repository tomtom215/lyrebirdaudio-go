// SPDX-License-Identifier: MIT

//go:build linux

package updater

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func inodeOf(t *testing.T, path string) uint64 {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return st.Ino
}

// TestInstallBinaryUsesAtomicRename verifies that installing a new binary
// replaces the target via rename (a new inode) rather than truncating and
// rewriting it in place. In-place truncation fails with ETXTBSY when the target
// is the running executable, which is the whole point of self-update, so this
// guards the fix.
func TestInstallBinaryUsesAtomicRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "lyrebird")
	if err := os.WriteFile(target, []byte("OLD"), 0755); err != nil { //nolint:gosec // test binary must be executable
		t.Fatalf("write target: %v", err)
	}
	newBin := filepath.Join(dir, "release", "lyrebird")
	if err := os.MkdirAll(filepath.Dir(newBin), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(newBin, []byte("NEW-CONTENT"), 0644); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	inoBefore := inodeOf(t, target)

	if err := installBinary(newBin, target); err != nil {
		t.Fatalf("installBinary: %v", err)
	}

	got, err := os.ReadFile(target) //nolint:gosec // reading a test file
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "NEW-CONTENT" {
		t.Errorf("target content = %q, want NEW-CONTENT", got)
	}

	if inodeOf(t, target) == inoBefore {
		t.Error("target inode unchanged: binary was overwritten in place (ETXTBSY on the running executable) instead of atomically renamed")
	}

	// The installed binary must be executable.
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("installed binary is not executable: mode %v", info.Mode().Perm())
	}

	// No staging temp file must be left behind.
	if _, err := os.Stat(filepath.Join(dir, ".lyrebird.new")); !os.IsNotExist(err) {
		t.Error("staging temp file was left behind")
	}
}
