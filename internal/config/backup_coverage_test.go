// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestListBackupsFileAsDir covers the non-IsNotExist ReadDir error in
// ListBackups. When backupDir is a regular file (not a directory),
// os.ReadDir fails with ENOTDIR — which is not os.IsNotExist — so the
// error branch at the end of the not-found check is hit.
func TestListBackupsFileAsDir(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ListBackups(filePath, "")
	if err == nil {
		t.Error("expected error when backupDir is a regular file, got nil")
	}
}

// TestCleanOldBackupsFileAsDir covers the ListBackups error propagation in
// CleanOldBackups when the backup directory is actually a file.
func TestCleanOldBackupsFileAsDir(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := CleanOldBackups(filePath, "config.yaml", 3)
	if err == nil {
		t.Error("expected error when backupDir is a regular file, got nil")
	}
}

// TestRestoreBackupReadFileError covers the ReadFile error in RestoreBackup
// by passing a directory as backupPath. os.Stat succeeds on a directory, but
// os.ReadFile fails with EISDIR.
func TestRestoreBackupReadFileError(t *testing.T) {
	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "mybackup")
	if err := os.Mkdir(backupPath, 0750); err != nil {
		t.Fatalf("Mkdir backupPath: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.yaml")
	backupDir := filepath.Join(tmpDir, "backups")

	_, err := RestoreBackup(backupPath, configPath, backupDir)
	if err == nil {
		t.Error("expected error reading backup directory as file, got nil")
	}
}

// TestRestoreBackupCurrentConfigIsDir covers the BackupConfig error in
// RestoreBackup when the current configPath is a directory (exists, passes
// Stat check, but BackupConfig rejects it as a directory).
func TestRestoreBackupCurrentConfigIsDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid backup file with correct YAML content.
	backupContent := "# valid yaml\n"
	backupFile := filepath.Join(tmpDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(backupFile, []byte(backupContent), 0644); err != nil {
		t.Fatalf("WriteFile backup: %v", err)
	}

	// Make configPath a directory — os.Stat succeeds but BackupConfig fails.
	configPath := filepath.Join(tmpDir, "config-dir")
	if err := os.Mkdir(configPath, 0750); err != nil {
		t.Fatalf("Mkdir configPath: %v", err)
	}

	backupDir := filepath.Join(tmpDir, "backups")

	_, err := RestoreBackup(backupFile, configPath, backupDir)
	if err == nil {
		t.Error("expected error when configPath is a directory, got nil")
	}
}

// TestParseBackupTimestampTooFewParts covers the len(parts) < 2 branch in
// parseBackupTimestamp. After stripping the .bak suffix, a name with no dots
// has only one part, which triggers the "invalid backup filename format" error.
func TestParseBackupTimestampTooFewParts(t *testing.T) {
	// "nodots" + ".bak" → after TrimSuffix → "nodots" → parts=["nodots"] → len < 2
	_, err := parseBackupTimestamp("nodots.bak")
	if err == nil {
		t.Error("expected error for filename with too few parts, got nil")
	}
}

// TestBackupBeforeSaveConfigIsDir covers the BackupConfig error in
// BackupBeforeSave when the existing configPath is a directory.
func TestBackupBeforeSaveConfigIsDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory at the config path so Stat succeeds but BackupConfig fails.
	configPath := filepath.Join(tmpDir, "config-is-dir")
	if err := os.Mkdir(configPath, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	backupDir := filepath.Join(tmpDir, "backups")
	cfg := DefaultConfig()

	_, err := BackupBeforeSave(cfg, configPath, backupDir)
	if err == nil {
		t.Error("expected error when configPath is a directory, got nil")
	}
}

// TestBackupBeforeSaveSaveFails covers the cfg.Save error in BackupBeforeSave.
// The config does not exist yet (no backup step), and configPath is in a
// non-existent directory so Save fails (MkdirAll may fail with a file in the way).
func TestBackupBeforeSaveSaveFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file where the config's parent directory should be,
	// so cfg.Save cannot create the parent directory.
	parentPath := filepath.Join(tmpDir, "blocked")
	if err := os.WriteFile(parentPath, []byte("block"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// configPath is inside a "directory" that is actually a file.
	configPath := filepath.Join(parentPath, "config.yaml")
	backupDir := filepath.Join(tmpDir, "backups")
	cfg := DefaultConfig()

	// configPath doesn't exist (Stat fails) so no backup step is attempted.
	// cfg.Save will try to create the directory (parentPath) but it's a file.
	_, err := BackupBeforeSave(cfg, configPath, backupDir)
	if err == nil {
		t.Error("expected error when Save cannot create parent directory, got nil")
	}
}
