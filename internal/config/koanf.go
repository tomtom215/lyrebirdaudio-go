// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// KoanfConfig wraps koanf for enhanced configuration management.
//
// It provides:
//   - Multiple configuration sources (YAML file + environment variables)
//   - Configuration hot-reload via file watching
//   - Override precedence (env vars override YAML)
//   - Backward compatibility with existing LoadConfig() API
type KoanfConfig struct {
	k         *koanf.Koanf
	mu        sync.RWMutex
	filePath  string
	envPrefix string
}

// Option configures a KoanfConfig.
type Option func(*KoanfConfig) error

// WithYAMLFile sets the YAML configuration file path.
func WithYAMLFile(path string) Option {
	return func(kc *KoanfConfig) error {
		kc.filePath = path
		return nil
	}
}

// WithEnvPrefix sets the environment variable prefix (default: "LYREBIRD").
func WithEnvPrefix(prefix string) Option {
	return func(kc *KoanfConfig) error {
		kc.envPrefix = prefix
		return nil
	}
}

// NewKoanfConfig creates a new koanf-based configuration loader.
//
// It loads configuration from multiple sources with the following precedence (highest to lowest):
//  1. Environment variables (LYREBIRD_*)
//  2. YAML configuration file
//  3. Built-in defaults
//
// Parameters:
//   - opts: Configuration options (WithYAMLFile, WithEnvPrefix, etc.)
//
// Returns:
//   - *KoanfConfig: Configured loader
//   - error: if configuration cannot be loaded or validated
//
// Example:
//
//	kc, err := NewKoanfConfig(
//	    WithYAMLFile("/etc/lyrebird/config.yaml"),
//	    WithEnvPrefix("LYREBIRD"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	cfg, err := kc.Load()
func NewKoanfConfig(opts ...Option) (*KoanfConfig, error) {
	kc := &KoanfConfig{
		k:         koanf.New("."),
		envPrefix: "LYREBIRD",
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(kc); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Load initial configuration
	if err := kc.reload(); err != nil {
		return nil, err
	}

	return kc, nil
}

// Load unmarshals the configuration into a Config struct.
//
// Returns:
//   - *Config: Unmarshaled configuration
//   - error: if unmarshaling or validation fails
func (kc *KoanfConfig) Load() (*Config, error) {
	var cfg Config

	kc.mu.RLock()
	k := kc.k
	kc.mu.RUnlock()

	// Unmarshal into struct
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Reload reloads configuration from all sources.
//
// This is called internally during Watch() when file changes are detected,
// and can also be called manually to force a reload.
//
// Returns:
//   - error: if reload fails
func (kc *KoanfConfig) Reload() error {
	return kc.reload()
}

// reload is the internal reload implementation.
func (kc *KoanfConfig) reload() error {
	// Create a new koanf instance for atomic reload
	newK := koanf.New(".")

	// Load YAML file (if specified)
	if kc.filePath != "" {
		if err := newK.Load(file.Provider(kc.filePath), yaml.Parser()); err != nil {
			return fmt.Errorf("failed to load YAML file: %w", err)
		}
	}

	// Load environment variables (override YAML).
	// Strategy: transform LYREBIRD_DEVICES_BLUE_YETI_SAMPLE_RATE to
	// devices.blue_yeti.sample_rate by recognising the known top-level key
	// prefixes and stripping the suffix for known field names.
	// The env.Provider Prefix option already strips LYREBIRD_ before the
	// TransformFunc runs, so the function receives the remainder only.
	envProvider := env.Provider(".", env.Opt{
		Prefix: kc.envPrefix + "_",
		TransformFunc: func(k, v string) (string, any) {
			// k arrives WITHOUT the LYREBIRD_ prefix (stripped by env.Provider).
			// Convert to lowercase for case-insensitive matching.
			k = strings.TrimPrefix(k, kc.envPrefix+"_")
			// Convert to lowercase
			k = strings.ToLower(k)

			// Known top-level keys that should be separated
			// DEVICES_XXX -> devices.XXX
			// DEFAULT_XXX -> default.XXX
			// STREAM_XXX -> stream.XXX
			// MEDIAMTX_XXX -> mediamtx.XXX
			// MONITOR_XXX -> monitor.XXX
			topLevelKeys := []string{"devices_", "default_", "stream_", "mediamtx_", "monitor_"}

			for _, prefix := range topLevelKeys {
				if strings.HasPrefix(k, prefix) {
					// Split: "devices_blue_yeti_sample_rate" -> "devices" + "blue_yeti_sample_rate"
					rest := strings.TrimPrefix(k, prefix)
					topLevel := strings.TrimSuffix(prefix, "_")

					// For "devices", we need one more level (device name)
					if topLevel == "devices" {
						// "blue_yeti_sample_rate" -> "blue_yeti" + "sample_rate"
						// Find the last known field name
						knownFields := []string{"_sample_rate", "_channels", "_bitrate", "_codec", "_thread_queue"}
						for _, field := range knownFields {
							if strings.HasSuffix(rest, field) {
								deviceName := strings.TrimSuffix(rest, field)
								fieldName := strings.TrimPrefix(field, "_")
								return topLevel + "." + deviceName + "." + fieldName, v
							}
						}
						// If no known field, treat entire rest as device name
						return topLevel + "." + rest, v
					}

					// For other top-levels, just append the rest
					return topLevel + "." + rest, v
				}
			}

			// No known prefix, return as-is with underscores replaced by dots
			return strings.ReplaceAll(k, "_", "."), v
		},
	})

	if err := newK.Load(envProvider, nil); err != nil {
		return fmt.Errorf("failed to load environment variables: %w", err)
	}

	// Atomic swap (protected by write lock)
	kc.mu.Lock()
	kc.k = newK
	kc.mu.Unlock()

	return nil
}

// Watch starts watching the configuration file for changes.
//
// When changes are detected, the callback function is called with the event type
// and any error that occurred. The configuration is automatically reloaded before
// the callback is invoked on success.
//
// This enables hot-reload via file system events (fsnotify).
//
// M-9 known limitation: the underlying koanf file.Provider spawns an fsnotify
// goroutine internally.  koanf v2 does not expose a Stop() method on file.Provider,
// so that goroutine cannot be stopped when ctx is cancelled.  The goroutine will
// be collected when the process exits.  For long-lived applications that need
// clean goroutine shutdown, prefer triggering manual Reload() calls on SIGHUP
// instead of calling Watch().
//
// Parameters:
//   - ctx: Context for cancellation (stops the Watch blocking wait, but not the
//     underlying fsnotify goroutine — see M-9 note above).
//   - callback: Function called when configuration changes. Receives event description
//     and error (nil on success, non-nil on watch/reload errors).
//
// Returns:
//   - error: if watching cannot be started
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	err := kc.Watch(ctx, func(event string, err error) {
//	    if err != nil {
//	        log.Printf("Config watch error: %v", err)
//	        return
//	    }
//	    log.Printf("Config reloaded: %s", event)
//	    // Restart affected services...
//	})
func (kc *KoanfConfig) Watch(ctx context.Context, callback func(event string, err error)) error {
	if kc.filePath == "" {
		return fmt.Errorf("cannot watch: no file path specified")
	}

	// Create file provider with watch capability
	fp := file.Provider(kc.filePath)

	// Start watching
	watchErr := fp.Watch(func(event interface{}, err error) {
		if err != nil {
			// Propagate error to callback
			callback("watch error", fmt.Errorf("file watch error: %w", err))
			return
		}

		// Reload configuration
		if err := kc.reload(); err != nil {
			callback("reload error", fmt.Errorf("config reload failed: %w", err))
			return
		}

		callback("config reloaded", nil)
	})

	if watchErr != nil {
		return fmt.Errorf("failed to start watching: %w", watchErr)
	}

	// Wait for context cancellation
	<-ctx.Done()

	return nil
}

// GetString retrieves a string value from configuration.
func (kc *KoanfConfig) GetString(key string) string {
	kc.mu.RLock()
	k := kc.k
	kc.mu.RUnlock()
	return k.String(key)
}

// GetInt retrieves an integer value from configuration.
func (kc *KoanfConfig) GetInt(key string) int {
	kc.mu.RLock()
	k := kc.k
	kc.mu.RUnlock()
	return k.Int(key)
}

// GetBool retrieves a boolean value from configuration.
func (kc *KoanfConfig) GetBool(key string) bool {
	kc.mu.RLock()
	k := kc.k
	kc.mu.RUnlock()
	return k.Bool(key)
}

// GetDuration retrieves a duration value from configuration.
func (kc *KoanfConfig) GetDuration(key string) time.Duration {
	kc.mu.RLock()
	k := kc.k
	kc.mu.RUnlock()
	return k.Duration(key)
}

// Exists checks if a configuration key exists.
func (kc *KoanfConfig) Exists(key string) bool {
	kc.mu.RLock()
	k := kc.k
	kc.mu.RUnlock()
	return k.Exists(key)
}

// All returns the entire configuration as a map.
func (kc *KoanfConfig) All() map[string]interface{} {
	kc.mu.RLock()
	k := kc.k
	kc.mu.RUnlock()
	return k.All()
}
