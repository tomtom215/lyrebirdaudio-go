// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/mediamtx"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// kickRecoveryTimeout caps how long the stall detector will spend kicking
// reader sessions before giving up and proceeding to the hard restart. A
// hung MediaMTX API must not be allowed to stall recovery itself.
const kickRecoveryTimeout = 3 * time.Second

// kickStalledPathReaders tries to force-disconnect every RTSP reader session
// attached to the given path name. It is called as a soft cleanup step right
// before the supervisor hard-removes a stalled publisher.
//
// The function is deliberately best-effort:
//
//   - It uses its own short-timeout context (kickRecoveryTimeout) so a wedged
//     MediaMTX API can never stall the stall detector itself.
//   - It filters sessions to state=="read" so the publisher (the "publish"
//     session) is never accidentally kicked — the caller already handles that.
//   - ErrSessionNotFound is swallowed silently: the session raced us and is
//     already gone, which is exactly what we wanted.
//   - Any other per-session error is logged at debug level and iteration
//     continues; one bad session does not prevent kicking the rest.
//   - An overall list-sessions failure is logged and the function returns
//     without kicking anything; the caller proceeds to the hard restart.
func kickStalledPathReaders(ctx context.Context, logger *slog.Logger, client *mediamtx.Client, pathName string) {
	kickCtx, cancel := context.WithTimeout(ctx, kickRecoveryTimeout)
	defer cancel()

	sessions, err := client.ListRTSPSessions(kickCtx)
	if err != nil {
		logger.Debug("stall recovery: list sessions failed", "stream", pathName, "error", err)
		return
	}

	var kicked, failed int
	for _, s := range sessions {
		if s.Path != pathName || s.State != "read" {
			continue
		}
		if err := client.KickRTSPSession(kickCtx, s.ID); err != nil {
			if errors.Is(err, mediamtx.ErrSessionNotFound) {
				// Session disappeared between list and kick — fine.
				continue
			}
			failed++
			logger.Debug("stall recovery: kick session failed",
				"stream", pathName, "session_id", s.ID, "remote", s.RemoteAddr, "error", err)
			continue
		}
		kicked++
		logger.Info("stall recovery: kicked reader session",
			"stream", pathName, "session_id", s.ID, "remote", s.RemoteAddr)
	}
	if kicked > 0 || failed > 0 {
		logger.Info("stall recovery: session cleanup complete",
			"stream", pathName, "kicked", kicked, "failed", failed)
	}
}

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

			// Prune stall state for devices removed elsewhere (SIGHUP reload,
			// failed-stream recovery). Without this, a stale prevBytes/stallCount
			// carried into a re-registered device triggers a spurious restart or
			// bogus "stalled" warnings right after a reload.
			live := make(map[string]struct{}, len(names))
			for _, n := range names {
				live[n] = struct{}{}
			}
			for n := range stallCount {
				if _, ok := live[n]; !ok {
					delete(stallCount, n)
					delete(prevBytes, n)
				}
			}

			for _, name := range names {
				stats, err := mtxClient.GetStreamStats(ctx, name)
				if err != nil {
					logger.Debug("stream health check failed", "stream", name, "error", err)
					continue
				}

				if stats.Ready && stats.BytesReceived > 0 {
					// Only a byte counter that did NOT advance since the last check
					// is a stall. A DECREASE means the publisher reconnected (a new
					// RTSP session resets the counter) — a restart, not a stall — so
					// it resets the count rather than driving toward a restart.
					if prev, ok := prevBytes[name]; ok && stats.BytesReceived == prev {
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
					// Belt-and-suspenders cleanup: kick any lingering RTSP
					// reader sessions attached to this stalled path before
					// removing the publisher. In most cases MediaMTX will
					// tear down readers itself when the publisher exits,
					// but kicking first ensures a clean server-side state
					// machine when the publisher reconnects — this matters
					// in the "stuck reader back-pressuring the publisher"
					// case. Failures here are non-fatal: we still proceed
					// to the hard restart below.
					kickStalledPathReaders(ctx, logger, mtxClient, name)

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
