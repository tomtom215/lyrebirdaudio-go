package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestKoanfConfig_LoadYAML tests loading configuration from a YAML file.
func TestKoanfConfig_LoadYAML(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write test config
	testConfig := `
devices:
  blue_yeti:
    sample_rate: 48000
    channels: 2
    bitrate: 192k
    codec: opus
    thread_queue: 8192

default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s
  max_restart_delay: 300s
  max_restart_attempts: 50
  usb_stabilization_delay: 5s

mediamtx:
  api_url: http://localhost:9997
  rtsp_url: rtsp://localhost:8554
  config_path: /etc/mediamtx/mediamtx.yml

monitor:
  enabled: true
  interval: 5m
  restart_unhealthy: true
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config using koanf
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded configuration
	if cfg.Default.SampleRate != 48000 {
		t.Errorf("Expected default sample rate 48000, got %d", cfg.Default.SampleRate)
	}

	if cfg.Default.Codec != "opus" {
		t.Errorf("Expected default codec opus, got %s", cfg.Default.Codec)
	}

	// Verify device-specific config
	devCfg, ok := cfg.Devices["blue_yeti"]
	if !ok {
		t.Fatal("Expected blue_yeti device config")
	}

	if devCfg.SampleRate != 48000 {
		t.Errorf("Expected blue_yeti sample rate 48000, got %d", devCfg.SampleRate)
	}

	if devCfg.Bitrate != "192k" {
		t.Errorf("Expected blue_yeti bitrate 192k, got %s", devCfg.Bitrate)
	}

	// Verify stream config
	if cfg.Stream.InitialRestartDelay != 10*time.Second {
		t.Errorf("Expected initial restart delay 10s, got %v", cfg.Stream.InitialRestartDelay)
	}

	if cfg.Stream.MaxRestartDelay != 300*time.Second {
		t.Errorf("Expected max restart delay 300s, got %v", cfg.Stream.MaxRestartDelay)
	}
}

// TestKoanfConfig_BackwardCompatibility tests backward compatibility with LoadConfig.
func TestKoanfConfig_BackwardCompatibility(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write test config
	testConfig := `
devices:
  blue_yeti:
    sample_rate: 48000
    channels: 2
    bitrate: 192k
    codec: opus
    thread_queue: 8192

default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s
  max_restart_delay: 300s
  max_restart_attempts: 50
  usb_stabilization_delay: 5s

mediamtx:
  api_url: http://localhost:9997
  rtsp_url: rtsp://localhost:8554
  config_path: /etc/mediamtx/mediamtx.yml

monitor:
  enabled: true
  interval: 5m
  restart_unhealthy: true
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load with old API
	oldCfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Load with new koanf API
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	newCfg, err := kc.Load()
	if err != nil {
		t.Fatalf("koanf Load failed: %v", err)
	}

	// Compare configurations
	if oldCfg.Default.SampleRate != newCfg.Default.SampleRate {
		t.Errorf("Sample rate mismatch: old=%d, new=%d", oldCfg.Default.SampleRate, newCfg.Default.SampleRate)
	}

	if oldCfg.Default.Codec != newCfg.Default.Codec {
		t.Errorf("Codec mismatch: old=%s, new=%s", oldCfg.Default.Codec, newCfg.Default.Codec)
	}

	if oldCfg.Default.Bitrate != newCfg.Default.Bitrate {
		t.Errorf("Bitrate mismatch: old=%s, new=%s", oldCfg.Default.Bitrate, newCfg.Default.Bitrate)
	}

	// Compare device configs
	oldDev := oldCfg.Devices["blue_yeti"]
	newDev := newCfg.Devices["blue_yeti"]

	if oldDev.SampleRate != newDev.SampleRate {
		t.Errorf("Device sample rate mismatch: old=%d, new=%d", oldDev.SampleRate, newDev.SampleRate)
	}

	if oldDev.Bitrate != newDev.Bitrate {
		t.Errorf("Device bitrate mismatch: old=%s, new=%s", oldDev.Bitrate, newDev.Bitrate)
	}
}

// TestKoanfConfig_InvalidYAML tests handling of invalid YAML.
func TestKoanfConfig_InvalidYAML(t *testing.T) {
	// Create temp directory and invalid config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write invalid YAML
	invalidConfig := `
default:
  sample_rate: "not a number"
  channels: invalid
`
	if err := os.WriteFile(configPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Attempt to load config
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		// This is expected - invalid config should fail during NewKoanfConfig
		return
	}

	// If NewKoanfConfig succeeded, Load should fail
	_, err = kc.Load()
	if err == nil {
		t.Error("Expected error loading invalid YAML, got nil")
	}
}

// TestKoanfConfig_MissingFile tests handling of missing config file.
func TestKoanfConfig_MissingFile(t *testing.T) {
	// Try to load non-existent file
	_, err := NewKoanfConfig(WithYAMLFile("/nonexistent/config.yaml"))
	if err == nil {
		t.Error("Expected error loading missing file, got nil")
	}
}

// TestKoanfConfig_NoFile tests loading without a file (env vars only).
func TestKoanfConfig_NoFile(t *testing.T) {
	// Set environment variables for complete config
	t.Setenv("LYREBIRD_DEFAULT_SAMPLE_RATE", "48000")
	t.Setenv("LYREBIRD_DEFAULT_CHANNELS", "2")
	t.Setenv("LYREBIRD_DEFAULT_BITRATE", "128k")
	t.Setenv("LYREBIRD_DEFAULT_CODEC", "opus")
	t.Setenv("LYREBIRD_DEFAULT_THREAD_QUEUE", "8192")

	// Load config with env vars only (no file)
	kc, err := NewKoanfConfig(WithEnvPrefix("LYREBIRD"))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify config loaded from env vars
	if cfg.Default.SampleRate != 48000 {
		t.Errorf("Expected sample rate 48000, got %d", cfg.Default.SampleRate)
	}

	if cfg.Default.Codec != "opus" {
		t.Errorf("Expected codec opus, got %s", cfg.Default.Codec)
	}
}
