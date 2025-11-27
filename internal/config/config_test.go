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

// TestGetDeviceConfig verifies device lookup with default fallback.
func TestGetDeviceConfig(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	tests := []struct {
		name           string
		deviceName     string
		wantSampleRate int
		wantChannels   int
		wantBitrate    string
		wantCodec      string
		wantThreadQue  int
	}{
		{
			name:           "blue_yeti - device-specific config",
			deviceName:     "blue_yeti",
			wantSampleRate: 48000,
			wantChannels:   2,
			wantBitrate:    "192k",
			wantCodec:      "opus",
			wantThreadQue:  8192,
		},
		{
			name:           "usb_audio_1 - device-specific config",
			deviceName:     "usb_audio_1",
			wantSampleRate: 44100,
			wantChannels:   1,
			wantBitrate:    "128k",
			wantCodec:      "opus",
			wantThreadQue:  8192, // Should fall back to default
		},
		{
			name:           "unknown_device - falls back to default",
			deviceName:     "unknown_device",
			wantSampleRate: 48000,
			wantChannels:   2,
			wantBitrate:    "128k",
			wantCodec:      "opus",
			wantThreadQue:  8192,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devCfg := cfg.GetDeviceConfig(tt.deviceName)

			if devCfg.SampleRate != tt.wantSampleRate {
				t.Errorf("SampleRate = %d, want %d", devCfg.SampleRate, tt.wantSampleRate)
			}
			if devCfg.Channels != tt.wantChannels {
				t.Errorf("Channels = %d, want %d", devCfg.Channels, tt.wantChannels)
			}
			if devCfg.Bitrate != tt.wantBitrate {
				t.Errorf("Bitrate = %q, want %q", devCfg.Bitrate, tt.wantBitrate)
			}
			if devCfg.Codec != tt.wantCodec {
				t.Errorf("Codec = %q, want %q", devCfg.Codec, tt.wantCodec)
			}
			if devCfg.ThreadQueue != tt.wantThreadQue {
				t.Errorf("ThreadQueue = %d, want %d", devCfg.ThreadQueue, tt.wantThreadQue)
			}
		})
	}
}

// TestValidateConfig verifies configuration validation.
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				Default: DeviceConfig{
					SampleRate:  48000,
					Channels:    2,
					Bitrate:     "128k",
					Codec:       "opus",
					ThreadQueue: 8192,
				},
				Stream: StreamConfig{
					InitialRestartDelay:   10 * time.Second,
					MaxRestartDelay:       300 * time.Second,
					MaxRestartAttempts:    50,
					USBStabilizationDelay: 5 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid sample rate - zero",
			config: &Config{
				Default: DeviceConfig{
					SampleRate: 0,
					Channels:   2,
					Bitrate:    "128k",
					Codec:      "opus",
				},
			},
			wantErr: true,
			errMsg:  "default config: sample_rate must be positive",
		},
		{
			name: "invalid sample rate - negative",
			config: &Config{
				Default: DeviceConfig{
					SampleRate: -1,
					Channels:   2,
					Bitrate:    "128k",
					Codec:      "opus",
				},
			},
			wantErr: true,
			errMsg:  "default config: sample_rate must be positive",
		},
		{
			name: "invalid channels - zero",
			config: &Config{
				Default: DeviceConfig{
					SampleRate: 48000,
					Channels:   0,
					Bitrate:    "128k",
					Codec:      "opus",
				},
			},
			wantErr: true,
			errMsg:  "default config: channels must be positive",
		},
		{
			name: "invalid channels - too many",
			config: &Config{
				Default: DeviceConfig{
					SampleRate: 48000,
					Channels:   33,
					Bitrate:    "128k",
					Codec:      "opus",
				},
			},
			wantErr: true,
			errMsg:  "default config: channels must be between 1 and 32",
		},
		{
			name: "invalid bitrate - empty",
			config: &Config{
				Default: DeviceConfig{
					SampleRate: 48000,
					Channels:   2,
					Bitrate:    "",
					Codec:      "opus",
				},
			},
			wantErr: true,
			errMsg:  "default config: bitrate cannot be empty",
		},
		{
			name: "invalid codec - empty",
			config: &Config{
				Default: DeviceConfig{
					SampleRate: 48000,
					Channels:   2,
					Bitrate:    "128k",
					Codec:      "",
				},
			},
			wantErr: true,
			errMsg:  "default config: codec cannot be empty",
		},
		{
			name: "invalid codec - unsupported",
			config: &Config{
				Default: DeviceConfig{
					SampleRate: 48000,
					Channels:   2,
					Bitrate:    "128k",
					Codec:      "mp3",
				},
			},
			wantErr: true,
			errMsg:  "default config: codec must be opus or aac",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
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

// TestDefaultConfig verifies default configuration values.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify default device settings
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

	// Verify default stream settings
	if cfg.Stream.InitialRestartDelay != 10*time.Second {
		t.Errorf("Stream.InitialRestartDelay = %v, want 10s", cfg.Stream.InitialRestartDelay)
	}
	if cfg.Stream.MaxRestartDelay != 300*time.Second {
		t.Errorf("Stream.MaxRestartDelay = %v, want 300s", cfg.Stream.MaxRestartDelay)
	}
	if cfg.Stream.MaxRestartAttempts != 50 {
		t.Errorf("Stream.MaxRestartAttempts = %d, want 50", cfg.Stream.MaxRestartAttempts)
	}

	// Verify default MediaMTX settings
	if cfg.MediaMTX.APIURL != "http://localhost:9997" {
		t.Errorf("MediaMTX.APIURL = %q, want \"http://localhost:9997\"", cfg.MediaMTX.APIURL)
	}

	// Verify default monitor settings
	if !cfg.Monitor.Enabled {
		t.Error("Monitor.Enabled = false, want true")
	}
}

// TestSaveConfig verifies configuration file writing.
func TestSaveConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Devices = map[string]DeviceConfig{
		"test_device": {
			SampleRate:  44100,
			Channels:    1,
			Bitrate:     "96k",
			Codec:       "opus",
			ThreadQueue: 4096,
		},
	}

	// Write to temp file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	err := cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Save() did not create config file")
	}

	// Load and verify
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() after Save() error = %v", err)
	}

	// Verify device was saved
	testDev, ok := loaded.Devices["test_device"]
	if !ok {
		t.Fatal("test_device not found in saved config")
	}
	if testDev.SampleRate != 44100 {
		t.Errorf("test_device.SampleRate = %d, want 44100", testDev.SampleRate)
	}
}

// BenchmarkLoadConfig measures config loading performance.
func BenchmarkLoadConfig(b *testing.B) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadConfig(configPath)
	}
}

// BenchmarkGetDeviceConfig measures device lookup performance.
func BenchmarkGetDeviceConfig(b *testing.B) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	cfg, _ := LoadConfig(configPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetDeviceConfig("blue_yeti")
	}
}
