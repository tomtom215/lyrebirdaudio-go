// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

// TestCleanupSegmentsSkipsDirectories verifies directories are not deleted.
func TestCleanupSegmentsSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a subdirectory (should be skipped)
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an old file
	oldFile := filepath.Join(dir, "old.wav")
	if err := os.WriteFile(oldFile, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	cfg := config.StreamConfig{
		LocalRecordDir: dir,
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	cleanupSegments(logger, cfg)

	// Old file should be deleted
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should be deleted")
	}

	// Subdirectory should still exist
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Error("subdirectory should not be deleted")
	}
}

// TestCleanupSegmentsEmptyLocalRecordDir verifies no-op when dir is empty string.
func TestCleanupSegmentsEmptyLocalRecordDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.StreamConfig{
		LocalRecordDir: "",
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	// Should return immediately without error
	cleanupSegments(logger, cfg)
}

// TestCleanupSegmentsNonExistentDir verifies graceful handling of missing dir.
func TestCleanupSegmentsNonExistentDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.StreamConfig{
		LocalRecordDir: "/nonexistent/dir/for/testing",
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	// Should log warning but not panic
	cleanupSegments(logger, cfg)
}

// TestCleanupSegmentsBothLimits verifies combined max age and size limits.
func TestCleanupSegmentsBothLimits(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	now := time.Now()

	// Create 3 files:
	// old1: 20 days old, 1KB
	// old2: 5 days old, 1KB
	// new1: 1 hour old, 1KB
	files := []struct {
		name string
		age  time.Duration
	}{
		{"old1.wav", 20 * 24 * time.Hour},
		{"old2.wav", 5 * 24 * time.Hour},
		{"new1.wav", 1 * time.Hour},
	}

	data := make([]byte, 1024)
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		mtime := now.Add(-f.age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.StreamConfig{
		LocalRecordDir:       dir,
		SegmentMaxAge:        10 * 24 * time.Hour, // Delete files > 10 days
		SegmentMaxTotalBytes: 1500,                // Budget: 1500 bytes (only 1 file fits)
	}

	cleanupSegments(logger, cfg)

	// old1 should be deleted by age (20 > 10 days)
	if _, err := os.Stat(filepath.Join(dir, "old1.wav")); !os.IsNotExist(err) {
		t.Error("old1.wav should be deleted by max age")
	}

	// old2 should be deleted by size budget (2KB > 1.5KB budget)
	if _, err := os.Stat(filepath.Join(dir, "old2.wav")); !os.IsNotExist(err) {
		t.Error("old2.wav should be deleted by size budget")
	}

	// new1 should be kept
	if _, err := os.Stat(filepath.Join(dir, "new1.wav")); os.IsNotExist(err) {
		t.Error("new1.wav should be kept")
	}
}
