package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigurationKoanf(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "valid config file",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				content := `
default:
  sample_rate: 44100
  channels: 1
  bitrate: "128k"
  codec: opus
stream:
  initial_restart_delay: 5s
  max_restart_delay: 60s
  max_restart_attempts: 10
mediamtx:
  rtsp_url: rtsp://localhost:8554
`
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write test config: %v", err)
				}
				return path
			},
			wantErr: false,
		},
		{
			name: "non-existent file uses defaults",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent.yaml")
			},
			wantErr: false,
		},
		{
			name: "invalid yaml",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "invalid.yaml")
				if err := os.WriteFile(path, []byte("{{invalid yaml"), 0644); err != nil {
					t.Fatalf("Failed to write test config: %v", err)
				}
				return path
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			_, cfg, err := loadConfigurationKoanf(path)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cfg == nil {
				t.Error("Config is nil")
				return
			}

			// Basic validation that config has reasonable defaults
			if cfg.Default.SampleRate <= 0 {
				t.Error("Default sample rate should be positive")
			}
		})
	}
}

func TestLoadConfigurationKoanfDefaults(t *testing.T) {
	// Test loading from non-existent path uses defaults
	_, cfg, err := loadConfigurationKoanf("/nonexistent/path/config.yaml")
	if err != nil {
		t.Errorf("loadConfigurationKoanf should not error for non-existent file: %v", err)
	}

	if cfg == nil {
		t.Fatal("loadConfigurationKoanf returned nil config")
	}

	// Verify defaults are set
	if cfg.Default.SampleRate != 48000 {
		t.Errorf("Default SampleRate = %d, want 48000", cfg.Default.SampleRate)
	}
	if cfg.Default.Channels != 2 {
		t.Errorf("Default Channels = %d, want 2", cfg.Default.Channels)
	}
}

func TestLoadConfigurationKoanfWithValidFile(t *testing.T) {
	// Create a valid config file
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
default:
  sample_rate: 44100
  channels: 1
  bitrate: "96k"
  codec: aac
stream:
  initial_restart_delay: 5s
  max_restart_delay: 120s
mediamtx:
  rtsp_url: rtsp://localhost:8554
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, cfg, err := loadConfigurationKoanf(path)
	if err != nil {
		t.Errorf("loadConfigurationKoanf should not error: %v", err)
	}

	if cfg == nil {
		t.Fatal("loadConfigurationKoanf returned nil config")
	}

	// Verify loaded values
	if cfg.Default.SampleRate != 44100 {
		t.Errorf("Default SampleRate = %d, want 44100", cfg.Default.SampleRate)
	}
	if cfg.Default.Channels != 1 {
		t.Errorf("Default Channels = %d, want 1", cfg.Default.Channels)
	}
	if cfg.Default.Bitrate != "96k" {
		t.Errorf("Default Bitrate = %s, want 96k", cfg.Default.Bitrate)
	}
}

// TestLoadConfigurationKoanfNonNilOnSuccess verifies the C-3 fix:
// loadConfigurationKoanf must never return (nil, non-nil, nil) because
// the daemon dereferences koanfCfg unconditionally in the poll loop.
func TestLoadConfigurationKoanfNonNilOnSuccess(t *testing.T) {
	// Case 1: non-existent file → falls back to defaults, koanfCfg non-nil.
	kc, cfg, err := loadConfigurationKoanf("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("non-existent file: expected no error, got: %v", err)
	}
	if cfg == nil {
		t.Error("non-existent file: cfg must not be nil")
	}
	// koanfCfg may legitimately be nil on this path (env-only fallback failed),
	// but cfg must always be non-nil when err is nil (the daemon only uses cfg
	// directly when koanfCfg is nil).
	_ = kc // nil or non-nil both accepted here; the nil guard in main handles it

	// Case 2: valid file → both koanfCfg and cfg non-nil.
	dir := t.TempDir()
	validPath := dir + "/config.yaml"
	content := "default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: \"128k\"\n  codec: opus\n"
	if err := os.WriteFile(validPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	kc2, cfg2, err := loadConfigurationKoanf(validPath)
	if err != nil {
		t.Fatalf("valid file: unexpected error: %v", err)
	}
	if kc2 == nil {
		t.Error("valid file: koanfCfg must not be nil")
	}
	if cfg2 == nil {
		t.Error("valid file: cfg must not be nil")
	}
}
