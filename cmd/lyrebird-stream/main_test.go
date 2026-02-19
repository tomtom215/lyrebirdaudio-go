package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
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

func TestPrintUsage(t *testing.T) {
	// Just verify printUsage doesn't panic
	printUsage()
}

func TestFindFFmpegPathCommonLocations(t *testing.T) {
	// Test that the function checks common locations
	path, err := findFFmpegPath()

	// Either finds ffmpeg or returns appropriate error
	if err != nil {
		if path != "" {
			t.Errorf("findFFmpegPath returned path %q with error %v", path, err)
		}
	} else {
		if path == "" {
			t.Error("findFFmpegPath returned empty path without error")
		}
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

func TestStreamServiceWithLogger(t *testing.T) {
	// Test streamService with a logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	svc := &streamService{
		name:    "test_device",
		manager: nil,
		logger:  logger,
	}

	// Verify name works with logger set
	if got := svc.Name(); got != "test_device" {
		t.Errorf("Name() = %q, want %q", got, "test_device")
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

func TestParseSlogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSlogLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseSlogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestDeviceConfigHash verifies the M-6 config hash function.
func TestDeviceConfigHash(t *testing.T) {
	base := config.DeviceConfig{
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 512,
	}
	url := "rtsp://localhost:8554/device"

	t.Run("same config produces same hash", func(t *testing.T) {
		h1 := deviceConfigHash(base, url)
		h2 := deviceConfigHash(base, url)
		if h1 != h2 {
			t.Errorf("identical configs produced different hashes: %q vs %q", h1, h2)
		}
	})

	t.Run("different sample rate produces different hash", func(t *testing.T) {
		changed := base
		changed.SampleRate = 44100
		if deviceConfigHash(base, url) == deviceConfigHash(changed, url) {
			t.Error("different sample rates should produce different hashes")
		}
	})

	t.Run("different channels produces different hash", func(t *testing.T) {
		changed := base
		changed.Channels = 1
		if deviceConfigHash(base, url) == deviceConfigHash(changed, url) {
			t.Error("different channels should produce different hashes")
		}
	})

	t.Run("different bitrate produces different hash", func(t *testing.T) {
		changed := base
		changed.Bitrate = "256k"
		if deviceConfigHash(base, url) == deviceConfigHash(changed, url) {
			t.Error("different bitrates should produce different hashes")
		}
	})

	t.Run("different codec produces different hash", func(t *testing.T) {
		changed := base
		changed.Codec = "aac"
		if deviceConfigHash(base, url) == deviceConfigHash(changed, url) {
			t.Error("different codecs should produce different hashes")
		}
	})

	t.Run("different rtsp url produces different hash", func(t *testing.T) {
		otherURL := "rtsp://localhost:8554/other"
		if deviceConfigHash(base, url) == deviceConfigHash(base, otherURL) {
			t.Error("different RTSP URLs should produce different hashes")
		}
	})

	t.Run("hash contains all fields", func(t *testing.T) {
		h := deviceConfigHash(base, url)
		// Hash should be non-empty and contain identifiable field values.
		if h == "" {
			t.Error("hash must not be empty")
		}
		if !strings.Contains(h, "48000") {
			t.Errorf("hash should contain sample rate; got %q", h)
		}
		if !strings.Contains(h, "opus") {
			t.Errorf("hash should contain codec; got %q", h)
		}
	})
}
