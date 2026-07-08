// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDaemonSystemInfoProviderEmptyRecordDir verifies fallback to "/" when
// recordDir is empty.
func TestDaemonSystemInfoProviderEmptyRecordDir(t *testing.T) {
	p := &daemonSystemInfoProvider{
		recordDir:        "",
		diskLowThreshold: 0,
	}
	si := p.SystemInfo(context.Background())

	// Should still report disk stats (for "/")
	if si.DiskTotalBytes == 0 {
		t.Error("DiskTotalBytes should be non-zero for root filesystem")
	}
}

// TestRunDaemonLogDirCreationFailure verifies behavior when log dir cannot be
// created (falls back to no logging).
func TestRunDaemonLogDirCreationFailure(t *testing.T) {
	// Ensure ffmpeg is not found so runDaemon exits early after the log dir
	// warning instead of starting the supervisor and blocking forever.
	t.Setenv("PATH", t.TempDir())

	tmpDir := t.TempDir()
	flags := daemonFlags{
		ConfigPath: filepath.Join(tmpDir, "nonexistent.yaml"),
		LockDir:    tmpDir,
		LogLevel:   "error",
		LogDir:     "/\x00invalid/log/dir",
	}

	// The daemon warns about the bad log dir, then fails at ffmpeg lookup.
	code := runDaemon(flags)
	if code != 1 {
		t.Errorf("expected exit code 1 (ffmpeg not found), got %d", code)
	}
}

// TestRunDaemonWithConfigAndLogDir exercises runDaemon with valid lock dir and
// log dir but no ffmpeg (expected to fail at ffmpeg check).
func TestRunDaemonWithConfigAndLogDir(t *testing.T) {
	if _, err := findFFmpegPath(); err == nil {
		t.Skip("ffmpeg is installed; this test requires ffmpeg to be absent")
	}

	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")

	flags := daemonFlags{
		ConfigPath: filepath.Join(tmpDir, "config.yaml"),
		LockDir:    tmpDir,
		LogLevel:   "debug",
		LogDir:     logDir,
	}

	code := runDaemon(flags)
	if code != 1 {
		t.Errorf("runDaemon() returned %d, want 1 (ffmpeg not found)", code)
	}

	// Log dir should have been created
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Error("log directory should have been created")
	}
}

// TestRunDaemonWithInvalidConfig exercises runDaemon when config file exists
// but is invalid YAML.
func TestRunDaemonWithInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	flags := daemonFlags{
		ConfigPath: cfgPath,
		LockDir:    tmpDir,
		LogLevel:   "error",
	}

	code := runDaemon(flags)
	if code != 1 {
		t.Errorf("runDaemon() with invalid config returned %d, want 1", code)
	}
}
