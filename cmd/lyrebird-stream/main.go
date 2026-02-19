// SPDX-License-Identifier: MIT

// Package main implements the lyrebird-stream daemon, the core audio streaming service.
//
// lyrebird-stream is designed for 24/7 unattended operation, managing multiple
// audio streams with automatic failure recovery and graceful shutdown.
//
// Usage:
//
//	lyrebird-stream [options]
//
// Options:
//
//	--config=PATH     Path to config file (default: /etc/lyrebird/config.yaml)
//	--lock-dir=PATH   Directory for lock files (default: /var/run/lyrebird)
//	--log-level=LEVEL Log level: debug, info, warn, error (default: info)
//	--help            Show this help message
//
// Example:
//
//	# Run with default config
//	lyrebird-stream
//
//	# Run with custom config
//	lyrebird-stream --config=/path/to/config.yaml
//
// The daemon automatically:
//   - Detects USB audio devices
//   - Starts FFmpeg streams for each device
//   - Restarts failed streams with exponential backoff
//   - Handles SIGINT/SIGTERM for graceful shutdown
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/health"
	"github.com/tomtom215/lyrebirdaudio-go/internal/stream"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// Build information (set by ldflags)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Command line flags
var (
	configPath = flag.String("config", config.ConfigFilePath, "Path to configuration file")
	lockDir    = flag.String("lock-dir", "/var/run/lyrebird", "Directory for lock files")
	logLevel   = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	showHelp   = flag.Bool("help", false, "Show help message")
)

func main() {
	flag.Parse()

	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	// Initialize structured logger
	slogLevel := parseSlogLevel(*logLevel)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel}))
	slog.SetDefault(logger)
	logger.Info("starting lyrebird-stream", "version", Version, "commit", Commit, "built", BuildTime)

	// Create lock directory if it doesn't exist
	if err := os.MkdirAll(*lockDir, 0750); err != nil { //nolint:gosec // Lock directory needs group read for service monitoring
		logger.Error("failed to create lock directory", "error", err)
		os.Exit(1)
	}

	// Load configuration using koanf (supports env vars and hot-reload)
	koanfCfg, cfg, err := loadConfigurationKoanf(*configPath)
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}
	logger.Info("loaded configuration", "path", *configPath)

	// Create supervisor
	var supLogger *slog.Logger
	if slogLevel <= slog.LevelDebug {
		supLogger = logger.With("component", "supervisor")
	}

	sup := supervisor.New(supervisor.Config{
		ShutdownTimeout: 30 * time.Second,
		Logger:          supLogger,
	})

	// Find ffmpeg path
	ffmpegPath, err := findFFmpegPath()
	if err != nil {
		logger.Error("ffmpeg not found", "error", err)
		os.Exit(1)
	}
	logger.Info("using ffmpeg", "path", ffmpegPath)

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())

	shutdownCh := make(chan os.Signal, 1)
	reloadCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(reloadCh, syscall.SIGHUP)

	// Handle shutdown signals
	go func() {
		sig := <-shutdownCh
		logger.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	// registeredServices tracks which device names have been registered with the
	// supervisor.  It is accessed from the main goroutine, the poll goroutine,
	// and the SIGHUP reload goroutine — a plain map would be a data race (C-2,
	// L-14 fix).
	var (
		registeredMu       sync.RWMutex
		registeredServices = make(map[string]bool)
	)

	// registerDevices detects USB audio devices and registers new ones with the supervisor.
	// Returns the number of newly registered devices.
	registerDevices := func(cfg *config.Config) int {
		devices, err := audio.DetectDevices("/proc/asound")
		if err != nil {
			logger.Warn("failed to detect audio devices", "error", err)
			return 0
		}

		registered := 0
		for _, dev := range devices {
			devName := audio.SanitizeDeviceName(dev.Name)

			// Skip already-registered devices (read lock).
			registeredMu.RLock()
			alreadyRegistered := registeredServices[devName]
			registeredMu.RUnlock()
			if alreadyRegistered {
				continue
			}

			devCfg := cfg.GetDeviceConfig(devName)
			streamName := devName
			rtspURL := fmt.Sprintf("%s/%s", cfg.MediaMTX.RTSPURL, streamName)
			alsaDevice := fmt.Sprintf("hw:%d,0", dev.CardNumber)

			mgrCfg := &stream.ManagerConfig{
				DeviceName:  devName,
				ALSADevice:  alsaDevice,
				StreamName:  streamName,
				SampleRate:  devCfg.SampleRate,
				Channels:    devCfg.Channels,
				Bitrate:     devCfg.Bitrate,
				Codec:       devCfg.Codec,
				ThreadQueue: devCfg.ThreadQueue,
				RTSPURL:     rtspURL,
				LockDir:     *lockDir,
				FFmpegPath:  ffmpegPath,
				Backoff: stream.NewBackoff(
					cfg.Stream.InitialRestartDelay,
					cfg.Stream.MaxRestartDelay,
					cfg.Stream.MaxRestartAttempts,
				),
				Logger: logger.With("component", "manager", "device", devName),
			}

			mgr, err := stream.NewManager(mgrCfg)
			if err != nil {
				logger.Warn("failed to create manager", "device", devName, "error", err)
				continue
			}

			svc := &streamService{
				name:    devName,
				manager: mgr,
				logger:  logger,
			}

			if err := sup.Add(svc); err != nil {
				logger.Warn("failed to add service", "device", devName, "error", err)
				continue
			}

			registeredMu.Lock()
			registeredServices[devName] = true
			registeredMu.Unlock()
			registered++
			logger.Info("registered stream", "alsa_device", alsaDevice, "rtsp_url", rtspURL)
		}

		return registered
	}

	// Initial device registration
	registerDevices(cfg)

	if sup.ServiceCount() == 0 {
		logger.Info("no USB audio devices found, waiting for devices")
	} else {
		logger.Info("detected USB audio devices", "count", sup.ServiceCount())
	}

	// Device polling loop: periodically scan for newly plugged-in USB devices
	// and register any that are not yet supervised.
	//
	// M-4 fix: the poll runs unconditionally on every tick, not only when the
	// service count is zero.  This is the correct hotplug support: a second
	// microphone plugged in after the daemon started will be registered here.
	go func() {
		const pollInterval = 10 * time.Second
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				var pollCfg *config.Config
				if koanfCfg != nil {
					// C-3 guard: koanfCfg can be nil when NewKoanfConfig failed.
					var loadErr error
					pollCfg, loadErr = koanfCfg.Load()
					if loadErr != nil {
						logger.Warn("failed to load config for device scan", "error", loadErr)
						continue
					}
				} else {
					pollCfg = cfg
				}
				n := registerDevices(pollCfg)
				if n > 0 {
					logger.Info("discovered new devices", "count", n)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle reload signals (SIGHUP) — re-detect devices and register new ones
	go func() {
		for {
			select {
			case <-reloadCh:
				logger.Info("received SIGHUP, reloading configuration")

				// C-3 guard: koanfCfg can be nil when loadConfigurationKoanf
				// fell back to defaults and returned a nil KoanfConfig.
				if koanfCfg == nil {
					logger.Info("no active config file; SIGHUP is a no-op")
					continue
				}

				if err := koanfCfg.Reload(); err != nil {
					logger.Warn("failed to reload configuration", "error", err)
					continue
				}
				logger.Info("configuration reloaded successfully")

				// Re-detect devices and register any new ones
				newCfg, err := koanfCfg.Load()
				if err != nil {
					logger.Warn("failed to load updated config", "error", err)
					continue
				}

				n := registerDevices(newCfg)
				if n > 0 {
					logger.Info("registered new devices on reload", "count", n)
				}

				// Log current configuration for all known devices (read lock).
				registeredMu.RLock()
				names := make([]string, 0, len(registeredServices))
				for name := range registeredServices {
					names = append(names, name)
				}
				registeredMu.RUnlock()

				for _, devName := range names {
					devCfg := newCfg.GetDeviceConfig(devName)
					logger.Info("device config after reload",
						"device", devName,
						"sample_rate", devCfg.SampleRate,
						"channels", devCfg.Channels,
						"codec", devCfg.Codec,
						"bitrate", devCfg.Bitrate)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start health check HTTP server (M-3 fix).
	healthHandler := health.NewHandler(nil) // nil provider: basic liveness only
	go func() {
		if err := health.ListenAndServe(ctx, ":9998", healthHandler); err != nil {
			// Not fatal: health endpoint failure must not crash the daemon.
			logger.Warn("health endpoint error", "error", err)
		}
	}()

	// Run supervisor (blocks until shutdown)
	logger.Info("starting supervisor", "streams", sup.ServiceCount())
	if err := sup.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		// M-1 fix: use errors.Is to handle wrapped context errors.
		logger.Error("supervisor error", "error", err)
	}

	logger.Info("shutdown complete")
}

// streamService wraps a stream.Manager to implement supervisor.Service.
type streamService struct {
	name    string
	manager *stream.Manager
	logger  *slog.Logger
}

func (s *streamService) Name() string {
	return s.name
}

func (s *streamService) Run(ctx context.Context) error {
	s.logger.Info("starting stream", "service", s.name)
	err := s.manager.Run(ctx)
	// M-5 fix: Release the lock file and other resources held by the manager
	// regardless of whether Run returned an error.  Without this call the lock
	// file fd is not closed until the process exits, which leaks the fd and
	// prevents a clean re-acquire after a hot-reload.
	if closeErr := s.manager.Close(); closeErr != nil {
		s.logger.Warn("manager close error", "service", s.name, "error", closeErr)
	}
	// M-1 fix: use errors.Is so that wrapped context errors are handled correctly.
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Error("stream stopped with error", "service", s.name, "error", err)
	} else {
		s.logger.Info("stream stopped", "service", s.name)
	}
	return err
}

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

func printUsage() {
	fmt.Println("lyrebird-stream - Audio streaming daemon")
	fmt.Printf("Version: %s (%s)\n\n", Version, Commit)
	fmt.Println("Usage: lyrebird-stream [options]")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("The daemon streams audio from USB devices to RTSP via MediaMTX.")
	fmt.Println("It automatically detects devices and restarts failed streams.")
	fmt.Println()
	fmt.Println("Signals:")
	fmt.Println("  SIGINT, SIGTERM  Graceful shutdown")
	fmt.Println("  SIGHUP           Reload configuration and log new settings")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  LYREBIRD_DEFAULT_SAMPLE_RATE     Override default sample rate")
	fmt.Println("  LYREBIRD_DEFAULT_CODEC           Override default codec (opus/aac)")
	fmt.Println("  LYREBIRD_DEFAULT_BITRATE         Override default bitrate (e.g., 128k)")
	fmt.Println("  LYREBIRD_DEVICES_<NAME>_<FIELD>  Override device-specific settings")
	fmt.Println("  See documentation for full list of environment variables")
}
