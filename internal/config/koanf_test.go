package config

import (
	"context"
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

// TestKoanfConfig_LoadWithEnvOverride tests environment variable overrides.
func TestKoanfConfig_LoadWithEnvOverride(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write test config
	testConfig := `
default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s
  max_restart_delay: 300s

mediamtx:
  api_url: http://localhost:9997
  rtsp_url: rtsp://localhost:8554

monitor:
  enabled: true
  interval: 5m
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set environment variables
	t.Setenv("LYREBIRD_DEFAULT_SAMPLE_RATE", "44100")
	t.Setenv("LYREBIRD_DEFAULT_CODEC", "aac")
	t.Setenv("LYREBIRD_DEFAULT_BITRATE", "256k")

	// Load config with env overrides
	kc, err := NewKoanfConfig(
		WithYAMLFile(configPath),
		WithEnvPrefix("LYREBIRD"),
	)
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify environment variables override YAML
	if cfg.Default.SampleRate != 44100 {
		t.Errorf("Expected sample rate 44100 (from env), got %d", cfg.Default.SampleRate)
	}

	if cfg.Default.Codec != "aac" {
		t.Errorf("Expected codec aac (from env), got %s", cfg.Default.Codec)
	}

	if cfg.Default.Bitrate != "256k" {
		t.Errorf("Expected bitrate 256k (from env), got %s", cfg.Default.Bitrate)
	}

	// Verify non-overridden values still come from YAML
	if cfg.Default.Channels != 2 {
		t.Errorf("Expected channels 2 (from YAML), got %d", cfg.Default.Channels)
	}
}

// TestKoanfConfig_LoadDeviceEnvOverride tests device-specific env overrides.
func TestKoanfConfig_LoadDeviceEnvOverride(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write test config with device
	testConfig := `
devices:
  blue_yeti:
    sample_rate: 48000
    channels: 2
    bitrate: 192k
    codec: opus

default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set device-specific environment variables
	t.Setenv("LYREBIRD_DEVICES_BLUE_YETI_SAMPLE_RATE", "96000")
	t.Setenv("LYREBIRD_DEVICES_BLUE_YETI_CODEC", "aac")

	// Load config
	kc, err := NewKoanfConfig(
		WithYAMLFile(configPath),
		WithEnvPrefix("LYREBIRD"),
	)
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify device-specific overrides
	devCfg, ok := cfg.Devices["blue_yeti"]
	if !ok {
		t.Fatal("Expected blue_yeti device config")
	}

	if devCfg.SampleRate != 96000 {
		t.Errorf("Expected blue_yeti sample rate 96000 (from env), got %d", devCfg.SampleRate)
	}

	if devCfg.Codec != "aac" {
		t.Errorf("Expected blue_yeti codec aac (from env), got %s", devCfg.Codec)
	}

	// Verify non-overridden values still come from YAML
	if devCfg.Bitrate != "192k" {
		t.Errorf("Expected blue_yeti bitrate 192k (from YAML), got %s", devCfg.Bitrate)
	}
}

// TestKoanfConfig_Reload tests manual configuration reload.
func TestKoanfConfig_Reload(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write initial config
	initialConfig := `
default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Default.SampleRate != 48000 {
		t.Fatalf("Expected initial sample rate 48000, got %d", cfg.Default.SampleRate)
	}

	// Modify config file
	updatedConfig := `
default:
  sample_rate: 44100
  channels: 2
  bitrate: 192k
  codec: aac
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config: %v", err)
	}

	// Reload configuration
	if err := kc.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Load again and verify changes
	cfg, err = kc.Load()
	if err != nil {
		t.Fatalf("Load after reload failed: %v", err)
	}

	if cfg.Default.SampleRate != 44100 {
		t.Errorf("Expected reloaded sample rate 44100, got %d", cfg.Default.SampleRate)
	}

	if cfg.Default.Bitrate != "192k" {
		t.Errorf("Expected reloaded bitrate 192k, got %s", cfg.Default.Bitrate)
	}

	if cfg.Default.Codec != "aac" {
		t.Errorf("Expected reloaded codec aac, got %s", cfg.Default.Codec)
	}
}

// TestKoanfConfig_Watch tests configuration file watching.
func TestKoanfConfig_Watch(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write initial config
	initialConfig := `
default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	// Channel to signal when watch callback is called
	watchCalled := make(chan string, 1)

	// Start watching in background
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = kc.Watch(ctx, func(event string) {
			watchCalled <- event
		})
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Modify config file
	updatedConfig := `
default:
  sample_rate: 44100
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config: %v", err)
	}

	// Wait for watch callback
	select {
	case event := <-watchCalled:
		if event != "config reloaded" {
			t.Errorf("Expected event 'config reloaded', got %s", event)
		}
	case <-time.After(2 * time.Second):
		t.Error("Watch callback not called within timeout")
	}

	// Verify config was reloaded
	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load after watch failed: %v", err)
	}

	if cfg.Default.SampleRate != 44100 {
		t.Errorf("Expected watched sample rate 44100, got %d", cfg.Default.SampleRate)
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

// TestKoanfConfig_GetMethods tests typed getter methods.
func TestKoanfConfig_GetMethods(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write test config
	testConfig := `
default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
  interval: 5m
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	// Test GetInt
	sampleRate := kc.GetInt("default.sample_rate")
	if sampleRate != 48000 {
		t.Errorf("Expected sample rate 48000, got %d", sampleRate)
	}

	// Test GetString
	codec := kc.GetString("default.codec")
	if codec != "opus" {
		t.Errorf("Expected codec opus, got %s", codec)
	}

	// Test GetBool
	enabled := kc.GetBool("monitor.enabled")
	if !enabled {
		t.Error("Expected monitor enabled to be true")
	}

	// Test GetDuration
	delay := kc.GetDuration("stream.initial_restart_delay")
	if delay != 10*time.Second {
		t.Errorf("Expected delay 10s, got %v", delay)
	}

	// Test Exists
	if !kc.Exists("default.codec") {
		t.Error("Expected default.codec to exist")
	}

	if kc.Exists("nonexistent.key") {
		t.Error("Expected nonexistent.key to not exist")
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
