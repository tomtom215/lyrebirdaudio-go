// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/mediamtx"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// startDevicePoller starts the device polling goroutine that periodically scans
// for newly plugged-in USB devices and registers them with the supervisor.
//
// M-4 fix: the poll runs unconditionally on every tick, not only when the
// service count is zero. This is the correct hotplug support.
func startDevicePoller(
	ctx context.Context,
	logger *slog.Logger,
	koanfCfg *config.KoanfConfig,
	fallbackCfg *config.Config,
	registerDevices func(cfg *config.Config) int,
) {
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
				pollCfg = fallbackCfg
			}
			n := registerDevices(pollCfg)
			if n > 0 {
				logger.Info("discovered new devices", "count", n)
			}
		case <-ctx.Done():
			return
		}
	}
}

// startReloadHandler starts the SIGHUP reload goroutine that reloads
// configuration and restarts streams whose config has changed (M-6 fix).
func startReloadHandler(
	ctx context.Context,
	logger *slog.Logger,
	reloadCh <-chan struct{},
	koanfCfg *config.KoanfConfig,
	sup *supervisor.Supervisor,
	registeredMu *sync.RWMutex,
	registeredServices map[string]bool,
	registeredConfigHashes map[string]string,
	registerDevices func(cfg *config.Config) int,
) {
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

			newCfg, err := koanfCfg.Load()
			if err != nil {
				logger.Warn("failed to load updated config", "error", err)
				continue
			}

			// M-6 fix: detect parameter changes and restart affected streams.
			registeredMu.RLock()
			names := make([]string, 0, len(registeredServices))
			for name := range registeredServices {
				names = append(names, name)
			}
			registeredMu.RUnlock()

			for _, devName := range names {
				newDevCfg := newCfg.GetDeviceConfig(devName)
				newRTSPURL := newCfg.MediaMTX.RTSPURL + "/" + devName
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
}

// startFailedStreamRecovery starts a goroutine that periodically clears failed
// stream registrations so the device polling loop can re-register them (P-3 fix).
func startFailedStreamRecovery(
	ctx context.Context,
	logger *slog.Logger,
	recoveryInterval time.Duration,
	sup *supervisor.Supervisor,
	registeredMu *sync.RWMutex,
	registeredServices map[string]bool,
	registeredConfigHashes map[string]string,
) {
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
}

// startStallDetector starts the stream health check loop using MediaMTX API
// to detect silent/stalled streams (P-1/P-2 fix).
//
// H-2 fix: Uses separate StallCheckInterval (default 60s) instead of the
// general monitor interval (5 min).
func startStallDetector(
	ctx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	sup *supervisor.Supervisor,
	registeredMu *sync.RWMutex,
	registeredServices map[string]bool,
	registeredConfigHashes map[string]string,
) {
	mtxClient := mediamtx.NewClient(cfg.MediaMTX.APIURL)
	checkInterval := cfg.Monitor.StallCheckInterval
	if checkInterval <= 0 {
		checkInterval = 60 * time.Second
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	prevBytes := make(map[string]int64)
	stallCount := make(map[string]int)
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
}
