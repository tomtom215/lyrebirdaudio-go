package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestDeviceConfigFieldSuffixesReflection verifies that buildDeviceConfigFieldSuffixes
// derives a complete, up-to-date set of field names from the DeviceConfig struct tags.
// This test will fail if a new field is added to DeviceConfig without a koanf/yaml tag,
// which is the early-warning signal the reflection approach is designed to provide.
func TestDeviceConfigFieldSuffixesReflection(t *testing.T) {
	suffixes := buildDeviceConfigFieldSuffixes()

	// Must not be empty
	if len(suffixes) == 0 {
		t.Fatal("buildDeviceConfigFieldSuffixes() returned empty slice")
	}

	// All suffixes must start with "_"
	for _, s := range suffixes {
		if !strings.HasPrefix(s, "_") {
			t.Errorf("suffix %q does not start with '_'", s)
		}
	}

	// All expected DeviceConfig fields must appear
	expected := []string{"_sample_rate", "_channels", "_bitrate", "_codec", "_thread_queue"}
	suffixSet := make(map[string]bool, len(suffixes))
	for _, s := range suffixes {
		suffixSet[s] = true
	}
	for _, want := range expected {
		if !suffixSet[want] {
			t.Errorf("expected suffix %q not in reflection-derived set %v", want, suffixes)
		}
	}
}

// TestDeviceConfigFieldSuffixesMatchesEnvOverride verifies that env override works
// for every field in deviceConfigFieldSuffixes so adding a field in DeviceConfig
// does not silently break env-var-based overrides.
func TestDeviceConfigFieldSuffixesMatchesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	base := "default:\n  sample_rate: 48000\n  channels: 2\n  bitrate: \"128k\"\n  codec: opus\n"
	if err := os.WriteFile(path, []byte(base), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Set env vars for every deviceConfigFieldSuffixes entry on a test device
	// LYREBIRD_DEVICES_TESTMIC_<FIELD_UPPER>
	fieldValues := map[string]string{
		"SAMPLE_RATE":  "44100",
		"CHANNELS":     "1",
		"BITRATE":      "64k",
		"CODEC":        "aac",
		"THREAD_QUEUE": "1024",
	}
	for field, val := range fieldValues {
		t.Setenv("LYREBIRD_DEVICES_TESTMIC_"+field, val)
	}

	kc, err := NewKoanfConfig(WithYAMLFile(path), WithEnvPrefix("LYREBIRD"))
	if err != nil {
		t.Fatalf("NewKoanfConfig: %v", err)
	}
	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	devCfg := cfg.GetDeviceConfig("testmic")
	if devCfg.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100 (env override did not apply)", devCfg.SampleRate)
	}
	if devCfg.Channels != 1 {
		t.Errorf("Channels = %d, want 1 (env override did not apply)", devCfg.Channels)
	}
	if devCfg.Codec != "aac" {
		t.Errorf("Codec = %q, want \"aac\" (env override did not apply)", devCfg.Codec)
	}
}
