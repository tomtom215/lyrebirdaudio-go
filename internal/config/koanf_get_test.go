package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

// TestKoanfConfig_All tests the All() method for complete map access.
func TestKoanfConfig_All(t *testing.T) {
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
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	// Test All() method
	allConfig := kc.All()

	if allConfig == nil {
		t.Fatal("All() returned nil")
	}

	// Verify the map contains expected keys (koanf returns flat dot-notation keys)
	if _, ok := allConfig["default.sample_rate"]; !ok {
		t.Error("All() should contain 'default.sample_rate' key")
	}

	if _, ok := allConfig["stream.initial_restart_delay"]; !ok {
		t.Error("All() should contain 'stream.initial_restart_delay' key")
	}

	if _, ok := allConfig["mediamtx.api_url"]; !ok {
		t.Error("All() should contain 'mediamtx.api_url' key")
	}

	if _, ok := allConfig["monitor.enabled"]; !ok {
		t.Error("All() should contain 'monitor.enabled' key")
	}
}
