package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MigrateFromBash migrates configuration from bash environment variables to YAML.
//
// The bash version used environment variables in this format:
//
//	SAMPLE_RATE_device_name=48000
//	CHANNELS_device_name=2
//	BITRATE_device_name=192k
//	CODEC_device_name=opus
//	THREAD_QUEUE_SIZE_device_name=8192
//
//	DEFAULT_SAMPLE_RATE=48000
//	DEFAULT_CHANNELS=2
//	DEFAULT_BITRATE=128k
//	DEFAULT_CODEC=opus
//	DEFAULT_THREAD_QUEUE_SIZE=8192
//
// This function parses those variables and creates a Config struct
// that can be saved as YAML.
//
// Parameters:
//   - bashConfigPath: Path to bash config file with environment variables
//
// Returns:
//   - *Config: Migrated configuration
//   - error: if file cannot be read or parsed
//
// Example:
//
//	cfg, err := MigrateFromBash("/etc/mediamtx/audio-devices.conf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	cfg.Save("/etc/lyrebird/config.yaml")
func MigrateFromBash(bashConfigPath string) (*Config, error) {
	// Start with default config
	cfg := DefaultConfig()

	// Open bash config file
	file, err := os.Open(bashConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bash config: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Track which devices we've seen
	devices := make(map[string]*DeviceConfig)

	// Parse line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		varName, deviceName, value, ok := parseBashEnvLine(line)
		if !ok {
			continue // Skip comments, empty lines, etc.
		}

		// Handle default variables (no device suffix)
		if strings.HasPrefix(varName, "DEFAULT_") {
			if err := applyDefaultValue(&cfg.Default, varName, value); err != nil {
				return nil, fmt.Errorf("invalid default value for %s: %w", varName, err)
			}
			continue
		}

		// Handle device-specific variables
		if deviceName == "" {
			continue // Skip variables without device suffix
		}

		// Get or create device config
		if _, exists := devices[deviceName]; !exists {
			devices[deviceName] = &DeviceConfig{}
		}

		// Apply value to device config
		if err := applyDeviceValue(devices[deviceName], varName, value); err != nil {
			return nil, fmt.Errorf("invalid value for %s_%s: %w", varName, deviceName, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading bash config: %w", err)
	}

	// Convert map to config.Devices
	cfg.Devices = make(map[string]DeviceConfig, len(devices))
	for name, devCfg := range devices {
		cfg.Devices[name] = *devCfg
	}

	return cfg, nil
}

// parseBashEnvLine parses a bash environment variable assignment.
//
// Returns:
//   - varName: Variable name (e.g., "SAMPLE_RATE", "DEFAULT_CODEC")
//   - deviceName: Device name suffix (e.g., "blue_yeti", "" for defaults)
//   - value: Variable value (unquoted)
//   - ok: true if line was successfully parsed
//
// Example:
//
//	varName, device, value, ok := parseBashEnvLine("SAMPLE_RATE_blue_yeti=48000")
//	// varName = "SAMPLE_RATE", device = "blue_yeti", value = "48000", ok = true
//
//	varName, device, value, ok := parseBashEnvLine("DEFAULT_CODEC=opus")
//	// varName = "DEFAULT_CODEC", device = "", value = "opus", ok = true
func parseBashEnvLine(line string) (varName, deviceName, value string, ok bool) {
	// Trim whitespace
	line = strings.TrimSpace(line)

	// Skip empty lines and comments
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", "", false
	}

	// Remove "export " prefix if present
	line = strings.TrimPrefix(line, "export ")
	line = strings.TrimSpace(line)

	// Split on first '='
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", "", false
	}

	fullVarName := strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])

	// Remove quotes from value
	value = strings.Trim(value, `"'`)

	// Check for default variables (no device suffix)
	if strings.HasPrefix(fullVarName, "DEFAULT_") {
		return fullVarName, "", value, true
	}

	// Parse device-specific variables: VAR_NAME_device_name
	// Known variable prefixes
	knownVars := []string{
		"SAMPLE_RATE_",
		"CHANNELS_",
		"BITRATE_",
		"CODEC_",
		"THREAD_QUEUE_SIZE_",
	}

	// Check each known variable prefix
	for _, prefix := range knownVars {
		if strings.HasPrefix(fullVarName, prefix) {
			varName = strings.TrimSuffix(prefix, "_")
			deviceName = strings.TrimPrefix(fullVarName, prefix)
			return varName, deviceName, value, true
		}
	}

	// Unknown variable format
	return "", "", "", false
}

// applyDefaultValue applies a default configuration value.
func applyDefaultValue(cfg *DeviceConfig, varName, value string) error {
	switch varName {
	case "DEFAULT_SAMPLE_RATE":
		rate, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid sample rate: %w", err)
		}
		cfg.SampleRate = rate

	case "DEFAULT_CHANNELS":
		channels, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid channels: %w", err)
		}
		cfg.Channels = channels

	case "DEFAULT_BITRATE":
		cfg.Bitrate = value

	case "DEFAULT_CODEC":
		cfg.Codec = value

	case "DEFAULT_THREAD_QUEUE_SIZE":
		queue, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid thread queue size: %w", err)
		}
		cfg.ThreadQueue = queue
	}

	return nil
}

// applyDeviceValue applies a device-specific configuration value.
func applyDeviceValue(cfg *DeviceConfig, varName, value string) error {
	switch varName {
	case "SAMPLE_RATE":
		rate, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid sample rate: %w", err)
		}
		cfg.SampleRate = rate

	case "CHANNELS":
		channels, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid channels: %w", err)
		}
		cfg.Channels = channels

	case "BITRATE":
		cfg.Bitrate = value

	case "CODEC":
		cfg.Codec = value

	case "THREAD_QUEUE_SIZE":
		queue, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid thread queue size: %w", err)
		}
		cfg.ThreadQueue = queue
	}

	return nil
}
