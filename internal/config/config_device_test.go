package config

import (
	"path/filepath"
	"testing"
)

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
