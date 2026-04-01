// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestKoanfTransformDevicesUnknownField covers koanf.go:208-209 —
// the `return topLevel + "." + rest, v` branch in the TransformFunc.
// When a LYREBIRD_DEVICES_* env var is set but the suffix after the device
// name does not match any known DeviceConfig field, the entire remainder is
// treated as the device name and returned as a leaf key.
func TestKoanfTransformDevicesUnknownField(t *testing.T) {
	// LYREBIRD_DEVICES_MYDEV_ZZXUNKNOWN → after stripping prefix:
	// k = "devices_mydev_zzxunknown"
	// topLevel = "devices", rest = "mydev_zzxunknown"
	// No suffix in deviceConfigFieldSuffixes matches "_zzxunknown"
	// → return "devices.mydev_zzxunknown", "42"
	t.Setenv("LYREBIRD_DEVICES_MYDEV_ZZXUNKNOWN", "42")

	// NewKoanfConfig calls reload() which calls the transform function.
	kc, err := NewKoanfConfig(WithEnvPrefix("LYREBIRD"))
	if err != nil {
		t.Fatalf("NewKoanfConfig: %v", err)
	}
	_ = kc
}

// TestKoanfTransformNoKnownPrefix covers koanf.go:217-218 —
// the `return strings.ReplaceAll(k, "_", "."), v` branch in the TransformFunc.
// When the env var key (after stripping the LYREBIRD_ prefix) does not start
// with any of the known top-level prefixes, underscores are replaced with dots.
func TestKoanfTransformNoKnownPrefix(t *testing.T) {
	// LYREBIRD_ZZQUX_SOMETHING → k = "zzqux_something" → no prefix match
	// → return "zzqux.something", "test"
	t.Setenv("LYREBIRD_ZZQUX_SOMETHING", "test")

	kc, err := NewKoanfConfig(WithEnvPrefix("LYREBIRD"))
	if err != nil {
		t.Fatalf("NewKoanfConfig: %v", err)
	}
	_ = kc
}

// TestBackupConfigMkdirAllFails covers backup.go:75-77 — the
// `failed to create backup directory` error path in BackupConfig.
// A regular file is placed at backupDir so os.MkdirAll returns ENOTDIR.
func TestBackupConfigMkdirAllFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid config file (Stat and IsDir checks pass).
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("# valid\n"), 0640); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	// Regular file at backupDir path → MkdirAll returns ENOTDIR.
	backupDir := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(backupDir, []byte("file"), 0600); err != nil {
		t.Fatalf("WriteFile notadir: %v", err)
	}

	_, err := BackupConfig(configPath, backupDir)
	if err == nil {
		t.Error("BackupConfig() expected error when backupDir is a file, got nil")
	}
}

// TestBackupConfigReadFileError covers backup.go:82-84 — the
// `failed to read config file` error path in BackupConfig.
// The config file is made unreadable (mode 0000) so os.ReadFile fails with
// EACCES after Stat succeeds and MkdirAll creates the backup directory.
func TestBackupConfigReadFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test unreadable file as root")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("# valid\n"), 0640); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	// Remove read permission → ReadFile will fail.
	if err := os.Chmod(configPath, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(configPath, 0640) })

	backupDir := filepath.Join(tmpDir, "backups")
	_, err := BackupConfig(configPath, backupDir)
	if err == nil {
		t.Error("BackupConfig() expected error for unreadable config, got nil")
	}
}

// TestBackupConfigWriteFileError covers backup.go:104-106 — the
// `failed to write backup` error path in BackupConfig.
// The backupDir is made non-writable after creation so os.WriteFile fails.
func TestBackupConfigWriteFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test non-writable directory as root")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("# valid\n"), 0640); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	// Create backupDir first, then make it non-writable.
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(backupDir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(backupDir, 0750) })

	_, err := BackupConfig(configPath, backupDir)
	if err == nil {
		t.Error("BackupConfig() expected error for non-writable backupDir, got nil")
	}
}

// TestRestoreBackupMkdirAllFails covers backup.go:228-230 — the
// `failed to create config directory` error path in RestoreBackup.
// The parent of configPath is a regular file so os.MkdirAll returns ENOTDIR.
func TestRestoreBackupMkdirAllFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid backup file.
	backupPath := filepath.Join(tmpDir, "config.yaml.2025-01-01T00-00-00.bak")
	if err := os.WriteFile(backupPath, []byte("# valid yaml\n"), 0600); err != nil {
		t.Fatalf("WriteFile backup: %v", err)
	}

	// Place a regular file at the path that should be the config's parent dir.
	// filepath.Dir("tmpDir/notadir/config.yaml") == "tmpDir/notadir".
	// os.MkdirAll("tmpDir/notadir") fails because it already exists as a file.
	fakeParent := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(fakeParent, []byte("block"), 0600); err != nil {
		t.Fatalf("WriteFile fakeParent: %v", err)
	}
	configPath := filepath.Join(fakeParent, "config.yaml")
	backupDir := tmpDir

	_, err := RestoreBackup(backupPath, configPath, backupDir)
	if err == nil {
		t.Error("RestoreBackup() expected error when config parent path is a file, got nil")
	}
}

// TestSaveWithRenameFails covers config.go:199-201 — the
// `failed to rename temp config file` error path in saveWith.
// A custom atomicCreateTemp makes the parent directory non-writable after
// creating the temp file, so os.Rename fails with EACCES.
func TestSaveWithRenameFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test rename failure as root")
	}

	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "cfgdir")
	if err := os.Mkdir(parentDir, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	configPath := filepath.Join(parentDir, "config.yaml")

	// Custom createTemp: creates the temp file normally, then makes parentDir
	// non-writable so the subsequent os.Rename(tmpPath, configPath) fails.
	customCreateTemp := func(dir, pattern string) (atomicFile, error) {
		f, err := os.CreateTemp(dir, pattern) //#nosec G304 -- test helper
		if err != nil {
			return nil, err
		}
		// Make dir non-writable now that temp file is created.
		_ = os.Chmod(parentDir, 0555)
		t.Cleanup(func() { _ = os.Chmod(parentDir, 0750) })
		return f, nil
	}

	cfg := DefaultConfig()
	err := cfg.saveWith(configPath, customCreateTemp)
	if err == nil {
		t.Error("saveWith() expected rename error for non-writable parent dir, got nil")
	}
}

// TestCleanOldBackupsRemoveFails covers backup.go:271-273 — the
// `continue` branch when os.Remove fails in CleanOldBackups.
// The backup directory is made non-writable after the backup files are
// created, so os.Remove fails with EACCES and the loop continues.
func TestCleanOldBackupsRemoveFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test non-writable directory as root")
	}

	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create 2 backup files so CleanOldBackups(keepCount=0) tries to delete both.
	for _, name := range []string{
		"config.yaml.2025-01-01T00-00-00.bak",
		"config.yaml.2025-01-02T00-00-00.bak",
	} {
		p := filepath.Join(backupDir, name)
		if err := os.WriteFile(p, []byte("# backup"), 0600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	// Make backupDir non-writable so os.Remove fails.
	if err := os.Chmod(backupDir, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(backupDir, 0750) })

	// keepCount=0 → all backups should be deleted, but Remove fails → continue.
	deleted, err := CleanOldBackups(backupDir, "config.yaml", 0)
	if err != nil {
		t.Fatalf("CleanOldBackups() unexpected error: %v", err)
	}
	// No files could be deleted due to EACCES.
	if deleted != 0 {
		t.Errorf("CleanOldBackups() deleted=%d, want 0 (all removes failed)", deleted)
	}
}

// TestRestoreBackupWriteFileError covers backup.go:228-230 —
// the `failed to restore config` error path in RestoreBackup.
// The config parent directory is made non-writable after MkdirAll succeeds,
// so os.WriteFile fails with EACCES.
func TestRestoreBackupWriteFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test non-writable directory as root")
	}

	tmpDir := t.TempDir()

	// Create a valid backup file.
	backupPath := filepath.Join(tmpDir, "config.yaml.2025-01-01T00-00-00.bak")
	if err := os.WriteFile(backupPath, []byte("# valid yaml\n"), 0600); err != nil {
		t.Fatalf("WriteFile backup: %v", err)
	}

	// Create the config parent directory, then make it non-writable.
	configParent := filepath.Join(tmpDir, "cfgdir")
	if err := os.Mkdir(configParent, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(configParent, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(configParent, 0750) })

	configPath := filepath.Join(configParent, "config.yaml")
	backupDir := tmpDir

	_, err := RestoreBackup(backupPath, configPath, backupDir)
	if err == nil {
		t.Error("RestoreBackup() expected error for non-writable config dir, got nil")
	}
}

// TestSaveWithCreateTempError covers config.go:195-197 —
// the `failed to create temp config file` error path in saveWith.
// A non-writable parent directory causes os.CreateTemp to fail.
func TestSaveWithCreateTempError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test CreateTemp failure as root")
	}

	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "cfgdir")
	if err := os.Mkdir(parentDir, 0555); err != nil { // non-writable from the start
		t.Fatalf("Mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parentDir, 0750) })

	configPath := filepath.Join(parentDir, "config.yaml")
	cfg := DefaultConfig()
	err := cfg.saveWith(configPath, defaultCreateTemp)
	if err == nil {
		t.Error("saveWith() expected CreateTemp error for non-writable parent dir, got nil")
	}
}

// TestSaveWithMarshalError covers config.go:191-193 —
// the `failed to marshal config` error path in saveWith. In practice, the
// default Config type marshals without error; we call saveWith with a
// valid config to exercise the surrounding code and confirm no panic.
// (The marshal-error branch itself is unreachable for well-formed configs.)
func TestSaveConfigRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfg := DefaultConfig()
	cfg.Stream.InitialRestartDelay = 5 * time.Second
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}
	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() after Save(): %v", err)
	}
	if loaded.Stream.InitialRestartDelay != cfg.Stream.InitialRestartDelay {
		t.Errorf("round-trip: InitialRestartDelay = %v, want %v",
			loaded.Stream.InitialRestartDelay, cfg.Stream.InitialRestartDelay)
	}
}
