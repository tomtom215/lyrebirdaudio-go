// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// minimalConfigYAML has a valid default section and one device but omits the
// stream, monitor and mediamtx sections entirely.
const minimalConfigYAML = `default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
devices:
  mic0:
    sample_rate: 44100
`

// TestOmittedSectionsGetDefaults verifies that fields omitted from the config
// file keep their built-in defaults instead of collapsing to the Go zero value.
// Previously an omitted stream section left MaxRestartAttempts=0, which made
// every stream fail before FFmpeg launched.
func TestOmittedSectionsGetDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(minimalConfigYAML), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	check := func(t *testing.T, cfg *Config) {
		t.Helper()
		if cfg.Stream.MaxRestartAttempts != 50 {
			t.Errorf("Stream.MaxRestartAttempts = %d, want 50 (default)", cfg.Stream.MaxRestartAttempts)
		}
		if cfg.Stream.InitialRestartDelay <= 0 {
			t.Errorf("Stream.InitialRestartDelay = %v, want the default", cfg.Stream.InitialRestartDelay)
		}
		if cfg.Stream.SegmentDuration != 3600 {
			t.Errorf("Stream.SegmentDuration = %d, want 3600 (default)", cfg.Stream.SegmentDuration)
		}
		if cfg.Monitor.HealthAddr != "127.0.0.1:9998" {
			t.Errorf("Monitor.HealthAddr = %q, want default", cfg.Monitor.HealthAddr)
		}
		if cfg.MediaMTX.APIURL == "" {
			t.Error("MediaMTX.APIURL is empty, want default")
		}
		// The explicitly-set field must still win.
		if cfg.Devices["mic0"].SampleRate != 44100 {
			t.Errorf("Devices[mic0].SampleRate = %d, want 44100", cfg.Devices["mic0"].SampleRate)
		}
	}

	// YAML path (used by `lyrebird validate`).
	t.Run("LoadConfig", func(t *testing.T) {
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		check(t, cfg)
	})

	// koanf path (used by the daemon).
	t.Run("Koanf", func(t *testing.T) {
		kc, err := NewKoanfConfig(WithYAMLFile(path))
		if err != nil {
			t.Fatalf("NewKoanfConfig: %v", err)
		}
		cfg, err := kc.Load()
		if err != nil {
			t.Fatalf("koanf Load: %v", err)
		}
		check(t, cfg)
	})
}
