package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

	// Create some test backup files
	testFiles := []string{
		"config.yaml.2025-12-14T10-00-00.bak",
		"config.yaml.2025-12-14T11-00-00.bak",
		"config.yaml.2025-12-14T12-00-00.bak",
		"other.yaml.2025-12-14T10-00-00.bak",
		"not-a-backup.txt",
	}

	for _, f := range testFiles {
		path := filepath.Join(backupDir, f)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// List all backups
	backups, err := ListBackups(backupDir, "")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}

	// Should find 4 backup files (excluding not-a-backup.txt)
	if len(backups) != 4 {
		t.Errorf("ListBackups() returned %d backups, want 4", len(backups))
	}

	// List only config.yaml backups
	backups, err = ListBackups(backupDir, "config.yaml")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("ListBackups() with filter returned %d backups, want 3", len(backups))
	}

	// Verify sorted by time (newest first)
	if len(backups) >= 2 {
		if backups[0].Timestamp.Before(backups[1].Timestamp) {
			t.Error("Backups not sorted newest first")
		}
	}
}

func TestListBackupsNonexistentDir(t *testing.T) {
	backups, err := ListBackups("/nonexistent/backups", "")
	if err != nil {
		t.Errorf("ListBackups() should return nil for nonexistent dir, got error: %v", err)
	}
	if backups != nil {
		t.Errorf("ListBackups() should return nil for nonexistent dir, got: %v", backups)
	}
}

// TestListBackupsWithDirectories tests ListBackups ignores directories in backup folder.
func TestListBackupsWithDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

	// Create a valid backup file
	backupFile := filepath.Join(backupDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(backupFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	// Create a subdirectory (should be ignored)
	subDir := filepath.Join(backupDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	backups, err := ListBackups(backupDir, "")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}

	// Should only find 1 backup (not the directory)
	if len(backups) != 1 {
		t.Errorf("ListBackups() returned %d backups, want 1", len(backups))
	}
}

// TestListBackupsInvalidTimestamp tests ListBackups skips files with invalid timestamps.
func TestListBackupsInvalidTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	// Create backup directory
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}

	// Create files with valid and invalid timestamps
	validFile := filepath.Join(backupDir, "config.yaml.2025-12-14T10-00-00.bak")
	if err := os.WriteFile(validFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create valid backup file: %v", err)
	}

	invalidFile := filepath.Join(backupDir, "config.yaml.invalid-timestamp.bak")
	if err := os.WriteFile(invalidFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create invalid backup file: %v", err)
	}

	backups, err := ListBackups(backupDir, "")
	if err != nil {
		t.Fatalf("ListBackups() error: %v", err)
	}

	// Should only find 1 backup (the valid one)
	if len(backups) != 1 {
		t.Errorf("ListBackups() returned %d backups, want 1", len(backups))
	}
}
