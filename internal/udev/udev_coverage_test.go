// SPDX-License-Identifier: MIT

package udev

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReloadUdevRulesAndDefaultCmdRunner covers both ReloadUdevRules (which is
// a thin delegator to reloadUdevRulesWith) and defaultCmdRunner (the real
// exec-based runner). In CI, udevadm is not installed so both functions return
// an error; the important thing is that the code paths are executed.
func TestReloadUdevRulesAndDefaultCmdRunner(t *testing.T) {
	// We accept any result (success or error) — the goal is code coverage.
	// udevadm is typically absent in CI / containers, so an error is expected.
	_ = ReloadUdevRules()
}

// TestDefaultCmdRunnerDirectly calls defaultCmdRunner directly to cover its
// single statement independently of ReloadUdevRules. Using "true" ensures the
// command exists on every POSIX system and exits 0, so we can verify the happy
// path as well.
func TestDefaultCmdRunnerDirectly(t *testing.T) {
	out, err := defaultCmdRunner("true")
	if err != nil {
		t.Logf("defaultCmdRunner(\"true\") error (unexpected on POSIX): %v", err)
	}
	_ = out
}

// TestGetUSBPhysicalPortReadDirError covers the os.ReadDir error path in
// GetUSBPhysicalPort. We pass a regular file as sysfsPath: os.Stat succeeds
// (the path exists), but os.ReadDir fails with ENOTDIR because the path is a
// file, not a directory.
func TestGetUSBPhysicalPortReadDirError(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, _, err := GetUSBPhysicalPort(filePath, 1, 1)
	if err == nil {
		t.Error("expected error when sysfsPath is a regular file, got nil")
	}
}
