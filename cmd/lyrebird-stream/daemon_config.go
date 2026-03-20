// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

// loadConfigurationKoanf loads configuration using koanf with support for:
//   - YAML configuration file
//   - Environment variable overrides (LYREBIRD_*)
//   - Hot-reload via SIGHUP
//
// Returns both the KoanfConfig (for reload) and the loaded Config.
func loadConfigurationKoanf(path string) (*config.KoanfConfig, *config.Config, error) {
	// Check if file exists
	fileExists := true
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fileExists = false
	}

	var kc *config.KoanfConfig
	var err error

	if fileExists {
		// Load with file + env vars
		kc, err = config.NewKoanfConfig(
			config.WithYAMLFile(path),
			config.WithEnvPrefix("LYREBIRD"),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create koanf config: %w", err)
		}
	} else {
		// No config file — load with env vars only
		kc, err = config.NewKoanfConfig(
			config.WithEnvPrefix("LYREBIRD"),
		)
		if err != nil {
			// If no file and env vars fail, return default config
			return nil, config.DefaultConfig(), nil
		}
	}

	// Load the configuration
	cfg, err := kc.Load()
	if err != nil {
		if !fileExists {
			// No file and env vars insufficient — use defaults
			return kc, config.DefaultConfig(), nil
		}
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	return kc, cfg, nil
}

// findFFmpegPath locates the ffmpeg binary using exec.LookPath,
// which respects PATH and verifies the file is executable.
func findFFmpegPath() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}
	return path, nil
}

// parseSlogLevel converts a log level string to slog.Level.
func parseSlogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// deviceConfigHash returns a stable string key representing the streaming
// parameters that are passed to FFmpeg for a device.  When the hash changes
// between reloads the stream must be restarted with the new parameters (M-6).
//
// The hash includes every field that changes the FFmpeg command line so that
// parameter changes are never silently ignored.
//
// M-2 fix: Now accepts the full stream config to include LocalRecordDir,
// SegmentDuration, SegmentFormat, and StopTimeout in the hash, ensuring that
// changes to these fields trigger a stream restart on SIGHUP.
func deviceConfigHash(devCfg config.DeviceConfig, rtspURL string, streamCfg config.StreamConfig) string {
	return fmt.Sprintf("%d/%d/%s/%s/%d/%s/%s/%d/%s/%v",
		devCfg.SampleRate,
		devCfg.Channels,
		devCfg.Bitrate,
		devCfg.Codec,
		devCfg.ThreadQueue,
		rtspURL,
		streamCfg.LocalRecordDir,
		streamCfg.SegmentDuration,
		streamCfg.SegmentFormat,
		streamCfg.StopTimeout,
	)
}
