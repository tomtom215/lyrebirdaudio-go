package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanOldBackups(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

	// Create test backup files with different timestamps
	for i := 0; i < 5; i++ {
		name := time.Now().Add(time.Duration(-i) * time.Hour).Format(BackupTimestampFormat)
		path := filepath.Join(backupDir, "config.yaml."+name+BackupSuffix)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Keep only 2 backups
	deleted, err := CleanOldBackups(backupDir, "config.yaml", 2)
	if err != nil {
		t.Fatalf("CleanOldBackups() error: %v", err)
	}

	if deleted != 3 {
		t.Errorf("CleanOldBackups() deleted %d files, want 3", deleted)
	}

	// Verify remaining backups
	remaining, _ := ListBackups(backupDir, "config.yaml")
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining backups, got %d", len(remaining))
	}
}

func TestCleanOldBackupsNegativeKeep(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := CleanOldBackups(tmpDir, "config.yaml", -1)
	if err == nil {
		t.Error("Expected error for negative keepCount")
	}
}

// TestCleanOldBackupsFewerThanKeep tests CleanOldBackups when there are fewer backups than keepCount.
func TestCleanOldBackupsFewerThanKeep(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

	// Create only 2 backups
	for i := 0; i < 2; i++ {
		name := time.Now().Add(time.Duration(-i) * time.Hour).Format(BackupTimestampFormat)
		path := filepath.Join(backupDir, "config.yaml."+name+BackupSuffix)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Keep 5 (more than we have)
	deleted, err := CleanOldBackups(backupDir, "config.yaml", 5)
	if err != nil {
		t.Fatalf("CleanOldBackups() error: %v", err)
	}

	if deleted != 0 {
		t.Errorf("CleanOldBackups() deleted %d files, want 0", deleted)
	}
}
