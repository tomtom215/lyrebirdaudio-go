package main

import (
	"path/filepath"
	"testing"
)

// TestDaemonFlagsStruct verifies the daemonFlags struct fields.
func TestDaemonFlagsStruct(t *testing.T) {
	flags := daemonFlags{
		ConfigPath: "/tmp/config.yaml",
		LockDir:    "/tmp/lyrebird",
		LogLevel:   "debug",
	}
	if flags.ConfigPath != "/tmp/config.yaml" {
		t.Errorf("ConfigPath = %q, want %q", flags.ConfigPath, "/tmp/config.yaml")
	}
	if flags.LockDir != "/tmp/lyrebird" {
		t.Errorf("LockDir = %q, want %q", flags.LockDir, "/tmp/lyrebird")
	}
	if flags.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", flags.LogLevel, "debug")
	}
}

// TestRunDaemonLockDirError verifies that runDaemon returns 1 when the lock
// directory cannot be created (e.g. path with null bytes).
func TestRunDaemonLockDirError(t *testing.T) {
	flags := daemonFlags{
		ConfigPath: "/tmp/config.yaml",
		LockDir:    "/\x00invalid",
		LogLevel:   "error",
	}
	code := runDaemon(flags)
	if code != 1 {
		t.Errorf("runDaemon() with invalid lock dir returned %d, want 1", code)
	}
}

// TestRunDaemonFFmpegNotFound verifies that runDaemon returns 1 when ffmpeg is not
// found (use a config path that does not exist, so defaults are used, and put a
// non-existent lock dir on a real temp path so MkdirAll succeeds before the
// ffmpeg check).
func TestRunDaemonFFmpegNotFound(t *testing.T) {
	if _, err := findFFmpegPath(); err == nil {
		t.Skip("ffmpeg is installed; cannot test missing-ffmpeg path")
	}
	tmpDir := t.TempDir()
	flags := daemonFlags{
		ConfigPath: filepath.Join(tmpDir, "nonexistent.yaml"),
		LockDir:    tmpDir,
		LogLevel:   "error",
	}
	code := runDaemon(flags)
	if code != 1 {
		t.Errorf("runDaemon() without ffmpeg returned %d, want 1", code)
	}
}

func TestDaemonSystemInfoProviderDiskSpace(t *testing.T) {
	dir := t.TempDir()
	p := &daemonSystemInfoProvider{
		recordDir:        dir,
		diskLowThreshold: 0, // disabled
	}
	si := p.SystemInfo()

	// On any real filesystem, disk total should be positive.
	if si.DiskTotalBytes == 0 {
		t.Error("DiskTotalBytes should be non-zero for real filesystem")
	}
	if si.DiskLowWarning {
		t.Error("DiskLowWarning should be false when threshold is 0")
	}
}

func TestDaemonSystemInfoProviderDiskLowThreshold(t *testing.T) {
	dir := t.TempDir()
	// Set an impossibly high threshold so it always triggers.
	p := &daemonSystemInfoProvider{
		recordDir:        dir,
		diskLowThreshold: 1<<62 - 1, // absurdly large
	}
	si := p.SystemInfo()
	if !si.DiskLowWarning {
		t.Error("DiskLowWarning should be true when threshold exceeds free space")
	}
}
