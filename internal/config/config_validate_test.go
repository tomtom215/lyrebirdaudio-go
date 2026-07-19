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
					SegmentDuration:       3600,
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
				Stream: StreamConfig{
					InitialRestartDelay: 10 * time.Second,
					MaxRestartDelay:     300 * time.Second,
					MaxRestartAttempts:  50,
					SegmentDuration:     3600,
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

// TestStreamConfigValidate tests SegmentFormat, size and restart-timing
// validation. Each case starts from a known-valid base (the defaults) and
// mutates one field, so a single invalid field is what triggers the error.
func TestStreamConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(s *StreamConfig)
		wantErr     bool
		errContains string
	}{
		{name: "defaults are valid", mutate: func(s *StreamConfig) {}},
		{name: "empty segment format is valid", mutate: func(s *StreamConfig) { s.SegmentFormat = "" }},
		{name: "wav is valid", mutate: func(s *StreamConfig) { s.SegmentFormat = "wav" }},
		{name: "flac is valid", mutate: func(s *StreamConfig) { s.SegmentFormat = "flac" }},
		{name: "ogg is valid", mutate: func(s *StreamConfig) { s.SegmentFormat = "ogg" }},
		{name: "mp3 is invalid", mutate: func(s *StreamConfig) { s.SegmentFormat = "mp3" }, wantErr: true, errContains: "segment_format"},
		{name: "xyz is invalid", mutate: func(s *StreamConfig) { s.SegmentFormat = "xyz" }, wantErr: true, errContains: "segment_format"},
		{name: "WAV uppercase is invalid", mutate: func(s *StreamConfig) { s.SegmentFormat = "WAV" }, wantErr: true, errContains: "segment_format"},
		{name: "negative total bytes is invalid", mutate: func(s *StreamConfig) { s.SegmentMaxTotalBytes = -1 }, wantErr: true, errContains: "segment_max_total_bytes"},
		{name: "zero total bytes is valid (disabled)", mutate: func(s *StreamConfig) { s.SegmentMaxTotalBytes = 0 }},
		{name: "positive total bytes is valid", mutate: func(s *StreamConfig) { s.SegmentMaxTotalBytes = 32 * 1024 * 1024 * 1024 }},

		// Restart/backoff timing (H7).
		{name: "zero max_restart_attempts is invalid", mutate: func(s *StreamConfig) { s.MaxRestartAttempts = 0 }, wantErr: true, errContains: "max_restart_attempts"},
		{name: "nanosecond initial delay is invalid", mutate: func(s *StreamConfig) { s.InitialRestartDelay = 45 }, wantErr: true, errContains: "initial_restart_delay"},
		{name: "max delay below initial is invalid", mutate: func(s *StreamConfig) {
			s.InitialRestartDelay = 10 * time.Second
			s.MaxRestartDelay = time.Second
		}, wantErr: true, errContains: "max_restart_delay"},
		{name: "zero segment duration is invalid", mutate: func(s *StreamConfig) { s.SegmentDuration = 0 }, wantErr: true, errContains: "segment_duration"},
		{name: "negative stop timeout is invalid", mutate: func(s *StreamConfig) { s.StopTimeout = -1 }, wantErr: true, errContains: "stop_timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig().Stream // known-valid base
			tt.mutate(&cfg)
			err := cfg.Validate()
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

// TestConfigValidateRecordingCodecContainer verifies the local-recording
// codec/container compatibility check. Pairings verified empirically against
// ffmpeg 7.x: opus records only as ogg, aac only as wav. The check applies only
// when recording is enabled.
func TestConfigValidateRecordingCodecContainer(t *testing.T) {
	tests := []struct {
		name        string
		codec       string
		segFormat   string
		record      bool
		wantErr     bool
		errContains string
	}{
		{name: "opus+ogg records", codec: "opus", segFormat: "ogg", record: true},
		{name: "aac+wav records", codec: "aac", segFormat: "wav", record: true},
		{name: "opus+wav rejected", codec: "opus", segFormat: "wav", record: true, wantErr: true, errContains: "ogg"},
		{name: "opus+flac rejected", codec: "opus", segFormat: "flac", record: true, wantErr: true, errContains: "ogg"},
		{name: "aac+ogg rejected", codec: "aac", segFormat: "ogg", record: true, wantErr: true, errContains: "wav"},
		{name: "aac+flac rejected", codec: "aac", segFormat: "flac", record: true, wantErr: true, errContains: "wav"},
		{name: "incompatible pairing ok when recording disabled", codec: "opus", segFormat: "wav", record: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Default.Codec = tt.codec
			cfg.Stream.SegmentFormat = tt.segFormat
			if tt.record {
				cfg.Stream.LocalRecordDir = "/var/lib/lyrebird/rec"
			} else {
				cfg.Stream.LocalRecordDir = ""
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

// TestConfigValidateRecordingDeviceCodecMismatch verifies a per-device codec
// incompatible with the global segment_format is caught and names the device.
func TestConfigValidateRecordingDeviceCodecMismatch(t *testing.T) {
	cfg := DefaultConfig() // default opus + ogg
	cfg.Stream.LocalRecordDir = "/var/lib/lyrebird/rec"
	cfg.Devices = map[string]DeviceConfig{
		"aac_mic": {Codec: "aac"}, // aac needs wav, but segment_format is ogg
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for aac device with ogg segment_format")
	}
	if !strings.Contains(err.Error(), "aac_mic") {
		t.Errorf("error should name the offending device, got: %v", err)
	}
}

// TestDefaultConfigRecordsCleanly guards against a default regression: the
// default config must record without a codec/container error once a record dir
// is set (default opus codec requires an ogg segment_format).
func TestDefaultConfigRecordsCleanly(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Stream.SegmentFormat != "ogg" {
		t.Errorf("default SegmentFormat = %q, want ogg (must match the default opus codec)", cfg.Stream.SegmentFormat)
	}
	cfg.Stream.LocalRecordDir = "/var/lib/lyrebird/rec"
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config with recording enabled should validate, got: %v", err)
	}
}
