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

	// Load configuration
	cfg, err := loadConfiguration(*configPath)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}
	logger.Printf("Loaded configuration from %s", *configPath)

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
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-sigCh
		logger.Printf("Received signal %v, initiating shutdown...", sig)
		cancel()
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
	fmt.Println("  SIGHUP           Reload configuration (planned)")
}
