// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckLockDirExistsIsDir verifies checkLockDir with an existing directory.
func TestCheckLockDirExistsIsDir(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	opts := DefaultOptions()
	opts.LockDir = lockDir
	r := NewRunner(opts)

	result := r.checkLockDir(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK, got %s: %s", result.Status, result.Message)
	}
}

// TestCheckLockDirNotExist verifies checkLockDir when directory is absent.
func TestCheckLockDirNotExist(t *testing.T) {
	opts := DefaultOptions()
	opts.LockDir = "/tmp/lyrebird-nonexistent-" + fmt.Sprintf("%d", os.Getpid())
	r := NewRunner(opts)

	result := r.checkLockDir(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing lockDir, got %s: %s", result.Status, result.Message)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestCheckLockDirNotADirectory verifies checkLockDir when path is a file.
func TestCheckLockDirNotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	notDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(notDir, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	opts := DefaultOptions()
	opts.LockDir = notDir
	r := NewRunner(opts)

	result := r.checkLockDir(context.Background())
	if result.Status != StatusCritical {
		t.Errorf("expected CRITICAL when lockDir is a file, got %s: %s", result.Status, result.Message)
	}
}

// TestCheckLockDirWithLockFiles verifies that existing .lock files are counted.
func TestCheckLockDirWithLockFiles(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a fake lock file
	if err := os.WriteFile(filepath.Join(lockDir, "mic.lock"), []byte("1234"), 0640); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	opts := DefaultOptions()
	opts.LockDir = lockDir
	r := NewRunner(opts)

	result := r.checkLockDir(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK with lock files, got %s", result.Status)
	}
	if result.Details == "" {
		t.Error("expected Details to mention active locks")
	}
}

// TestCheckLockFilePermissionsGoodDir verifies checkLockFilePermissions with mode 0750.
func TestCheckLockFilePermissionsGoodDir(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	opts := DefaultOptions()
	opts.LockDir = lockDir
	r := NewRunner(opts)

	result := r.checkLockFilePermissions(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK for mode 0750 lockDir, got %s: %s", result.Status, result.Message)
	}
}

// TestCheckLockFilePermissionsWorldAccessible verifies that world-accessible mode triggers warning.
func TestCheckLockFilePermissionsWorldAccessible(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.MkdirAll(lockDir, 0755); err != nil { // world-read+exec set
		t.Fatalf("mkdir: %v", err)
	}

	opts := DefaultOptions()
	opts.LockDir = lockDir
	r := NewRunner(opts)

	result := r.checkLockFilePermissions(context.Background())
	if result.Status != StatusWarning {
		t.Errorf("expected WARNING for world-accessible lockDir, got %s: %s", result.Status, result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions to fix permissions")
	}
}

// TestCheckLockFilePermissionsNotADirectory tests lockDir that is a file.
func TestCheckLockFilePermissionsNotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	notDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(notDir, []byte("x"), 0640); err != nil {
		t.Fatalf("write file: %v", err)
	}

	opts := DefaultOptions()
	opts.LockDir = notDir
	r := NewRunner(opts)

	result := r.checkLockFilePermissions(context.Background())
	if result.Status != StatusWarning {
		t.Errorf("expected WARNING when lockDir is a file, got %s: %s", result.Status, result.Message)
	}
}

// TestCheckLockFilePermissionsWithStaleLocks verifies stale lock detection.
func TestCheckLockFilePermissionsWithStaleLocks(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "lyrebird")
	if err := os.MkdirAll(lockDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Stale: PID that cannot exist
	staleLock := filepath.Join(lockDir, "stale.lock")
	if err := os.WriteFile(staleLock, []byte("999999999"), 0640); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	// Invalid: non-numeric content
	badLock := filepath.Join(lockDir, "bad.lock")
	if err := os.WriteFile(badLock, []byte("not-a-pid"), 0640); err != nil {
		t.Fatalf("write bad lock: %v", err)
	}

	opts := DefaultOptions()
	opts.LockDir = lockDir
	r := NewRunner(opts)

	result := r.checkLockFilePermissions(context.Background())
	if result.Status != StatusWarning {
		t.Errorf("expected WARNING for stale locks, got %s: %s", result.Status, result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions to remove stale locks")
	}
}

// TestCheckLockFilePermissionsNotExist verifies that a missing lockDir is OK.
func TestCheckLockFilePermissionsNotExist(t *testing.T) {
	opts := DefaultOptions()
	opts.LockDir = "/tmp/lyrebird-definitely-does-not-exist-9999"
	r := NewRunner(opts)

	result := r.checkLockFilePermissions(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing lockDir, got %s: %s", result.Status, result.Message)
	}
}

// TestCheckLogFilesMissingDir verifies checkLogFiles with non-existent log dir.
func TestCheckLogFilesMissingDir(t *testing.T) {
	opts := DefaultOptions()
	opts.LogDir = "/tmp/lyrebird-nonexistent-logs-9999"
	r := NewRunner(opts)

	result := r.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing log dir, got %s: %s", result.Status, result.Message)
	}
}

// TestCheckConfigMissing verifies checkConfig with non-existent config path.
func TestCheckConfigMissing(t *testing.T) {
	opts := DefaultOptions()
	opts.ConfigPath = "/tmp/lyrebird-no-config-9999.yaml"
	r := NewRunner(opts)

	result := r.checkConfig(context.Background())
	if result.Status != StatusWarning {
		t.Errorf("expected WARNING for missing config, got %s: %s", result.Status, result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions for missing config")
	}
}

// TestCheckConfigPresent verifies checkConfig with an existing config file.
func TestCheckConfigPresent(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("stream:\n  devices: []\n"), 0640); err != nil {
		t.Fatalf("write config: %v", err)
	}

	opts := DefaultOptions()
	opts.ConfigPath = cfgPath
	r := NewRunner(opts)

	result := r.checkConfig(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected OK for present config, got %s: %s", result.Status, result.Message)
	}
}
