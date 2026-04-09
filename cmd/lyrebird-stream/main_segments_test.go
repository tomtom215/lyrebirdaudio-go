package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

func TestCleanupSegmentsMaxAge(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	now := time.Now()
	oldFile := filepath.Join(dir, "old_segment.wav")
	newFile := filepath.Join(dir, "new_segment.wav")

	// Write a file and backdate it via Chtimes.
	writeFile := func(path string, age time.Duration) {
		if err := os.WriteFile(path, []byte("audio data"), 0600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		mtime := now.Add(-age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}

	writeFile(oldFile, 10*24*time.Hour) // 10 days old — should be deleted
	writeFile(newFile, 1*time.Hour)     // 1 hour old — should be kept

	cfg := config.StreamConfig{
		LocalRecordDir: dir,
		SegmentMaxAge:  7 * 24 * time.Hour, // 7-day limit
	}

	cleanupSegments(logger, cfg)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("old file %q should have been deleted", oldFile)
	}
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Errorf("new file %q should have been kept", newFile)
	}
}

func TestCleanupSegmentsMaxTotalBytes(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	now := time.Now()
	// Create 3 files: oldest 3 hours ago, middle 2 hours ago, newest 1 hour ago.
	// Each is 1024 bytes. Budget = 2048 bytes, so oldest should be deleted.
	files := []struct {
		name string
		age  time.Duration
	}{
		{"oldest.wav", 3 * time.Hour},
		{"middle.wav", 2 * time.Hour},
		{"newest.wav", 1 * time.Hour},
	}

	data := make([]byte, 1024)
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		mtime := now.Add(-f.age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	cfg := config.StreamConfig{
		LocalRecordDir:       dir,
		SegmentMaxTotalBytes: 2048, // 2 KB budget, 3 files × 1024 = 3 KB total
	}

	cleanupSegments(logger, cfg)

	// Oldest should be deleted, middle and newest kept.
	if _, err := os.Stat(filepath.Join(dir, "oldest.wav")); !os.IsNotExist(err) {
		t.Error("oldest.wav should have been deleted to meet size budget")
	}
	if _, err := os.Stat(filepath.Join(dir, "middle.wav")); os.IsNotExist(err) {
		t.Error("middle.wav should have been kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "newest.wav")); os.IsNotExist(err) {
		t.Error("newest.wav should have been kept")
	}
}

func TestCleanupSegmentsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Should not panic on empty directory.
	cfg := config.StreamConfig{
		LocalRecordDir: dir,
		SegmentMaxAge:  7 * 24 * time.Hour,
	}
	cleanupSegments(logger, cfg)
}

func TestCleanupSegmentsNoLimits(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a file that would be deleted if limits were set.
	oldFile := filepath.Join(dir, "old.wav")
	if err := os.WriteFile(oldFile, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	cfg := config.StreamConfig{
		LocalRecordDir:       dir,
		SegmentMaxAge:        0, // disabled
		SegmentMaxTotalBytes: 0, // disabled
	}

	cleanupSegments(logger, cfg)

	// File should still exist — no limits set.
	if _, err := os.Stat(oldFile); os.IsNotExist(err) {
		t.Error("file should not be deleted when no limits are set")
	}
}
