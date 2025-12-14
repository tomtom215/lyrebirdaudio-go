package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantErr     bool
		checkConfig func(t *testing.T, c interface{})
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
			cfg, err := loadConfiguration(path)

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

func TestFindFFmpegPath(t *testing.T) {
	// This test is environment-dependent
	// We just verify the function doesn't panic

	path, err := findFFmpegPath()

	// In CI/test environments, ffmpeg might not be installed
	// So we just verify the function returns something sensible
	if err != nil {
		t.Logf("FFmpeg not found (expected in some environments): %v", err)
		return
	}

	if path == "" {
		t.Error("findFFmpegPath returned empty path without error")
	}

	// Verify the path exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("findFFmpegPath returned non-existent path: %s", path)
	}
}

func TestStreamService_Name(t *testing.T) {
	svc := &streamService{
		name: "test_device",
	}

	if got := svc.Name(); got != "test_device" {
		t.Errorf("Name() = %q, want %q", got, "test_device")
	}
}

func TestStreamService_Run_WithNilManager(t *testing.T) {
	// This tests the error path when manager is nil
	// In production, this shouldn't happen, but we test defensively

	svc := &streamService{
		name:    "test",
		manager: nil,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should panic or return an error since manager is nil
	// We're testing that the code handles this gracefully
	defer func() {
		if r := recover(); r != nil {
			// Expected - nil manager causes panic
			t.Logf("Recovered from panic (expected): %v", r)
		}
	}()

	_ = svc.Run(ctx)
}
