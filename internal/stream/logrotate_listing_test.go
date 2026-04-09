package stream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListRotatedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create some rotated files
	if err := os.WriteFile(logPath+".1", []byte("data1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".2", []byte("data22"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	files, err := ListRotatedFiles(logPath)
	if err != nil {
		t.Fatalf("ListRotatedFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 rotated files, got %d", len(files))
	}
}

func TestTotalLogSize(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create main log and rotated files
	if err := os.WriteFile(logPath, []byte("mainlog"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".1", []byte("rotated1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	total, err := TotalLogSize(logPath)
	if err != nil {
		t.Fatalf("TotalLogSize failed: %v", err)
	}

	expected := int64(len("mainlog") + len("rotated1"))
	if total != expected {
		t.Errorf("TotalLogSize = %d, want %d", total, expected)
	}
}

func TestCleanupLogs(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create log files
	if err := os.WriteFile(logPath, []byte("main"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".1", []byte("rot1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := CleanupLogs(logPath)
	if err != nil {
		t.Fatalf("CleanupLogs failed: %v", err)
	}

	// Check files are removed
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("Expected main log to be removed")
	}
	if _, err := os.Stat(logPath + ".1"); !os.IsNotExist(err) {
		t.Error("Expected rotated log to be removed")
	}
}

func TestListRotatedFilesNoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nonexistent.log")

	files, err := ListRotatedFiles(logPath)
	if err != nil {
		t.Fatalf("ListRotatedFiles failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(files))
	}
}

func TestTotalLogSizeNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nonexistent.log")

	total, err := TotalLogSize(logPath)
	if err != nil {
		t.Fatalf("TotalLogSize failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected 0, got %d", total)
	}
}

func TestListRotatedFilesWithCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create regular and compressed rotated files
	if err := os.WriteFile(logPath+".1", []byte("data1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".2.gz", []byte("compressed"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	files, err := ListRotatedFiles(logPath)
	if err != nil {
		t.Fatalf("ListRotatedFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Check that compressed file is identified correctly
	hasCompressed := false
	for _, f := range files {
		if f.Compressed {
			hasCompressed = true
		}
	}
	if !hasCompressed {
		t.Error("Expected at least one compressed file")
	}
}

func TestCleanupLogsWithCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create main, rotated, and compressed files
	if err := os.WriteFile(logPath, []byte("main"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".1", []byte("rot1"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(logPath+".2.gz", []byte("comp"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := CleanupLogs(logPath)
	if err != nil {
		t.Fatalf("CleanupLogs failed: %v", err)
	}

	// All files should be removed
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("Expected main log to be removed")
	}
	if _, err := os.Stat(logPath + ".1"); !os.IsNotExist(err) {
		t.Error("Expected rotated log to be removed")
	}
	if _, err := os.Stat(logPath + ".2.gz"); !os.IsNotExist(err) {
		t.Error("Expected compressed log to be removed")
	}
}
