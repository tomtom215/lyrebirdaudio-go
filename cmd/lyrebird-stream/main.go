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
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/health"
	"github.com/tomtom215/lyrebirdaudio-go/internal/mediamtx"
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
// Using a struct instead of global variables makes the daemon testable:
// tests can call runDaemon(daemonFlags{...}) without touching flag.Parse().
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

	// registeredServices tracks which device names have been registered with the
	// supervisor.  It is accessed from the main goroutine, the poll goroutine,
	// and the SIGHUP reload goroutine — a plain map would be a data race (C-2,
	// L-14 fix).
	//
	// registeredConfigHashes stores a stable hash of the ManagerConfig for each
	// registered device so that SIGHUP can detect parameter changes and restart
	// only the affected streams (M-6 fix).
	var (
		registeredMu           sync.RWMutex
		registeredServices     = make(map[string]bool)
		registeredConfigHashes = make(map[string]string)
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

			// P-7 fix: Wait for USB device to finish Linux initialization
			// before creating the stream manager. Without this delay, a
			// hot-plugged device may not be fully ready when FFmpeg opens it.
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
				StopTimeout:     cfg.Stream.StopTimeout,     // H-1 fix
				LocalRecordDir:  cfg.Stream.LocalRecordDir,  // C-1 fix
				SegmentDuration: cfg.Stream.SegmentDuration, // C-1 fix
				SegmentFormat:   cfg.Stream.SegmentFormat,   // C-1 fix
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

				// M-6 fix: detect parameter changes and restart affected streams.
				// Collect registered device names under RLock, then compare hashes.
				registeredMu.RLock()
				names := make([]string, 0, len(registeredServices))
				for name := range registeredServices {
					names = append(names, name)
				}
				registeredMu.RUnlock()

				for _, devName := range names {
					newDevCfg := newCfg.GetDeviceConfig(devName)
					newRTSPURL := fmt.Sprintf("%s/%s", newCfg.MediaMTX.RTSPURL, devName)
					newHash := deviceConfigHash(newDevCfg, newRTSPURL, newCfg.Stream)

					registeredMu.RLock()
					oldHash := registeredConfigHashes[devName]
					registeredMu.RUnlock()

					logger.Info("device config after reload",
						"device", devName,
						"sample_rate", newDevCfg.SampleRate,
						"channels", newDevCfg.Channels,
						"codec", newDevCfg.Codec,
						"bitrate", newDevCfg.Bitrate,
						"config_changed", oldHash != newHash)

					if oldHash == newHash {
						continue
					}

					// Config changed — stop the old stream so registerDevices
					// will restart it with the new parameters.
					logger.Info("config changed for device, restarting stream", "device", devName)
					if removeErr := sup.Remove(devName); removeErr != nil {
						logger.Warn("failed to remove service for restart", "device", devName, "error", removeErr)
						continue
					}
					registeredMu.Lock()
					delete(registeredServices, devName)
					delete(registeredConfigHashes, devName)
					registeredMu.Unlock()
				}

				n := registerDevices(newCfg)
				if n > 0 {
					logger.Info("registered new/restarted devices on reload", "count", n)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// GAP-5/A-5: Start systemd watchdog heartbeat goroutine.
	// Sends WATCHDOG=1 via NOTIFY_SOCKET every half of WATCHDOG_USEC.
	// If the Go runtime deadlocks, the heartbeat stops and systemd restarts the unit.
	// Requires WatchdogSec= and NotifyAccess=main in the systemd unit file.
	startWatchdog(ctx, logger)

	// Start health check HTTP server (M-3 fix).
	// SEC-1: Bind to localhost only to prevent network exposure of service status.
	// The health endpoint is intended for local monitoring (systemd, localhost probes)
	// and should not be accessible from the network by default.
	// P-4 fix: Wire real StatusProvider that reports supervisor service states,
	// replacing the nil provider that always returned 503.
	// H-3 fix: Use ListenAndServeReady with a ready channel to verify the health
	// endpoint is actually listening before completing initialization. Bind failures
	// (e.g., port already in use) are detected synchronously.
	// GAP-8: Health endpoint address is now configurable via monitor.health_addr.
	healthAddr := cfg.Monitor.HealthAddr
	if healthAddr == "" {
		healthAddr = "127.0.0.1:9998"
	}
	// DiskLowThresholdMB is validated non-negative in Config.Validate; safe to
	// convert to uint64 for unsigned comparison with syscall.Statfs free bytes.
	sysInfoProvider := &daemonSystemInfoProvider{
		recordDir:        cfg.Stream.LocalRecordDir,
		diskLowThreshold: uint64(cfg.Monitor.DiskLowThresholdMB) * 1024 * 1024, //#nosec G115 -- cfg.Monitor.DiskLowThresholdMB is always ≥ 0
	}
	healthHandler := health.NewHandler(&supervisorStatusProvider{sup: sup}).
		WithSystemInfo(sysInfoProvider)
	healthReady := make(chan struct{})
	go func() {
		if err := health.ListenAndServeReady(ctx, healthAddr, healthHandler, healthReady); err != nil {
			// Not fatal: health endpoint failure must not crash the daemon.
			logger.Warn("health endpoint error", "error", err)
		}
	}()
	// H-3 fix: Wait for health endpoint to bind or detect bind failure.
	select {
	case <-healthReady:
		logger.Info("health endpoint listening", "addr", healthAddr)
	case <-time.After(2 * time.Second):
		logger.Warn("health endpoint did not start within 2s, continuing without health monitoring")
	case <-ctx.Done():
		// Shutting down before health started
	}

	// P-3 fix: Periodic recovery for permanently failed streams.
	// When a stream exceeds MaxRestartAttempts, it enters StateFailed and the
	// manager's Run() returns. This goroutine periodically clears failed
	// registrations so that the device polling loop can re-register them with
	// a fresh manager and reset backoff — but only if the USB device is still
	// present. This prevents permanent death after transient failures.
	go func() {
		recoveryInterval := cfg.Monitor.Interval
		if recoveryInterval <= 0 {
			recoveryInterval = 5 * time.Minute
		}
		ticker := time.NewTicker(recoveryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				statuses := sup.Status()
				for _, status := range statuses {
					if status.State != supervisor.ServiceStateFailed {
						continue
					}
					logger.Info("attempting recovery of failed stream", "device", status.Name, "restarts", status.Restarts)
					if removeErr := sup.Remove(status.Name); removeErr != nil {
						logger.Warn("failed to remove failed service for recovery", "device", status.Name, "error", removeErr)
						continue
					}
					registeredMu.Lock()
					delete(registeredServices, status.Name)
					delete(registeredConfigHashes, status.Name)
					registeredMu.Unlock()
					logger.Info("cleared failed stream for re-registration", "device", status.Name)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// P-1/P-2 fix: Stream health check loop using MediaMTX API client.
	// This wires the previously dead-code mediamtx client into the daemon to
	// detect silent/stalled streams. Polls MediaMTX API at the configured
	// stall check interval. If a stream's bytes_received stops increasing for
	// maxStallChecks consecutive polls, the stream is restarted.
	//
	// H-2 fix: Uses separate StallCheckInterval (default 60s) instead of the
	// general monitor interval (5 min). This reduces stall detection from
	// 15 minutes (3 x 5 min) to 3 minutes (3 x 60s).
	if cfg.Monitor.Enabled {
		go func() {
			mtxClient := mediamtx.NewClient(cfg.MediaMTX.APIURL)
			// H-2 fix: use StallCheckInterval for stall detection, falling back
			// to 60 seconds if unset, rather than the general monitor interval.
			checkInterval := cfg.Monitor.StallCheckInterval
			if checkInterval <= 0 {
				checkInterval = 60 * time.Second
			}
			ticker := time.NewTicker(checkInterval)
			defer ticker.Stop()

			// Track bytes received per stream for stall detection.
			prevBytes := make(map[string]int64)
			stallCount := make(map[string]int)
			// H-2 fix: use configurable MaxStallChecks instead of hardcoded 3.
			maxStallChecks := cfg.Monitor.MaxStallChecks
			if maxStallChecks <= 0 {
				maxStallChecks = 3
			}

			for {
				select {
				case <-ticker.C:
					registeredMu.RLock()
					names := make([]string, 0, len(registeredServices))
					for name := range registeredServices {
						names = append(names, name)
					}
					registeredMu.RUnlock()

					for _, name := range names {
						stats, err := mtxClient.GetStreamStats(ctx, name)
						if err != nil {
							// MediaMTX may not be running yet; debug-level only.
							logger.Debug("stream health check failed", "stream", name, "error", err)
							continue
						}

						if stats.Ready && stats.BytesReceived > 0 {
							if prev, ok := prevBytes[name]; ok && stats.BytesReceived <= prev {
								stallCount[name]++
								logger.Warn("stream data stalled", "stream", name, "bytes", stats.BytesReceived, "stall_count", stallCount[name])
							} else {
								stallCount[name] = 0
							}
							prevBytes[name] = stats.BytesReceived
						} else {
							stallCount[name]++
							logger.Warn("stream not ready or no data", "stream", name, "ready", stats.Ready, "bytes", stats.BytesReceived, "stall_count", stallCount[name])
						}

						if cfg.Monitor.RestartUnhealthy && stallCount[name] >= maxStallChecks {
							logger.Warn("restarting stalled stream", "stream", name, "stall_count", stallCount[name])
							if removeErr := sup.Remove(name); removeErr != nil {
								logger.Warn("failed to remove stalled service", "stream", name, "error", removeErr)
								continue
							}
							registeredMu.Lock()
							delete(registeredServices, name)
							delete(registeredConfigHashes, name)
							registeredMu.Unlock()
							delete(stallCount, name)
							delete(prevBytes, name)
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// GAP-1c: Segment retention goroutine.
	// Periodically scans LocalRecordDir and deletes segments older than
	// SegmentMaxAge or when total size exceeds SegmentMaxTotalBytes.
	// This prevents ENOSPC on long-running unattended deployments.
	if cfg.Stream.LocalRecordDir != "" &&
		(cfg.Stream.SegmentMaxAge > 0 || cfg.Stream.SegmentMaxTotalBytes > 0) {
		go runSegmentRetention(ctx, logger, cfg.Stream)
	}

	// GAP-1d: Disk space monitoring goroutine.
	// Warns when free disk drops below the configured threshold.
	// Runs independently of the stall check loop so it fires even when
	// monitor.enabled = false.
	if cfg.Monitor.DiskLowThresholdMB > 0 {
		go runDiskSpaceMonitor(ctx, logger, cfg)
	}

	// Run supervisor (blocks until shutdown)
	logger.Info("starting supervisor", "streams", sup.ServiceCount())
	if err := sup.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		// M-1 fix: use errors.Is to handle wrapped context errors.
		logger.Error("supervisor error", "error", err)
	}

	logger.Info("shutdown complete")
	return 0
}

// supervisorStatusProvider implements health.StatusProvider by querying the
// supervisor for live service state. This replaces the nil provider that was
// previously passed to health.NewHandler (P-4 fix).
type supervisorStatusProvider struct {
	sup *supervisor.Supervisor
}

func (p *supervisorStatusProvider) Services() []health.ServiceInfo {
	statuses := p.sup.Status()
	services := make([]health.ServiceInfo, len(statuses))
	for i, s := range statuses {
		services[i] = health.ServiceInfo{
			Name:     s.Name,
			State:    s.State.String(),
			Uptime:   s.Uptime,
			Healthy:  s.State == supervisor.ServiceStateRunning,
			Restarts: s.Restarts,
		}
		if s.LastError != nil {
			services[i].Error = s.LastError.Error()
		}
	}
	return services
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

// sdNotify sends a state notification to systemd via NOTIFY_SOCKET.
// This is a pure-Go implementation with no cgo dependency.
// Used for WATCHDOG=1 keepalive pings (GAP-2 / A-5).
func sdNotify(state string) error {
	socketPath := os.Getenv("NOTIFY_SOCKET")
	if socketPath == "" {
		return nil // Not running under systemd; silently ignore.
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		return fmt.Errorf("sd_notify dial: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(state)); err != nil {
		return fmt.Errorf("sd_notify write: %w", err)
	}
	return nil
}

// startWatchdog starts a goroutine that sends WATCHDOG=1 pings to systemd
// on a schedule derived from the WATCHDOG_USEC environment variable (GAP-2 / A-5).
//
// If WATCHDOG_USEC is not set (i.e., not running under systemd with WatchdogSec=),
// the goroutine exits immediately.
func startWatchdog(ctx context.Context, logger *slog.Logger) {
	usecStr := os.Getenv("WATCHDOG_USEC")
	if usecStr == "" {
		return // watchdog not configured
	}

	var usec int64
	if _, err := fmt.Sscanf(usecStr, "%d", &usec); err != nil || usec <= 0 {
		logger.Warn("invalid WATCHDOG_USEC, watchdog disabled", "value", usecStr)
		return
	}

	// Ping at half the watchdog interval for safety margin.
	interval := time.Duration(usec/2) * time.Microsecond
	logger.Info("systemd watchdog enabled", "interval", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sdNotify("WATCHDOG=1"); err != nil {
					logger.Warn("watchdog notify failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// daemonSystemInfoProvider implements health.SystemInfoProvider for the daemon.
// It reports disk space for the recording directory and NTP sync status (GAP-7, GAP-1d).
type daemonSystemInfoProvider struct {
	recordDir        string // LocalRecordDir, or "/" if empty
	diskLowThreshold uint64 // bytes; 0 = disabled (always initialized from a positive int64)
}

func (p *daemonSystemInfoProvider) SystemInfo() health.SystemInfo {
	dir := p.recordDir
	if dir == "" {
		dir = "/"
	}

	var si health.SystemInfo

	// Disk space check via syscall.Statfs.
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err == nil {
		si.DiskFreeBytes = stat.Bavail * uint64(stat.Bsize)  //#nosec G115 -- Bavail and Bsize are always ≥ 0
		si.DiskTotalBytes = stat.Blocks * uint64(stat.Bsize) //#nosec G115 -- same
		if p.diskLowThreshold > 0 && si.DiskFreeBytes < p.diskLowThreshold {
			si.DiskLowWarning = true
		}
	}

	// NTP sync check via timedatectl.
	out, err := exec.Command("timedatectl", "show", "--property=NTPSynchronized", "--value").Output() //#nosec G204 -- fixed args
	if err == nil && strings.TrimSpace(string(out)) == "yes" {
		si.NTPSynced = true
	} else {
		si.NTPMessage = "NTP not synchronized or timedatectl unavailable"
	}

	return si
}

// runSegmentRetention is a goroutine that periodically deletes old recording
// segments to prevent disk exhaustion on unattended deployments (GAP-1c / A-3).
//
// Deletion policy (applied in order):
//  1. Files older than SegmentMaxAge are deleted first.
//  2. If total remaining size exceeds SegmentMaxTotalBytes, oldest files are
//     deleted until total size is within budget.
func runSegmentRetention(ctx context.Context, logger *slog.Logger, streamCfg config.StreamConfig) {
	// Run cleanup once at startup, then every hour.
	const cleanupInterval = 1 * time.Hour

	doCleanup := func() {
		cleanupSegments(logger, streamCfg)
	}

	doCleanup() // initial pass

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			doCleanup()
		case <-ctx.Done():
			return
		}
	}
}

// cleanupSegments performs one pass of segment file cleanup.
func cleanupSegments(logger *slog.Logger, streamCfg config.StreamConfig) {
	dir := streamCfg.LocalRecordDir
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("segment retention: failed to read recording directory", "dir", dir, "error", err)
		return
	}

	now := time.Now()
	type segFile struct {
		path    string
		modTime time.Time
		size    int64
	}

	var files []segFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, segFile{
			path:    filepath.Join(dir, e.Name()),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}

	// Step 1: Delete files older than SegmentMaxAge.
	if streamCfg.SegmentMaxAge > 0 {
		cutoff := now.Add(-streamCfg.SegmentMaxAge)
		remaining := files[:0]
		for _, f := range files {
			if f.modTime.Before(cutoff) {
				if err := os.Remove(f.path); err != nil {
					logger.Warn("segment retention: failed to delete old segment", "path", f.path, "error", err)
				} else {
					logger.Info("segment retention: deleted old segment", "path", f.path, "age", now.Sub(f.modTime).Round(time.Minute))
				}
				continue
			}
			remaining = append(remaining, f)
		}
		files = remaining
	}

	// Step 2: Delete oldest files until total size is within SegmentMaxTotalBytes.
	if streamCfg.SegmentMaxTotalBytes > 0 {
		// Sort ascending by mod time (oldest first).
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.Before(files[j].modTime)
		})

		var totalBytes int64
		for _, f := range files {
			totalBytes += f.size
		}

		for _, f := range files {
			if totalBytes <= streamCfg.SegmentMaxTotalBytes {
				break
			}
			if err := os.Remove(f.path); err != nil {
				logger.Warn("segment retention: failed to delete segment for size budget", "path", f.path, "error", err)
				continue
			}
			logger.Info("segment retention: deleted segment for size budget",
				"path", f.path,
				"freed_bytes", f.size,
				"total_bytes_before", totalBytes,
				"budget_bytes", streamCfg.SegmentMaxTotalBytes,
			)
			totalBytes -= f.size
		}
	}
}

// runDiskSpaceMonitor is a goroutine that warns when free disk space drops
// below the configured threshold (GAP-1d / A-4).
func runDiskSpaceMonitor(ctx context.Context, logger *slog.Logger, cfg *config.Config) {
	const checkInterval = 5 * time.Minute

	checkDisk := func() {
		dir := cfg.Stream.LocalRecordDir
		if dir == "" {
			dir = "/"
		}

		var stat syscall.Statfs_t
		if err := syscall.Statfs(dir, &stat); err != nil {
			logger.Warn("disk space check failed", "dir", dir, "error", err)
			return
		}

		freeBytes := stat.Bavail * uint64(stat.Bsize)                          //#nosec G115
		totalBytes := stat.Blocks * uint64(stat.Bsize)                         //#nosec G115
		thresholdBytes := uint64(cfg.Monitor.DiskLowThresholdMB) * 1024 * 1024 //#nosec G115 -- DiskLowThresholdMB > 0 is checked before goroutine start

		if freeBytes < thresholdBytes {
			logger.Warn("LOW DISK SPACE WARNING",
				"dir", dir,
				"free_bytes", freeBytes,
				"free_gb", fmt.Sprintf("%.2f", float64(freeBytes)/1e9),
				"total_gb", fmt.Sprintf("%.2f", float64(totalBytes)/1e9),
				"threshold_mb", cfg.Monitor.DiskLowThresholdMB,
			)
		}
	}

	checkDisk() // initial check

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			checkDisk()
		case <-ctx.Done():
			return
		}
	}
}
