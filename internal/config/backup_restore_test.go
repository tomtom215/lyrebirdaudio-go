package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreBackup(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

	// Create a valid backup file
	backupContent := `default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
`
	backupPath := filepath.Join(backupDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(backupPath, []byte(backupContent), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Restore when no current config exists
	prevBackup, err := RestoreBackup(backupPath, configPath, backupDir)
	if err != nil {
		t.Fatalf("RestoreBackup() error: %v", err)
	}

	if prevBackup != "" {
		t.Errorf("Expected no previous backup, got: %s", prevBackup)
	}

	// Verify config was restored
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read restored config: %v", err)
	}

	if string(restored) != backupContent {
		t.Error("Restored content doesn't match backup")
	}
}

func TestRestoreBackupWithExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

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

	// Create backup to restore
	backupContent := `default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
`
	backupPath := filepath.Join(backupDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(backupPath, []byte(backupContent), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Restore should backup existing config first
	prevBackup, err := RestoreBackup(backupPath, configPath, backupDir)
	if err != nil {
		t.Fatalf("RestoreBackup() error: %v", err)
	}

	if prevBackup == "" {
		t.Error("Expected previous backup to be created")
	}

	// Verify previous config was backed up
	if _, err := os.Stat(prevBackup); os.IsNotExist(err) {
		t.Errorf("Previous backup not created: %s", prevBackup)
	}
}

func TestRestoreBackupInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

	// Create invalid YAML backup
	backupPath := filepath.Join(backupDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(backupPath, []byte("invalid: yaml: content: ["), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	_, err := RestoreBackup(backupPath, configPath, backupDir)
	if err == nil {
		t.Error("Expected error for invalid YAML backup")
	}
}

// TestRestoreBackupNotFound tests RestoreBackup when backup file doesn't exist.
func TestRestoreBackupNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	configPath := filepath.Join(tmpDir, "config.yaml")

	_, err := RestoreBackup("/nonexistent/backup.bak", configPath, backupDir)
	if err == nil {
		t.Error("Expected error for nonexistent backup file")
	}
}
