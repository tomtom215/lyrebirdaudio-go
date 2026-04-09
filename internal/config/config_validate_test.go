package config

import (
	"strings"
	"testing"
	"time"
)

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

// TestStreamConfigValidate tests SegmentFormat validation (GAP-1b / A-2).
func TestStreamConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		cfg         StreamConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "empty segment format is valid (uses default wav)",
			cfg:  StreamConfig{SegmentFormat: ""},
		},
		{
			name: "wav is valid",
			cfg:  StreamConfig{SegmentFormat: "wav"},
		},
		{
			name: "flac is valid",
			cfg:  StreamConfig{SegmentFormat: "flac"},
		},
		{
			name: "ogg is valid",
			cfg:  StreamConfig{SegmentFormat: "ogg"},
		},
		{
			name:        "mp3 is invalid",
			cfg:         StreamConfig{SegmentFormat: "mp3"},
			wantErr:     true,
			errContains: "segment_format",
		},
		{
			name:        "xyz is invalid",
			cfg:         StreamConfig{SegmentFormat: "xyz"},
			wantErr:     true,
			errContains: "segment_format",
		},
		{
			name:        "WAV uppercase is invalid (must be lowercase)",
			cfg:         StreamConfig{SegmentFormat: "WAV"},
			wantErr:     true,
			errContains: "segment_format",
		},
		{
			name:        "negative total bytes is invalid",
			cfg:         StreamConfig{SegmentMaxTotalBytes: -1},
			wantErr:     true,
			errContains: "segment_max_total_bytes",
		},
		{
			name: "zero total bytes is valid (disabled)",
			cfg:  StreamConfig{SegmentMaxTotalBytes: 0},
		},
		{
			name: "positive total bytes is valid",
			cfg:  StreamConfig{SegmentMaxTotalBytes: 32 * 1024 * 1024 * 1024},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("StreamConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("StreamConfig.Validate() error = %q, want to contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestConfigValidateStreamConfig verifies that Config.Validate() calls StreamConfig.Validate().
func TestConfigValidateStreamConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Stream.SegmentFormat = "mp3" // invalid
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Config.Validate() should fail for invalid SegmentFormat")
	}
	if !strings.Contains(err.Error(), "stream config") {
		t.Errorf("error should mention 'stream config', got: %v", err)
	}
}
