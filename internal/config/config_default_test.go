package config

import (
	"testing"
	"time"
)

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

// TestDefaultConfigSegmentRetentionDefaults verifies default retention settings.
func TestDefaultConfigSegmentRetentionDefaults(t *testing.T) {
	cfg := DefaultConfig()

	wantMaxAge := 7 * 24 * time.Hour
	if cfg.Stream.SegmentMaxAge != wantMaxAge {
		t.Errorf("SegmentMaxAge = %v, want %v", cfg.Stream.SegmentMaxAge, wantMaxAge)
	}
	if cfg.Stream.SegmentMaxTotalBytes != 0 {
		t.Errorf("SegmentMaxTotalBytes = %d, want 0 (disabled by default)", cfg.Stream.SegmentMaxTotalBytes)
	}
}

// TestDefaultConfigHealthAddr verifies the health address default.
func TestDefaultConfigHealthAddr(t *testing.T) {
	cfg := DefaultConfig()
	want := "127.0.0.1:9998"
	if cfg.Monitor.HealthAddr != want {
		t.Errorf("Monitor.HealthAddr = %q, want %q", cfg.Monitor.HealthAddr, want)
	}
}

// TestDefaultConfigDiskLowThreshold verifies the disk low threshold default.
func TestDefaultConfigDiskLowThreshold(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Monitor.DiskLowThresholdMB <= 0 {
		t.Errorf("Monitor.DiskLowThresholdMB = %d, want positive default", cfg.Monitor.DiskLowThresholdMB)
	}
}
