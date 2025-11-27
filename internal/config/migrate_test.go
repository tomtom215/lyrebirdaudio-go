package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateFromBash verifies migration from bash environment variables to YAML.
//
// The bash version used environment variables in a shell script:
//
//	SAMPLE_RATE_blue_yeti=48000
//	CHANNELS_blue_yeti=2
//	BITRATE_blue_yeti=192k
//	CODEC_blue_yeti=opus
//	THREAD_QUEUE_SIZE_blue_yeti=8192
//
// This must be converted to YAML format:
//
//	devices:
//	  blue_yeti:
//	    sample_rate: 48000
//	    channels: 2
//	    bitrate: 192k
//	    codec: opus
//	    thread_queue: 8192
func TestMigrateFromBash(t *testing.T) {
	bashConfigPath := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")

	cfg, err := MigrateFromBash(bashConfigPath)
	if err != nil {
		t.Fatalf("MigrateFromBash() error = %v", err)
	}

	// Verify blue_yeti device was migrated
	blueYeti, ok := cfg.Devices["blue_yeti"]
	if !ok {
		t.Fatal("blue_yeti device not found after migration")
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

	// Verify usb_audio_1 device was migrated
	usbAudio, ok := cfg.Devices["usb_audio_1"]
	if !ok {
		t.Fatal("usb_audio_1 device not found after migration")
	}
	if usbAudio.SampleRate != 44100 {
		t.Errorf("usb_audio_1.SampleRate = %d, want 44100", usbAudio.SampleRate)
	}
	if usbAudio.Channels != 1 {
		t.Errorf("usb_audio_1.Channels = %d, want 1", usbAudio.Channels)
	}
}

// TestMigrateFromBashDefaults verifies migration of default settings.
func TestMigrateFromBashDefaults(t *testing.T) {
	bashConfigPath := filepath.Join("..", "..", "testdata", "config", "bash-env-defaults.conf")

	cfg, err := MigrateFromBash(bashConfigPath)
	if err != nil {
		t.Fatalf("MigrateFromBash() error = %v", err)
	}

	// Verify default settings were migrated
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
}

// TestMigrateFromBashMissingFile verifies error handling for missing files.
func TestMigrateFromBashMissingFile(t *testing.T) {
	_, err := MigrateFromBash("/nonexistent/bash.conf")
	if err == nil {
		t.Error("MigrateFromBash() expected error for missing file, got nil")
	}
}

// TestMigrateAndSave verifies full migration workflow.
func TestMigrateAndSave(t *testing.T) {
	bashConfigPath := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")

	// Migrate from bash
	cfg, err := MigrateFromBash(bashConfigPath)
	if err != nil {
		t.Fatalf("MigrateFromBash() error = %v", err)
	}

	// Save to temp file
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "config.yaml")

	err = cfg.Save(yamlPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Error("Save() did not create YAML file")
	}

	// Load and verify
	loaded, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatalf("LoadConfig() after migration error = %v", err)
	}

	// Verify device count matches
	if len(loaded.Devices) != len(cfg.Devices) {
		t.Errorf("Device count mismatch after migration: got %d, want %d",
			len(loaded.Devices), len(cfg.Devices))
	}

	// Verify blue_yeti device
	blueYeti, ok := loaded.Devices["blue_yeti"]
	if !ok {
		t.Fatal("blue_yeti device lost after migration and reload")
	}
	if blueYeti.SampleRate != 48000 {
		t.Errorf("blue_yeti.SampleRate = %d, want 48000 after migration", blueYeti.SampleRate)
	}
}

// TestParseBashEnvLine verifies individual bash environment variable parsing.
func TestParseBashEnvLine(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantVar    string
		wantDevice string
		wantValue  string
		wantOK     bool
	}{
		{
			name:       "sample rate",
			line:       "SAMPLE_RATE_blue_yeti=48000",
			wantVar:    "SAMPLE_RATE",
			wantDevice: "blue_yeti",
			wantValue:  "48000",
			wantOK:     true,
		},
		{
			name:       "channels",
			line:       "CHANNELS_usb_audio_1=1",
			wantVar:    "CHANNELS",
			wantDevice: "usb_audio_1",
			wantValue:  "1",
			wantOK:     true,
		},
		{
			name:       "bitrate",
			line:       "BITRATE_blue_yeti=192k",
			wantVar:    "BITRATE",
			wantDevice: "blue_yeti",
			wantValue:  "192k",
			wantOK:     true,
		},
		{
			name:       "codec",
			line:       "CODEC_blue_yeti=opus",
			wantVar:    "CODEC",
			wantDevice: "blue_yeti",
			wantValue:  "opus",
			wantOK:     true,
		},
		{
			name:       "thread queue",
			line:       "THREAD_QUEUE_SIZE_blue_yeti=8192",
			wantVar:    "THREAD_QUEUE_SIZE",
			wantDevice: "blue_yeti",
			wantValue:  "8192",
			wantOK:     true,
		},
		{
			name:       "comment line",
			line:       "# This is a comment",
			wantVar:    "",
			wantDevice: "",
			wantValue:  "",
			wantOK:     false,
		},
		{
			name:       "empty line",
			line:       "",
			wantVar:    "",
			wantDevice: "",
			wantValue:  "",
			wantOK:     false,
		},
		{
			name:       "export prefix",
			line:       "export SAMPLE_RATE_blue_yeti=48000",
			wantVar:    "SAMPLE_RATE",
			wantDevice: "blue_yeti",
			wantValue:  "48000",
			wantOK:     true,
		},
		{
			name:       "quoted value",
			line:       "CODEC_blue_yeti=\"opus\"",
			wantVar:    "CODEC",
			wantDevice: "blue_yeti",
			wantValue:  "opus",
			wantOK:     true,
		},
		{
			name:       "default variable (no device suffix)",
			line:       "DEFAULT_SAMPLE_RATE=48000",
			wantVar:    "DEFAULT_SAMPLE_RATE",
			wantDevice: "",
			wantValue:  "48000",
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVar, gotDevice, gotValue, gotOK := parseBashEnvLine(tt.line)

			if gotOK != tt.wantOK {
				t.Errorf("parseBashEnvLine() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotVar != tt.wantVar {
				t.Errorf("parseBashEnvLine() var = %q, want %q", gotVar, tt.wantVar)
			}
			if gotDevice != tt.wantDevice {
				t.Errorf("parseBashEnvLine() device = %q, want %q", gotDevice, tt.wantDevice)
			}
			if gotValue != tt.wantValue {
				t.Errorf("parseBashEnvLine() value = %q, want %q", gotValue, tt.wantValue)
			}
		})
	}
}

// BenchmarkMigrateFromBash measures migration performance.
func BenchmarkMigrateFromBash(b *testing.B) {
	bashConfigPath := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MigrateFromBash(bashConfigPath)
	}
}
