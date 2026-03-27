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
//	--log-dir=PATH    Directory for FFmpeg log files (default: /var/log/lyrebird)
//	--help            Show this help message
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
	"os/signal"
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

// daemonFlags holds parsed command-line flags for the daemon.
type daemonFlags struct {
	ConfigPath string
	LockDir    string
	LogLevel   string
	LogDir     string // Directory for FFmpeg log files (empty = discard)
}

// Command line flags (package-level for flag.Parse wiring only)
var (
	configPath = flag.String("config", config.ConfigFilePath, "Path to configuration file")
	lockDir    = flag.String("lock-dir", "/var/run/lyrebird", "Directory for lock files")
	logLevel   = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	logDir     = flag.String("log-dir", "/var/log/lyrebird", "Directory for FFmpeg log files (empty to discard)")
	showHelp   = flag.Bool("help", false, "Show help message")
)

func main() {
	flag.Parse()

	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	code := runDaemon(daemonFlags{
		ConfigPath: *configPath,
		LockDir:    *lockDir,
		LogLevel:   *logLevel,
		LogDir:     *logDir,
	})
	if code != 0 {
		os.Exit(code)
	}
}

// runDaemon is the core daemon implementation, separated from main() for testability.
// Returns 0 on clean shutdown, 1 on fatal startup errors.
func runDaemon(flags daemonFlags) int {
	// Initialize structured logger
	slogLevel := parseSlogLevel(flags.LogLevel)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel}))
	slog.SetDefault(logger)
	logger.Info("starting lyrebird-stream", "version", Version, "commit", Commit, "built", BuildTime)

	// Create lock directory if it doesn't exist
	if err := os.MkdirAll(flags.LockDir, 0750); err != nil { //nolint:gosec // Lock directory needs group read for service monitoring
		logger.Error("failed to create lock directory", "error", err)
		return 1
	}

	// Create FFmpeg log directory if specified
	if flags.LogDir != "" {
		if err := os.MkdirAll(flags.LogDir, 0750); err != nil { //nolint:gosec // Log directory needs group read for monitoring
			logger.Warn("failed to create log directory, FFmpeg output will be discarded", "error", err)
			flags.LogDir = "" // fall back to no logging
		}
	}

	// Load configuration using koanf (supports env vars and hot-reload)
	koanfCfg, cfg, err := loadConfigurationKoanf(flags.ConfigPath)
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		return 1
	}
	logger.Info("loaded configuration", "path", flags.ConfigPath)

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
		return 1
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

	// Shared device registration state, protected by mutex (C-2, L-14 fix).
	var (
		registeredMu           sync.RWMutex
		registeredServices     = make(map[string]bool)
		registeredConfigHashes = make(map[string]string)
	)

	// registerDevices detects USB audio devices and registers new ones with the supervisor.
	registerDevices := func(cfg *config.Config) int {
		return registerNewDevices(ctx, logger, cfg, flags, ffmpegPath, sup,
			&registeredMu, registeredServices, registeredConfigHashes)
	}

	// Initial device registration
	registerDevices(cfg)

	if sup.ServiceCount() == 0 {
		logger.Info("no USB audio devices found, waiting for devices")
	} else {
		logger.Info("detected USB audio devices", "count", sup.ServiceCount())
	}

	// Start background goroutines
	go startDevicePoller(ctx, logger, koanfCfg, cfg, registerDevices)

	// Bridge os.Signal channel to struct{} channel for testability
	reloadBridge := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-reloadCh:
				reloadBridge <- struct{}{}
			case <-ctx.Done():
				return
			}
		}
	}()
	go startReloadHandler(ctx, logger, reloadBridge, koanfCfg, sup,
		&registeredMu, registeredServices, registeredConfigHashes, registerDevices)

	// GAP-5/A-5: Start systemd watchdog heartbeat goroutine.
	startWatchdog(ctx, logger)

	// Start health check HTTP server
	startHealthEndpoint(ctx, logger, cfg, sup)

	// P-3 fix: Periodic recovery for permanently failed streams.
	go startFailedStreamRecovery(ctx, logger, cfg.Monitor.Interval, sup,
		&registeredMu, registeredServices, registeredConfigHashes)

	// P-1/P-2 fix: Stream health check loop using MediaMTX API.
	if cfg.Monitor.Enabled {
		go startStallDetector(ctx, logger, cfg, sup,
			&registeredMu, registeredServices, registeredConfigHashes)
	}

	// GAP-1c: Segment retention goroutine.
	if cfg.Stream.LocalRecordDir != "" &&
		(cfg.Stream.SegmentMaxAge > 0 || cfg.Stream.SegmentMaxTotalBytes > 0) {
		go runSegmentRetention(ctx, logger, cfg.Stream)
	}

	// GAP-1d: Disk space monitoring goroutine.
	if cfg.Monitor.DiskLowThresholdMB > 0 {
		go runDiskSpaceMonitor(ctx, logger, cfg)
	}

	// Run supervisor (blocks until shutdown)
	logger.Info("starting supervisor", "streams", sup.ServiceCount())
	if err := sup.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("supervisor error", "error", err)
	}

	logger.Info("shutdown complete")
	return 0
}

// registerNewDevices detects USB audio devices and registers new ones with the supervisor.
// Returns the number of newly registered devices.
func registerNewDevices(
	ctx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	flags daemonFlags,
	ffmpegPath string,
	sup *supervisor.Supervisor,
	registeredMu *sync.RWMutex,
	registeredServices map[string]bool,
	registeredConfigHashes map[string]string,
) int {
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil {
		logger.Warn("failed to detect audio devices", "error", err)
		return 0
	}

	registered := 0
	for _, dev := range devices {
		devName := audio.SanitizeDeviceName(dev.Name)

		registeredMu.RLock()
		alreadyRegistered := registeredServices[devName]
		registeredMu.RUnlock()
		if alreadyRegistered {
			continue
		}

		// P-7 fix: Wait for USB device to finish Linux initialization
		if cfg.Stream.USBStabilizationDelay > 0 {
			logger.Debug("waiting for USB device to stabilize", "device", devName, "delay", cfg.Stream.USBStabilizationDelay)
			select {
			case <-time.After(cfg.Stream.USBStabilizationDelay):
			case <-ctx.Done():
				return registered
			}
		}

		devCfg := cfg.GetDeviceConfig(devName)
		streamName := devName
		rtspURL := fmt.Sprintf("%s/%s", cfg.MediaMTX.RTSPURL, streamName)
		alsaDevice := fmt.Sprintf("hw:%d,0", dev.CardNumber)

		mgrCfg := &stream.ManagerConfig{
			DeviceName:      devName,
			ALSADevice:      alsaDevice,
			StreamName:      streamName,
			SampleRate:      devCfg.SampleRate,
			Channels:        devCfg.Channels,
			Bitrate:         devCfg.Bitrate,
			Codec:           devCfg.Codec,
			ThreadQueue:     devCfg.ThreadQueue,
			RTSPURL:         rtspURL,
			LockDir:         flags.LockDir,
			LogDir:          flags.LogDir,
			FFmpegPath:      ffmpegPath,
			StopTimeout:     cfg.Stream.StopTimeout,
			LocalRecordDir:  cfg.Stream.LocalRecordDir,
			SegmentDuration: cfg.Stream.SegmentDuration,
			SegmentFormat:   cfg.Stream.SegmentFormat,
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
		registeredConfigHashes[devName] = deviceConfigHash(devCfg, rtspURL, cfg.Stream)
		registeredMu.Unlock()
		registered++
		logger.Info("registered stream", "alsa_device", alsaDevice, "rtsp_url", rtspURL)
	}

	return registered
}

// startHealthEndpoint starts the health check HTTP server.
func startHealthEndpoint(ctx context.Context, logger *slog.Logger, cfg *config.Config, sup *supervisor.Supervisor) {
	healthAddr := cfg.Monitor.HealthAddr
	if healthAddr == "" {
		healthAddr = "127.0.0.1:9998"
	}
	sysInfoProvider := &daemonSystemInfoProvider{
		recordDir:        cfg.Stream.LocalRecordDir,
		diskLowThreshold: uint64(cfg.Monitor.DiskLowThresholdMB) * 1024 * 1024, //#nosec G115
	}
	healthHandler := health.NewHandler(&supervisorStatusProvider{sup: sup}).
		WithSystemInfo(sysInfoProvider)
	healthReady := make(chan struct{})
	go func() {
		if err := health.ListenAndServeReady(ctx, healthAddr, healthHandler, healthReady); err != nil {
			logger.Warn("health endpoint error", "error", err)
		}
	}()
	select {
	case <-healthReady:
		logger.Info("health endpoint listening", "addr", healthAddr)
	case <-time.After(2 * time.Second):
		logger.Warn("health endpoint did not start within 2s, continuing without health monitoring")
	case <-ctx.Done():
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
