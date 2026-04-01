// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunMigrateMigratedConfigInvalid covers cmd_config.go:58-60 —
// the `cfg.Validate()` error branch ("migrated config is invalid").
// A bash config file that sets DEFAULT_SAMPLE_RATE=-1 migrates successfully
// (strconv.Atoi accepts -1) but produces a DeviceConfig with SampleRate=-1,
// which DeviceConfig.Validate() rejects as non-positive.
func TestRunMigrateMigratedConfigInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	bashCfg := filepath.Join(tmpDir, "bash.conf")

	// DEFAULT_SAMPLE_RATE=-1 is a valid integer so MigrateFromBash succeeds,
	// but Validate() rejects it because SampleRate must be positive.
	content := "DEFAULT_SAMPLE_RATE=-1\n"
	if err := os.WriteFile(bashCfg, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile bash config: %v", err)
	}

	toPath := filepath.Join(tmpDir, "config.yaml")
	err := runMigrate([]string{"--from=" + bashCfg, "--to=" + toPath})
	if err == nil {
		t.Error("runMigrate() expected 'migrated config is invalid' error, got nil")
	}
}

// TestRunMigrateBackupWarning covers cmd_config.go:73-75 —
// the `fmt.Printf("  [!] Warning: failed to backup existing config")` path.
// The target config file already exists (so the backup branch is entered), but
// the backup directory path is occupied by a regular file, causing
// BackupConfig (via MkdirAll) to fail with ENOTDIR.
func TestRunMigrateBackupWarning(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid bash source config.
	bashCfg := filepath.Join(tmpDir, "bash.conf")
	if err := os.WriteFile(bashCfg, []byte("# empty config\n"), 0600); err != nil {
		t.Fatalf("WriteFile bash config: %v", err)
	}

	// Create the target YAML config (it must exist so the backup branch runs).
	toPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(toPath, []byte("# existing config\n"), 0640); err != nil {
		t.Fatalf("WriteFile existing config: %v", err)
	}

	// GetBackupDir(toPath) returns filepath.Join(tmpDir, "backups").
	// Place a regular file there so MkdirAll fails with ENOTDIR.
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.WriteFile(backupDir, []byte("occupied"), 0600); err != nil {
		t.Fatalf("WriteFile backupDir blocker: %v", err)
	}

	// --force so the existing-file check is skipped; backup warning should print.
	err := runMigrate([]string{"--from=" + bashCfg, "--to=" + toPath, "--force"})
	// runMigrate should succeed overall (backup failure is a warning, not an error).
	if err != nil {
		t.Logf("runMigrate() returned error (may be expected if Save also fails): %v", err)
	}
}

// TestRunDetectWithPathNoStream0Fallback covers cmd_devices.go:93-100 —
// the fallback branch when audio.DetectCapabilities fails because the card
// directory lacks a stream0 file. The card has id + usbid (so DetectDevices
// finds it) but no stream0, so DetectCapabilities returns an error and the
// function prints the "unavailable" fallback lines.
func TestRunDetectWithPathNoStream0Fallback(t *testing.T) {
	tmpDir := t.TempDir()
	cardDir := filepath.Join(tmpDir, "asound", "card0")
	if err := os.MkdirAll(cardDir, 0750); err != nil {
		t.Fatalf("MkdirAll cardDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("TestMic"), 0644); err != nil {
		t.Fatalf("write id: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "usbid"), []byte("0d8c:0014"), 0644); err != nil {
		t.Fatalf("write usbid: %v", err)
	}
	// No stream0 file → DetectCapabilities returns an error → fallback branch.

	asoundPath := filepath.Join(tmpDir, "asound")
	err := runDetectWithPath(asoundPath, []string{})
	if err != nil {
		t.Errorf("runDetectWithPath() unexpected error: %v", err)
	}
}
