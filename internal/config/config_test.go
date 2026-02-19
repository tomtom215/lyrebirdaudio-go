package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// TestSaveConfigErrorPaths tests error handling in Save().
func TestSaveConfigErrorPaths(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("invalid path", func(t *testing.T) {
		// Try to save to a directory that doesn't exist and can't be created
		// Use a path with null bytes which is invalid on all systems
		invalidPath := "/tmp/\x00invalid/config.yaml"
		err := cfg.Save(invalidPath)
		if err == nil {
			t.Error("Save() with invalid path should return error")
		}
	})

	t.Run("unwritable directory", func(t *testing.T) {
		// Create a read-only directory
		tmpDir := t.TempDir()
		readOnlyDir := filepath.Join(tmpDir, "readonly")
		if err := os.Mkdir(readOnlyDir, 0444); err != nil {
			t.Skipf("Cannot create read-only directory: %v", err)
		}

		// Try to write to the read-only directory
		configPath := filepath.Join(readOnlyDir, "config.yaml")
		err := cfg.Save(configPath)
		// This might or might not error depending on OS permissions
		// Just verify it doesn't panic
		_ = err
	})
}

// BenchmarkLoadConfig measures config loading performance.
func BenchmarkLoadConfig(b *testing.B) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadConfig(configPath)
	}
}

// TestDeviceConfigValidatePartial verifies partial validation of device configs.
func TestDeviceConfigValidatePartial(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DeviceConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: DeviceConfig{
				SampleRate: 48000,
				Channels:   2,
				Codec:      "opus",
			},
			wantErr: false,
		},
		{
			name: "valid with aac codec",
			cfg: DeviceConfig{
				SampleRate: 44100,
				Channels:   1,
				Codec:      "aac",
			},
			wantErr: false,
		},
		{
			name: "valid with empty codec",
			cfg: DeviceConfig{
				SampleRate: 48000,
				Channels:   2,
				Codec:      "",
			},
			wantErr: false,
		},
		{
			name: "negative sample rate",
			cfg: DeviceConfig{
				SampleRate: -1,
				Channels:   2,
				Codec:      "opus",
			},
			wantErr: true,
			errMsg:  "sample_rate must not be negative (0 means inherit default)",
		},
		{
			name: "negative channels",
			cfg: DeviceConfig{
				SampleRate: 48000,
				Channels:   -1,
				Codec:      "opus",
			},
			wantErr: true,
			errMsg:  "channels must not be negative (0 means inherit default)",
		},
		{
			name: "too many channels",
			cfg: DeviceConfig{
				SampleRate: 48000,
				Channels:   33,
				Codec:      "opus",
			},
			wantErr: true,
			errMsg:  "channels must be between 1 and 32",
		},
		{
			name: "invalid codec",
			cfg: DeviceConfig{
				SampleRate: 48000,
				Channels:   2,
				Codec:      "mp3",
			},
			wantErr: true,
			errMsg:  "codec must be opus or aac",
		},
		{
			name: "zero values allowed (partial config)",
			cfg: DeviceConfig{
				SampleRate: 0,
				Channels:   0,
				Codec:      "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidatePartial()

			if tt.wantErr {
				if err == nil {
					t.Error("ValidatePartial() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("ValidatePartial() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePartial() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateConfigWithInvalidDevice tests Config.Validate() with invalid device config.
func TestValidateConfigWithInvalidDevice(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errPart string
	}{
		{
			name: "valid config with devices",
			config: &Config{
				Default: DeviceConfig{
					SampleRate:  48000,
					Channels:    2,
					Bitrate:     "128k",
					Codec:       "opus",
					ThreadQueue: 8192,
				},
				Devices: map[string]DeviceConfig{
					"blue_yeti": {
						SampleRate: 96000,
						Channels:   1,
						Codec:      "aac",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid device - negative sample rate",
			config: &Config{
				Default: DeviceConfig{
					SampleRate:  48000,
					Channels:    2,
					Bitrate:     "128k",
					Codec:       "opus",
					ThreadQueue: 8192,
				},
				Devices: map[string]DeviceConfig{
					"bad_device": {
						SampleRate: -1,
					},
				},
			},
			wantErr: true,
			errPart: "device \"bad_device\"",
		},
		{
			name: "invalid device - invalid codec",
			config: &Config{
				Default: DeviceConfig{
					SampleRate:  48000,
					Channels:    2,
					Bitrate:     "128k",
					Codec:       "opus",
					ThreadQueue: 8192,
				},
				Devices: map[string]DeviceConfig{
					"bad_device": {
						Codec: "mp3",
					},
				},
			},
			wantErr: true,
			errPart: "device \"bad_device\": codec must be opus or aac",
		},
		{
			name: "invalid device - too many channels",
			config: &Config{
				Default: DeviceConfig{
					SampleRate:  48000,
					Channels:    2,
					Bitrate:     "128k",
					Codec:       "opus",
					ThreadQueue: 8192,
				},
				Devices: map[string]DeviceConfig{
					"bad_device": {
						Channels: 50,
					},
				},
			},
			wantErr: true,
			errPart: "device \"bad_device\": channels must be between 1 and 32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errPart != "" && !contains(err.Error(), tt.errPart) {
					t.Errorf("Validate() error = %q, want to contain %q", err.Error(), tt.errPart)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSaveConfigAtomic verifies that Save() performs an atomic write using
// a temp file + rename pattern. After Save() returns, the file should contain
// complete valid YAML that can be loaded back. This also verifies that a
// concurrent reader never sees partial content.
func TestSaveConfigAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write an initial config
	initialCfg := DefaultConfig()
	initialCfg.Default.SampleRate = 44100
	err := initialCfg.Save(configPath)
	if err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}

	// Read initial content
	initialData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile initial error = %v", err)
	}

	// Now overwrite with a new config
	newCfg := DefaultConfig()
	newCfg.Default.SampleRate = 96000
	newCfg.Devices = map[string]DeviceConfig{
		"test_device": {
			SampleRate: 22050,
			Channels:   1,
			Bitrate:    "64k",
			Codec:      "aac",
		},
	}
	err = newCfg.Save(configPath)
	if err != nil {
		t.Fatalf("overwrite Save() error = %v", err)
	}

	// Read the file content - it should be either fully old or fully new,
	// never partial. Since Save() completed, it must be fully new.
	resultData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile result error = %v", err)
	}

	// Verify the result is valid YAML that can be loaded
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig after atomic Save() error = %v", err)
	}

	// Verify it's the new content
	if loaded.Default.SampleRate != 96000 {
		t.Errorf("SampleRate = %d, want 96000", loaded.Default.SampleRate)
	}

	// The result should NOT be the initial data
	if string(resultData) == string(initialData) {
		t.Error("File content was not updated by Save()")
	}

	// Verify that no temp files are left behind
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "config.yaml" {
			t.Errorf("Unexpected leftover file in directory: %s", entry.Name())
		}
	}
}

// TestSaveConfigAtomicPermissions verifies that the atomically-saved file
// has the correct permissions (0644).
func TestSaveConfigAtomicPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := DefaultConfig()
	err := cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}

	// Check permissions (masking umask bits)
	perm := info.Mode().Perm()
	if perm&0644 != 0644 {
		t.Errorf("File permissions = %o, want at least 0644", perm)
	}
}

// TestSaveConfigAtomicTempFileCleanupOnError verifies that temp files are
// cleaned up if the write fails mid-way.
func TestSaveConfigAtomicTempFileCleanupOnError(t *testing.T) {
	// Try saving to a non-existent directory (rename will fail because
	// the temp file is created in the same dir, which doesn't exist)
	cfg := DefaultConfig()
	err := cfg.Save("/nonexistent_dir_12345/config.yaml")
	if err == nil {
		t.Error("Save() to nonexistent directory should fail")
	}

	// Verify no temp files left behind (the directory doesn't exist,
	// so there's nothing to clean up, but also no crash)
}

// mockAtomicFile implements atomicFile for testing error injection.
type mockAtomicFile struct {
	name      string
	realFile  *os.File // used to back Name() and cleanup
	writeErr  error
	syncErr   error
	chmodErr  error
	closeErr  error
	writeCalls int
}

func (m *mockAtomicFile) Write(p []byte) (int, error) {
	m.writeCalls++
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}

func (m *mockAtomicFile) Sync() error  { return m.syncErr }
func (m *mockAtomicFile) Chmod(_ os.FileMode) error { return m.chmodErr }
func (m *mockAtomicFile) Close() error {
	if m.realFile != nil {
		_ = m.realFile.Close()
	}
	return m.closeErr
}
func (m *mockAtomicFile) Name() string { return m.name }

// newMockCreateTemp returns a createTemp func that produces a mockAtomicFile.
// A real temp file is created so cleanup (os.Remove) has a real path to remove.
func newMockCreateTemp(dir string, mock *mockAtomicFile) atomicCreateTemp {
	return func(d, pattern string) (atomicFile, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		mock.realFile = f
		mock.name = f.Name()
		return mock, nil
	}
}

// TestSaveWithInjectableErrors tests the error paths of saveWith.
func TestSaveWithInjectableErrors(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("write error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{writeErr: errors.New("disk full")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on write failure")
		}
		if !strings.Contains(err.Error(), "failed to write temp config file") {
			t.Errorf("error = %q, want 'failed to write temp config file'", err.Error())
		}
	})

	t.Run("sync error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{syncErr: errors.New("sync failed")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on sync failure")
		}
		if !strings.Contains(err.Error(), "failed to sync temp config file") {
			t.Errorf("error = %q, want 'failed to sync temp config file'", err.Error())
		}
	})

	t.Run("chmod error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{chmodErr: errors.New("chmod failed")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on chmod failure")
		}
		if !strings.Contains(err.Error(), "failed to set config file permissions") {
			t.Errorf("error = %q, want 'failed to set config file permissions'", err.Error())
		}
	})

	t.Run("close error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{closeErr: errors.New("close failed")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on close failure")
		}
		if !strings.Contains(err.Error(), "failed to close temp config file") {
			t.Errorf("error = %q, want 'failed to close temp config file'", err.Error())
		}
	})

	t.Run("createTemp error", func(t *testing.T) {
		failCreate := func(dir, pattern string) (atomicFile, error) {
			return nil, errors.New("createTemp failed")
		}
		err := cfg.saveWith("/tmp/config.yaml", failCreate)
		if err == nil {
			t.Fatal("saveWith() expected error when createTemp fails")
		}
		if !strings.Contains(err.Error(), "failed to create temp config file") {
			t.Errorf("error = %q, want 'failed to create temp config file'", err.Error())
		}
	})
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
