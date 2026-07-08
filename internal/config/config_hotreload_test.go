// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestHotReloadRejectsInvalidAndKeepsLastGood verifies that a hot reload of a
// semantically-invalid config is rejected and the last-known-good config stays
// live, so a bad SIGHUP edit cannot take down a running daemon.
func TestHotReloadRejectsInvalidAndKeepsLastGood(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	good := "default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: 128k\n  codec: opus\n"
	if err := os.WriteFile(path, []byte(good), 0600); err != nil {
		t.Fatalf("write good config: %v", err)
	}

	kc, err := NewKoanfConfig(WithYAMLFile(path))
	if err != nil {
		t.Fatalf("NewKoanfConfig: %v", err)
	}
	cfg1, err := kc.Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}
	if cfg1.Default.Codec != "opus" {
		t.Fatalf("initial codec = %q, want opus", cfg1.Default.Codec)
	}

	// Overwrite with a semantically-invalid config (unsupported codec).
	bad := "default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: 128k\n  codec: mp3\n"
	if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}

	if err := kc.Reload(); err == nil {
		t.Fatal("Reload() accepted an invalid config; want an error keeping the current config")
	}

	// The live config must still be the last-known-good.
	cfg2, err := kc.Load()
	if err != nil {
		t.Fatalf("Load after a rejected reload should still succeed on the good config: %v", err)
	}
	if cfg2.Default.Codec != "opus" {
		t.Errorf("after a rejected reload, codec = %q, want opus (last-known-good preserved)", cfg2.Default.Codec)
	}

	// A subsequent VALID edit must be accepted.
	good2 := "default:\n  sample_rate: 44100\n  channels: 1\n  bitrate: 96k\n  codec: aac\n"
	if err := os.WriteFile(path, []byte(good2), 0600); err != nil {
		t.Fatalf("write good2 config: %v", err)
	}
	if err := kc.Reload(); err != nil {
		t.Fatalf("Reload() of a valid config should succeed: %v", err)
	}
	cfg3, err := kc.Load()
	if err != nil {
		t.Fatalf("Load after valid reload: %v", err)
	}
	if cfg3.Default.Codec != "aac" || cfg3.Default.SampleRate != 44100 {
		t.Errorf("after valid reload, got codec=%q rate=%d, want aac/44100", cfg3.Default.Codec, cfg3.Default.SampleRate)
	}
}
