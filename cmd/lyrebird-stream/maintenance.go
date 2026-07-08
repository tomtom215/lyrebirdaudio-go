// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

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
// Each ping is gated on the healthy probe: before sending the keepalive, the
// probe is run under a bounded context, and if it fails (or times out) the ping
// is WITHHELD so systemd's WatchdogSec restarts a wedged daemon. An unconditional
// ping would keep feeding the watchdog even if the daemon were logically hung
// (e.g. the supervisor mutex held forever), defeating the whole mechanism. A nil
// probe preserves the old always-ping behavior.
//
// If WATCHDOG_USEC is not set (i.e., not running under systemd with WatchdogSec=),
// the goroutine exits immediately.
func startWatchdog(ctx context.Context, logger *slog.Logger, healthy func(context.Context) bool) {
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
				if healthy != nil {
					probeCtx, cancel := context.WithTimeout(ctx, interval)
					ok := healthy(probeCtx)
					cancel()
					if !ok {
						logger.Warn("watchdog liveness probe failed; withholding keepalive so systemd can restart a wedged daemon")
						continue
					}
				}
				if err := sdNotify("WATCHDOG=1"); err != nil {
					logger.Warn("watchdog notify failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
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
