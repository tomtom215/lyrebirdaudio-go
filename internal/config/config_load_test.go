package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadConfig verifies basic YAML parsing and validation.
func TestLoadConfig(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify default settings
	if cfg.Default.SampleRate != 48000 {
		t.Errorf("Default.SampleRate = %d, want 48000", cfg.Default.SampleRate)
	}
	if cfg.Default.Channels != 2 {
		t.Errorf("Default.Channels = %d, want 2", cfg.Default.Channels)
	}
	if cfg.Default.Bitrate != "128k" {
		t.Errorf("Default.Bitrate = %q, want \"128k\"", cfg.Default.Bitrate)
	}
	if cfg.Default.Codec != "opus" {
		t.Errorf("Default.Codec = %q, want \"opus\"", cfg.Default.Codec)
	}
	if cfg.Default.ThreadQueue != 8192 {
		t.Errorf("Default.ThreadQueue = %d, want 8192", cfg.Default.ThreadQueue)
	}

	// Verify stream settings
	if cfg.Stream.InitialRestartDelay != 10*time.Second {
		t.Errorf("Stream.InitialRestartDelay = %v, want 10s", cfg.Stream.InitialRestartDelay)
	}
	if cfg.Stream.MaxRestartDelay != 300*time.Second {
		t.Errorf("Stream.MaxRestartDelay = %v, want 300s", cfg.Stream.MaxRestartDelay)
	}
	if cfg.Stream.MaxRestartAttempts != 50 {
		t.Errorf("Stream.MaxRestartAttempts = %d, want 50", cfg.Stream.MaxRestartAttempts)
	}
	if cfg.Stream.USBStabilizationDelay != 5*time.Second {
		t.Errorf("Stream.USBStabilizationDelay = %v, want 5s", cfg.Stream.USBStabilizationDelay)
	}

	// Verify MediaMTX settings
	if cfg.MediaMTX.APIURL != "http://localhost:9997" {
		t.Errorf("MediaMTX.APIURL = %q, want \"http://localhost:9997\"", cfg.MediaMTX.APIURL)
	}
	if cfg.MediaMTX.RTSPURL != "rtsp://localhost:8554" {
		t.Errorf("MediaMTX.RTSPURL = %q, want \"rtsp://localhost:8554\"", cfg.MediaMTX.RTSPURL)
	}
	if cfg.MediaMTX.ConfigPath != "/etc/mediamtx/mediamtx.yml" {
		t.Errorf("MediaMTX.ConfigPath = %q, want \"/etc/mediamtx/mediamtx.yml\"", cfg.MediaMTX.ConfigPath)
	}

	// Verify monitor settings
	if !cfg.Monitor.Enabled {
		t.Error("Monitor.Enabled = false, want true")
	}
	if cfg.Monitor.Interval != 5*time.Minute {
		t.Errorf("Monitor.Interval = %v, want 5m", cfg.Monitor.Interval)
	}
	if !cfg.Monitor.RestartUnhealthy {
		t.Error("Monitor.RestartUnhealthy = false, want true")
	}
}

// TestLoadConfigDevices verifies device-specific configuration parsing.
func TestLoadConfigDevices(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify device count
	if len(cfg.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want 2", len(cfg.Devices))
	}

	// Verify blue_yeti device
	blueYeti, ok := cfg.Devices["blue_yeti"]
	if !ok {
		t.Fatal("blue_yeti device not found in config")
	}
	if blueYeti.SampleRate != 48000 {
		t.Errorf("blue_yeti.SampleRate = %d, want 48000", blueYeti.SampleRate)
	}
	if blueYeti.Channels != 2 {
		t.Errorf("blue_yeti.Channels = %d, want 2", blueYeti.Channels)
	}
	if blueYeti.Bitrate != "192k" {
		t.Errorf("blue_yeti.Bitrate = %q, want \"192k\"", blueYeti.Bitrate)
	}
	if blueYeti.Codec != "opus" {
		t.Errorf("blue_yeti.Codec = %q, want \"opus\"", blueYeti.Codec)
	}
	if blueYeti.ThreadQueue != 8192 {
		t.Errorf("blue_yeti.ThreadQueue = %d, want 8192", blueYeti.ThreadQueue)
	}

	// Verify usb_audio_1 device
	usbAudio, ok := cfg.Devices["usb_audio_1"]
	if !ok {
		t.Fatal("usb_audio_1 device not found in config")
	}
	if usbAudio.SampleRate != 44100 {
		t.Errorf("usb_audio_1.SampleRate = %d, want 44100", usbAudio.SampleRate)
	}
	if usbAudio.Channels != 1 {
		t.Errorf("usb_audio_1.Channels = %d, want 1", usbAudio.Channels)
	}
	if usbAudio.Bitrate != "128k" {
		t.Errorf("usb_audio_1.Bitrate = %q, want \"128k\"", usbAudio.Bitrate)
	}
	if usbAudio.Codec != "opus" {
		t.Errorf("usb_audio_1.Codec = %q, want \"opus\"", usbAudio.Codec)
	}
	// Note: usb_audio_1 doesn't specify thread_queue, so it should be 0 (unset)
	if usbAudio.ThreadQueue != 0 {
		t.Errorf("usb_audio_1.ThreadQueue = %d, want 0", usbAudio.ThreadQueue)
	}
}

// TestLoadConfigMissingFile verifies error handling for missing files.
func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("LoadConfig() expected error for missing file, got nil")
	}
}

// TestLoadConfigInvalidYAML verifies error handling for invalid YAML.
func TestLoadConfigInvalidYAML(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "invalid.yaml")

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("LoadConfig() expected error for invalid YAML, got nil")
	}
}

// FuzzLoadConfig fuzz tests the YAML config loading path with arbitrary input.
//
// Invariants verified:
//   - No panics on any input
//   - If LoadConfig returns a non-nil *Config without error, the config is valid
//   - If LoadConfig returns an error, cfg is nil
func FuzzLoadConfig(f *testing.F) {
	// Seed corpus: valid YAML configs, invalid YAML, and edge cases
	seeds := []string{
		// Minimal valid config
		`default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
`,
		// Full valid config
		`devices:
  blue_yeti:
    sample_rate: 48000
    channels: 2
    bitrate: "192k"
    codec: opus
    thread_queue: 8192
default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
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
`,
		// Valid YAML but invalid config (missing required fields)
		`default:
  sample_rate: 0
  channels: 2
  bitrate: "128k"
  codec: opus
`,
		// Valid YAML with aac codec
		`default:
  sample_rate: 44100
  channels: 1
  bitrate: "256k"
  codec: aac
`,
		// Invalid YAML
		"not: valid: yaml: [",
		"{{{invalid",
		"---\n- - -\n  broken",

		// Empty input
		"",

		// Just whitespace
		"   \n\n\t  ",

		// YAML with unexpected types
		"default: 42",
		"default: [1, 2, 3]",
		"devices: true",

		// YAML with deeply nested structures
		`default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
devices:
  dev1:
    codec: opus
  dev2:
    codec: aac
  dev3:
    sample_rate: 44100
`,
		// YAML with special characters in keys
		"\"special key\": value\n",

		// YAML with very large numbers
		`default:
  sample_rate: 999999999
  channels: 2
  bitrate: "128k"
  codec: opus
`,
		// YAML with negative numbers
		`default:
  sample_rate: -1
  channels: -5
  bitrate: "128k"
  codec: opus
`,
		// YAML with invalid codec
		`default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: mp3
`,
		// Binary-looking content
		"\x00\x01\x02\x03",
		"\xff\xfe\xfd",

		// YAML bomb / alias expansion
		"a: &a\n  b: *a\n",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data string) {
		// Write fuzz data to a temp file since LoadConfig reads from a file path
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "fuzz_config.yaml")
		if err := os.WriteFile(configPath, []byte(data), 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}

		cfg, err := LoadConfig(configPath)

		// Invariant 1: If no error, cfg must not be nil
		if err == nil && cfg == nil {
			t.Error("LoadConfig returned nil config without error")
		}

		// Invariant 2: If error, cfg must be nil
		if err != nil && cfg != nil {
			t.Errorf("LoadConfig returned non-nil config with error: %v", err)
		}

		// Invariant 3: If config loaded successfully, it must pass validation
		if err == nil && cfg != nil {
			if validErr := cfg.Validate(); validErr != nil {
				t.Errorf("LoadConfig returned config that fails validation: %v", validErr)
			}

			// Invariant 4: GetDeviceConfig must not panic for any device name
			_ = cfg.GetDeviceConfig("blue_yeti")
			_ = cfg.GetDeviceConfig("nonexistent")
			_ = cfg.GetDeviceConfig("")
		}
	})
}
