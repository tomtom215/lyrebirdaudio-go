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
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
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

	// Initialize logger
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Printf("lyrebird-stream %s (%s) built %s", Version, Commit, BuildTime)

	// Create lock directory if it doesn't exist
	if err := os.MkdirAll(*lockDir, 0750); err != nil { //nolint:gosec // Lock directory needs group read for service monitoring
		logger.Fatalf("Failed to create lock directory: %v", err)
	}

	// Load configuration using koanf (supports env vars and hot-reload)
	koanfCfg, cfg, err := loadConfigurationKoanf(*configPath)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}
	logger.Printf("Loaded configuration from %s (env var overrides enabled)", *configPath)

	// Detect USB audio devices
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil {
		logger.Fatalf("Failed to detect audio devices: %v", err)
	}

	if len(devices) == 0 {
		logger.Println("No USB audio devices found, waiting for devices...")
	} else {
		logger.Printf("Detected %d USB audio device(s)", len(devices))
	}

	// Create supervisor
	var logWriter io.Writer
	if *logLevel == "debug" {
		logWriter = os.Stderr
	}

	sup := supervisor.New(supervisor.Config{
		ShutdownTimeout: 30 * time.Second,
		Logger:          logWriter,
	})

	// Find ffmpeg path
	ffmpegPath, err := findFFmpegPath()
	if err != nil {
		logger.Fatalf("FFmpeg not found: %v", err)
	}
	logger.Printf("Using FFmpeg: %s", ffmpegPath)

	// Register stream managers for each device
	for _, dev := range devices {
		devName := audio.SanitizeDeviceName(dev.Name)
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
		}

		if *logLevel == "debug" {
			mgrCfg.Logger = os.Stderr
		}

		mgr, err := stream.NewManager(mgrCfg)
		if err != nil {
			logger.Printf("Warning: Failed to create manager for %s: %v", devName, err)
			continue
		}

		// Wrap manager as a supervisor Service
		svc := &streamService{
			name:    devName,
			manager: mgr,
			logger:  logger,
		}

		if err := sup.Add(svc); err != nil {
			logger.Printf("Warning: Failed to add service %s: %v", devName, err)
			continue
		}

		logger.Printf("Registered stream: %s → %s", alsaDevice, rtspURL)
	}

	if sup.ServiceCount() == 0 {
		logger.Println("No streams registered. Exiting.")
		os.Exit(0)
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())

	// Separate channels for shutdown vs reload signals
	shutdownCh := make(chan os.Signal, 1)
	reloadCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(reloadCh, syscall.SIGHUP)

	// Handle shutdown signals
	go func() {
		sig := <-shutdownCh
		logger.Printf("Received signal %v, initiating shutdown...", sig)
		cancel()
	}()

	// Handle reload signals (SIGHUP)
	go func() {
		for {
			select {
			case <-reloadCh:
				logger.Println("Received SIGHUP, reloading configuration...")
				if err := koanfCfg.Reload(); err != nil {
					logger.Printf("Warning: Failed to reload configuration: %v", err)
					continue
				}
				logger.Println("Configuration reloaded successfully")
				// Note: In a full implementation, we would:
				// 1. Compare old vs new config
				// 2. Restart only changed streams
				// 3. Add/remove streams for new/removed devices
				// For now, we just reload the config and log it
			case <-ctx.Done():
				return
			}
		}
	}()

	// Run supervisor (blocks until shutdown)
	logger.Printf("Starting %d stream(s)...", sup.ServiceCount())
	if err := sup.Run(ctx); err != nil && err != context.Canceled {
		logger.Printf("Supervisor error: %v", err)
	}

	logger.Println("Shutdown complete")
}

// streamService wraps a stream.Manager to implement supervisor.Service.
type streamService struct {
	name    string
	manager *stream.Manager
	logger  *log.Logger
}

func (s *streamService) Name() string {
	return s.name
}

func (s *streamService) Run(ctx context.Context) error {
	s.logger.Printf("[%s] Starting stream", s.name)
	err := s.manager.Run(ctx)
	if err != nil && err != context.Canceled {
		s.logger.Printf("[%s] Stream stopped with error: %v", s.name, err)
	} else {
		s.logger.Printf("[%s] Stream stopped", s.name)
	}
	return err
}

// loadConfiguration loads the config file, creating a default if it doesn't exist.
// Deprecated: Use loadConfigurationKoanf for enhanced features (env vars, hot-reload).
func loadConfiguration(path string) (*config.Config, error) {
	// Check if file exists first
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config.DefaultConfig(), nil
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
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
		// Load with env vars only
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
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	return kc, cfg, nil
}

// findFFmpegPath locates the ffmpeg binary.
func findFFmpegPath() (string, error) {
	// Check common locations
	paths := []string{
		"/usr/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
		"/opt/homebrew/bin/ffmpeg",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Try PATH
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "ffmpeg")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("ffmpeg not found in common locations or PATH")
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
	fmt.Println("  SIGHUP           Reload configuration")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  LYREBIRD_DEFAULT_SAMPLE_RATE     Override default sample rate")
	fmt.Println("  LYREBIRD_DEFAULT_CODEC           Override default codec (opus/aac)")
	fmt.Println("  LYREBIRD_DEFAULT_BITRATE         Override default bitrate (e.g., 128k)")
	fmt.Println("  LYREBIRD_DEVICES_<NAME>_<FIELD>  Override device-specific settings")
	fmt.Println("  See documentation for full list of environment variables")
}
