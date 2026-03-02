// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

// ConfigFilePath is the default location for the configuration file.
const ConfigFilePath = "/etc/lyrebird/config.yaml"

// Config represents the complete LyreBird configuration.
type Config struct {
	// Devices contains device-specific configuration keyed by sanitized device name.
	Devices map[string]DeviceConfig `yaml:"devices" koanf:"devices"`

	// Default configuration used when device-specific config not found.
	Default DeviceConfig `yaml:"default" koanf:"default"`

	// Stream manager settings.
	Stream StreamConfig `yaml:"stream" koanf:"stream"`

	// MediaMTX integration settings.
	MediaMTX MediaMTXConfig `yaml:"mediamtx" koanf:"mediamtx"`

	// Monitor settings for health checks.
	Monitor MonitorConfig `yaml:"monitor" koanf:"monitor"`
}

// DeviceConfig contains FFmpeg encoding parameters for a device.
type DeviceConfig struct {
	SampleRate  int    `yaml:"sample_rate" koanf:"sample_rate"`   // Sample rate in Hz (e.g., 48000)
	Channels    int    `yaml:"channels" koanf:"channels"`         // Number of audio channels (1=mono, 2=stereo)
	Bitrate     string `yaml:"bitrate" koanf:"bitrate"`           // Bitrate (e.g., "128k", "192k")
	Codec       string `yaml:"codec" koanf:"codec"`               // Audio codec ("opus" or "aac")
	ThreadQueue int    `yaml:"thread_queue" koanf:"thread_queue"` // FFmpeg thread queue size
}

// StreamConfig contains stream lifecycle management settings.
type StreamConfig struct {
	InitialRestartDelay   time.Duration `yaml:"initial_restart_delay" koanf:"initial_restart_delay"`     // First restart delay
	MaxRestartDelay       time.Duration `yaml:"max_restart_delay" koanf:"max_restart_delay"`             // Maximum backoff delay
	MaxRestartAttempts    int           `yaml:"max_restart_attempts" koanf:"max_restart_attempts"`       // Max attempts before giving up
	USBStabilizationDelay time.Duration `yaml:"usb_stabilization_delay" koanf:"usb_stabilization_delay"` // Wait after USB changes
	StopTimeout           time.Duration `yaml:"stop_timeout" koanf:"stop_timeout"`                       // H-1 fix: graceful stop timeout (default: 5s)
	LocalRecordDir        string        `yaml:"local_record_dir" koanf:"local_record_dir"`               // C-1 fix: local recording directory (empty = disabled)
	SegmentDuration       int           `yaml:"segment_duration" koanf:"segment_duration"`               // C-1 fix: segment duration in seconds (default: 3600)
	SegmentFormat         string        `yaml:"segment_format" koanf:"segment_format"`                   // C-1 fix: segment format: wav, flac, ogg (default: wav)
	SegmentMaxAge         time.Duration `yaml:"segment_max_age" koanf:"segment_max_age"`                 // GAP-1c: max age of recording segments before deletion (0 = no limit)
	SegmentMaxTotalBytes  int64         `yaml:"segment_max_total_bytes" koanf:"segment_max_total_bytes"` // GAP-1c: max total bytes in LocalRecordDir before oldest deletion (0 = no limit)
}

// MediaMTXConfig contains MediaMTX server integration settings.
type MediaMTXConfig struct {
	APIURL     string `yaml:"api_url" koanf:"api_url"`         // API endpoint (e.g., "http://localhost:9997")
	RTSPURL    string `yaml:"rtsp_url" koanf:"rtsp_url"`       // RTSP server URL (e.g., "rtsp://localhost:8554")
	ConfigPath string `yaml:"config_path" koanf:"config_path"` // Path to mediamtx.yml
}

// MonitorConfig contains health monitoring settings.
type MonitorConfig struct {
	Enabled            bool          `yaml:"enabled" koanf:"enabled"`                             // Enable health monitoring
	Interval           time.Duration `yaml:"interval" koanf:"interval"`                           // Health check / recovery interval
	StallCheckInterval time.Duration `yaml:"stall_check_interval" koanf:"stall_check_interval"`   // H-2 fix: separate stall-check interval (default: 60s)
	MaxStallChecks     int           `yaml:"max_stall_checks" koanf:"max_stall_checks"`           // H-2 fix: consecutive stall checks before restart (default: 3)
	RestartUnhealthy   bool          `yaml:"restart_unhealthy" koanf:"restart_unhealthy"`         // Auto-restart failed streams
	HealthAddr         string        `yaml:"health_addr" koanf:"health_addr"`                     // GAP-8: health endpoint address (default: "127.0.0.1:9998")
	DiskLowThresholdMB int64         `yaml:"disk_low_threshold_mb" koanf:"disk_low_threshold_mb"` // GAP-1d: warn when free disk < this value in MB (0 = disabled)
}

// LoadConfig reads and parses the configuration file.
//
// Parameters:
//   - path: Path to YAML configuration file
//
// Returns:
//   - *Config: Parsed configuration
//   - error: if file not found, invalid YAML, or validation fails
//
// Example:
//
//	cfg, err := LoadConfig("/etc/lyrebird/config.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	devCfg := cfg.GetDeviceConfig("blue_yeti")
func LoadConfig(path string) (*Config, error) {
	// Read file
	// #nosec G304 - Config path is from administrator-controlled configuration
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Save writes the configuration to a YAML file.
//
// Parameters:
//   - path: Destination file path
//
// Returns:
//   - error: if marshaling fails or file write fails
//
// Example:
//
//	cfg := DefaultConfig()
//	err := cfg.Save("/etc/lyrebird/config.yaml")
//
// atomicFile abstracts file operations used by Save for testability.
type atomicFile interface {
	Write([]byte) (int, error)
	Sync() error
	Chmod(os.FileMode) error
	Close() error
	Name() string
}

// atomicCreateTemp is the injectable temp-file creator used by Save.
// Tests can replace this with a function returning a mock atomicFile.
type atomicCreateTemp func(dir, pattern string) (atomicFile, error)

func defaultCreateTemp(dir, pattern string) (atomicFile, error) {
	return os.CreateTemp(dir, pattern) // #nosec G304
}

func (c *Config) Save(path string) error {
	return c.saveWith(path, defaultCreateTemp)
}

func (c *Config) saveWith(path string, createTemp atomicCreateTemp) error {
	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Atomic write: write to a temp file in the same directory, sync to disk,
	// then rename to the target path. os.Rename is atomic on most filesystems,
	// so a crash mid-write leaves either the old file or the new file, never
	// a partially-written file.
	dir := filepath.Dir(path)

	tmpFile, err := createTemp(dir, ".config.*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on any error
	success := false
	defer func() {
		if !success {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// Write data to temp file
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	// Sync to disk to ensure data is persisted before rename
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp config file: %w", err)
	}

	// SEC-3: Restrict config file to owner+group only (least privilege).
	// Config files may contain sensitive settings (API URLs, server details)
	// and should not be world-readable.
	// #nosec G302 - Config file restricted to owner+group for security
	if err := tmpFile.Chmod(0640); err != nil {
		return fmt.Errorf("failed to set config file permissions: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil { // #nosec G703 -- path is from CLI flag/config, not web request input
		return fmt.Errorf("failed to rename temp config file: %w", err)
	}

	success = true
	return nil
}

// GetDeviceConfig returns configuration for a device, falling back to defaults.
//
// This is the primary config lookup method used by the stream manager.
// It performs a two-stage lookup:
//  1. Check for device-specific configuration
//  2. Fall back to default configuration
//  3. Merge defaults for any unset fields
//
// Parameters:
//   - deviceName: Sanitized device name (e.g., "blue_yeti")
//
// Returns:
//   - DeviceConfig: Device-specific config merged with defaults
//
// Example:
//
//	cfg, _ := LoadConfig("/etc/lyrebird/config.yaml")
//	devCfg := cfg.GetDeviceConfig("blue_yeti")
//	fmt.Printf("Sample rate: %d\n", devCfg.SampleRate)
func (c *Config) GetDeviceConfig(deviceName string) DeviceConfig {
	// Start with default config
	result := c.Default

	// Look up device-specific config
	if devCfg, ok := c.Devices[deviceName]; ok {
		// Override defaults with device-specific values (if set)
		if devCfg.SampleRate != 0 {
			result.SampleRate = devCfg.SampleRate
		}
		if devCfg.Channels != 0 {
			result.Channels = devCfg.Channels
		}
		if devCfg.Bitrate != "" {
			result.Bitrate = devCfg.Bitrate
		}
		if devCfg.Codec != "" {
			result.Codec = devCfg.Codec
		}
		if devCfg.ThreadQueue != 0 {
			result.ThreadQueue = devCfg.ThreadQueue
		}
	}

	return result
}

// Validate checks configuration for invalid values.
//
// Returns:
//   - error: describing the first validation error found, or nil if valid
//
// Validation rules:
//   - sample_rate must be positive
//   - channels must be between 1 and 32
//   - bitrate cannot be empty
//   - codec must be "opus" or "aac"
//   - segment_format must be "wav", "flac", or "ogg" (if set)
func (c *Config) Validate() error {
	// Validate default config
	if err := c.Default.Validate(); err != nil {
		return fmt.Errorf("default config: %w", err)
	}

	// Validate each device config
	for name, devCfg := range c.Devices {
		if err := devCfg.ValidatePartial(); err != nil {
			return fmt.Errorf("device %q: %w", name, err)
		}
	}

	// Validate stream config (GAP-1b)
	if err := c.Stream.Validate(); err != nil {
		return fmt.Errorf("stream config: %w", err)
	}

	return nil
}

// Validate checks stream configuration for invalid values.
//
// GAP-1b: SegmentFormat must be one of "wav", "flac", "ogg" when LocalRecordDir is set.
// An invalid format causes silent FFmpeg failure on every stream start.
func (s *StreamConfig) Validate() error {
	if s.SegmentFormat != "" {
		switch s.SegmentFormat {
		case "wav", "flac", "ogg":
			// valid
		default:
			return fmt.Errorf("segment_format must be one of wav, flac, ogg (got %q)", s.SegmentFormat)
		}
	}
	if s.SegmentMaxTotalBytes < 0 {
		return fmt.Errorf("segment_max_total_bytes must not be negative")
	}
	return nil
}

// Validate checks device configuration for invalid values.
//
// This is used for validating the default configuration which must be complete.
func (d *DeviceConfig) Validate() error {
	if d.SampleRate <= 0 {
		return fmt.Errorf("sample_rate must be positive")
	}
	if d.Channels <= 0 {
		return fmt.Errorf("channels must be positive")
	}
	if d.Channels > 32 {
		return fmt.Errorf("channels must be between 1 and 32")
	}
	if d.Bitrate == "" {
		return fmt.Errorf("bitrate cannot be empty")
	}
	if d.Codec == "" {
		return fmt.Errorf("codec cannot be empty")
	}
	if d.Codec != "opus" && d.Codec != "aac" {
		return fmt.Errorf("codec must be opus or aac")
	}
	return nil
}

// ValidatePartial checks device configuration for invalid values.
//
// This allows device-specific configs to omit fields (they'll inherit from default).
// Only validates fields that are explicitly set (non-zero/non-empty).
func (d *DeviceConfig) ValidatePartial() error {
	if d.SampleRate < 0 {
		return fmt.Errorf("sample_rate must not be negative (0 means inherit default)")
	}
	if d.Channels < 0 {
		return fmt.Errorf("channels must not be negative (0 means inherit default)")
	}
	if d.Channels > 32 {
		return fmt.Errorf("channels must be between 1 and 32")
	}
	if d.Codec != "" && d.Codec != "opus" && d.Codec != "aac" {
		return fmt.Errorf("codec must be opus or aac")
	}
	return nil
}

// DefaultConfig returns a configuration with sensible defaults.
//
// This is used when no config file exists or for testing.
//
// Returns:
//   - *Config: Configuration with production-tested defaults
//
// Example:
//
//	cfg := DefaultConfig()
//	cfg.Devices["my_device"] = DeviceConfig{
//	    SampleRate: 44100,
//	    Channels: 1,
//	}
//	cfg.Save("/etc/lyrebird/config.yaml")
func DefaultConfig() *Config {
	return &Config{
		Devices: make(map[string]DeviceConfig),
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
			StopTimeout:           5 * time.Second,    // H-1 fix: 5s default (was hardcoded 2s)
			SegmentDuration:       3600,               // C-1: 1-hour segments by default
			SegmentFormat:         "wav",              // C-1: lossless WAV by default
			SegmentMaxAge:         7 * 24 * time.Hour, // GAP-1c: retain segments for 7 days
			SegmentMaxTotalBytes:  0,                  // GAP-1c: no total-size limit by default
			// LocalRecordDir: empty by default (local recording disabled)
			// IMPORTANT: Set local_record_dir to enable redundant local recording.
			// Without it, a MediaMTX crash at 3 AM will lose audio with no recovery.
			// Example: local_record_dir: /var/lib/lyrebird/recordings
		},
		MediaMTX: MediaMTXConfig{
			APIURL:     "http://localhost:9997",
			RTSPURL:    "rtsp://localhost:8554",
			ConfigPath: "/etc/mediamtx/mediamtx.yml",
		},
		Monitor: MonitorConfig{
			Enabled:            true,
			Interval:           5 * time.Minute,
			StallCheckInterval: 60 * time.Second, // H-2 fix: check for stalls every 60s (was 5 min)
			MaxStallChecks:     3,                // H-2 fix: 3 consecutive checks = 3 min detection
			RestartUnhealthy:   true,
			HealthAddr:         "127.0.0.1:9998", // GAP-8: default health endpoint address
			DiskLowThresholdMB: 1024,             // GAP-1d: warn when free disk < 1 GB
		},
	}
}
