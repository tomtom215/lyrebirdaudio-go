package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBackupDirectoryPermissions verifies SEC-4: backup directory is created with 0750.
func TestBackupDirectoryPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "newbackupdir")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config file to backup
	if err := os.WriteFile(configPath, []byte("default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: \"128k\"\n  codec: opus\n"), 0640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := BackupConfig(configPath, backupDir)
	if err != nil {
		t.Fatalf("BackupConfig() error = %v", err)
	}

	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("Stat backup dir error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0750 {
		t.Errorf("backup directory permissions = %04o, want 0750 (SEC-4 least privilege)", perm)
	}
}

// TestBackupFilePermissions verifies SEC-4: backup files are created with 0600.
func TestBackupFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config file to backup
	if err := os.WriteFile(configPath, []byte("default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: \"128k\"\n  codec: opus\n"), 0640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	backupPath, err := BackupConfig(configPath, backupDir)
	if err != nil {
		t.Fatalf("BackupConfig() error = %v", err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("Stat backup file error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("backup file permissions = %04o, want 0600", perm)
	}
}

// TestRestoreBackupPermissions verifies SEC-4: restored config gets 0640 permissions.
func TestRestoreBackupPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "restored", "config.yaml")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Create a valid backup file
	backupContent := "default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: \"128k\"\n  codec: opus\n"
	backupPath := filepath.Join(backupDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(backupPath, []byte(backupContent), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := RestoreBackup(backupPath, configPath, backupDir)
	if err != nil {
		t.Fatalf("RestoreBackup() error = %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat restored config error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0640 {
		t.Errorf("restored config permissions = %04o, want 0640 (SEC-4 consistency)", perm)
	}
}

// TestRestoreConfigDirectoryPermissions verifies SEC-4: config directory is 0750.
func TestRestoreConfigDirectoryPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	newConfigDir := filepath.Join(tmpDir, "newconfigdir")
	configPath := filepath.Join(newConfigDir, "config.yaml")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Create a valid backup file
	backupContent := "default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: \"128k\"\n  codec: opus\n"
	backupPath := filepath.Join(backupDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(backupPath, []byte(backupContent), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := RestoreBackup(backupPath, configPath, backupDir)
	if err != nil {
		t.Fatalf("RestoreBackup() error = %v", err)
	}

	info, err := os.Stat(newConfigDir)
	if err != nil {
		t.Fatalf("Stat config dir error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0750 {
		t.Errorf("config directory permissions = %04o, want 0750 (SEC-4 least privilege)", perm)
	}
}
