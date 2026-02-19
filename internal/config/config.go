// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
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
}

// MediaMTXConfig contains MediaMTX server integration settings.
type MediaMTXConfig struct {
	APIURL     string `yaml:"api_url" koanf:"api_url"`         // API endpoint (e.g., "http://localhost:9997")
	RTSPURL    string `yaml:"rtsp_url" koanf:"rtsp_url"`       // RTSP server URL (e.g., "rtsp://localhost:8554")
	ConfigPath string `yaml:"config_path" koanf:"config_path"` // Path to mediamtx.yml
}

// MonitorConfig contains health monitoring settings.
type MonitorConfig struct {
	Enabled          bool          `yaml:"enabled" koanf:"enabled"`                     // Enable health monitoring
	Interval         time.Duration `yaml:"interval" koanf:"interval"`                   // Health check interval
	RestartUnhealthy bool          `yaml:"restart_unhealthy" koanf:"restart_unhealthy"` // Auto-restart failed streams
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

	// Set permissions before rename (0644: readable by all, writable by owner)
	// #nosec G302 - Config file should be world-readable (0644 is appropriate)
	if err := tmpFile.Chmod(0644); err != nil {
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
		},
		MediaMTX: MediaMTXConfig{
			APIURL:     "http://localhost:9997",
			RTSPURL:    "rtsp://localhost:8554",
			ConfigPath: "/etc/mediamtx/mediamtx.yml",
		},
		Monitor: MonitorConfig{
			Enabled:          true,
			Interval:         5 * time.Minute,
			RestartUnhealthy: true,
		},
	}
}
