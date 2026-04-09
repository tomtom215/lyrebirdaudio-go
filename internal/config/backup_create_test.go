package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a config file to backup
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `devices:
  test_device:
    sample_rate: 48000
default:
  sample_rate: 44100
  channels: 2
  bitrate: "128k"
  codec: opus
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	backupDir := filepath.Join(tmpDir, "backups")

	// Test backup
	backupPath, err := BackupConfig(configPath, backupDir)
	if err != nil {
		t.Fatalf("BackupConfig() error: %v", err)
	}

	// Verify backup was created
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Errorf("Backup file not created: %s", backupPath)
	}

	// Verify backup content matches original
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}

	if string(backupContent) != configContent {
		t.Errorf("Backup content mismatch")
	}
}

func TestBackupConfigFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	_, err := BackupConfig("/nonexistent/config.yaml", backupDir)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestBackupConfigDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Try to backup a directory
	_, err := BackupConfig(tmpDir, backupDir)
	if err == nil {
		t.Error("Expected error when trying to backup a directory")
	}
}

func TestBackupBeforeSave(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create existing config
	existingContent := `default:
  sample_rate: 44100
  channels: 1
  bitrate: "64k"
  codec: aac
`
	if err := os.WriteFile(configPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to create existing config: %v", err)
	}

	// Create new config
	cfg := DefaultConfig()
	cfg.Default.SampleRate = 48000

	// Backup and save
	backupPath, err := BackupBeforeSave(cfg, configPath, backupDir)
	if err != nil {
		t.Fatalf("BackupBeforeSave() error: %v", err)
	}

	if backupPath == "" {
		t.Error("Expected backup to be created")
	}

	// Verify new config was saved
	newCfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load new config: %v", err)
	}

	if newCfg.Default.SampleRate != 48000 {
		t.Errorf("New config not saved correctly, SampleRate = %d", newCfg.Default.SampleRate)
	}
}

// TestBackupBeforeSaveNoExistingConfig tests BackupBeforeSave when no config exists.
func TestBackupBeforeSaveNoExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create new config (no existing config at configPath)
	cfg := DefaultConfig()
	cfg.Default.SampleRate = 48000

	// Backup and save - should work without creating backup
	backupPath, err := BackupBeforeSave(cfg, configPath, backupDir)
	if err != nil {
		t.Fatalf("BackupBeforeSave() error: %v", err)
	}

	// No backup should be created since no existing config
	if backupPath != "" {
		t.Errorf("Expected no backup when config doesn't exist, got: %s", backupPath)
	}

	// Verify new config was saved
	newCfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load new config: %v", err)
	}

	if newCfg.Default.SampleRate != 48000 {
		t.Errorf("New config not saved correctly, SampleRate = %d", newCfg.Default.SampleRate)
	}
}

// TestBackupConfigDuplicateTimestamp tests creating backup when one already exists with same timestamp.
func TestBackupConfigDuplicateTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create a config file to backup
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create first backup
	backup1, err := BackupConfig(configPath, backupDir)
	if err != nil {
		t.Fatalf("First BackupConfig() error: %v", err)
	}

	// Create second backup immediately (same second, should add milliseconds)
	backup2, err := BackupConfig(configPath, backupDir)
	if err != nil {
		t.Fatalf("Second BackupConfig() error: %v", err)
	}

	// Paths should be different
	if backup1 == backup2 {
		t.Error("Duplicate backup should have unique path")
	}

	// Both backups should exist
	if _, err := os.Stat(backup1); os.IsNotExist(err) {
		t.Errorf("First backup not found: %s", backup1)
	}
	if _, err := os.Stat(backup2); os.IsNotExist(err) {
		t.Errorf("Second backup not found: %s", backup2)
	}
}
