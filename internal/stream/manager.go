// SPDX-License-Identifier: MIT

// Package stream provides FFmpeg process lifecycle management for audio streaming.
//
// This package implements the core streaming functionality of LyreBirdAudio,
// managing FFmpeg processes that capture audio from ALSA devices and stream
// them to MediaMTX via RTSP.
//
// Key components:
//   - Manager: Manages FFmpeg process lifecycle with automatic restart
//   - Backoff: Implements exponential backoff for process restarts
//   - RotatingWriter: Log rotation for FFmpeg output
//   - ResourceMetrics: Process resource monitoring
//
// The Manager uses a state machine with the following states:
//   - StateIdle: Not started
//   - StateStarting: Acquiring lock and starting FFmpeg
//   - StateRunning: FFmpeg process running
//   - StateStopping: Gracefully stopping FFmpeg
//   - StateFailed: FFmpeg failed, waiting for backoff
//   - StateStopped: Stopped (terminal state)
//
// Reference: mediamtx-stream-manager.sh from the original bash implementation
package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/lock"
)

// ManagerConfig contains configuration for a stream manager.
type ManagerConfig struct {
	DeviceName    string // Sanitized device name (e.g., "blue_yeti")
	ALSADevice    string // ALSA device identifier (e.g., "hw:0,0") or lavfi source
	InputFormat   string // Input format: "alsa" or "lavfi" (default: "alsa")
	RealtimeInput bool   // Pace a non-hardware input to real time with -re (e.g. a synthetic lavfi source); leave false for hardware ALSA capture, which is already real-time

	StreamName      string                // Stream name for MediaMTX path
	SampleRate      int                   // Sample rate in Hz
	Channels        int                   // Number of channels
	Bitrate         string                // Bitrate (e.g., "128k")
	Codec           string                // Codec ("opus" or "aac")
	ThreadQueue     int                   // FFmpeg thread queue size (optional)
	RTSPURL         string                // Full RTSP URL or file path for output
	OutputFormat    string                // Output format: "rtsp", "null", or empty for auto-detect (default: "rtsp")
	LockDir         string                // Directory for lock files
	FFmpegPath      string                // Path to ffmpeg binary
	Backoff         *Backoff              // Backoff policy for restarts
	Logger          *slog.Logger          // Optional structured logger (nil = no logging)
	LogDir          string                // Directory for FFmpeg log files (empty = no logging)
	MonitorInterval time.Duration         // Interval for resource monitoring (0 = disabled)
	AlertCallback   func([]ResourceAlert) // Optional callback for resource alerts
	StopTimeout     time.Duration         // Timeout for graceful FFmpeg stop before force-kill (default: 5s) (H-1 fix)
	LocalRecordDir  string                // Directory for local audio recording segments (C-1 fix, empty = disabled)
	SegmentDuration int                   // Duration in seconds for local recording segments (default: 3600 = 1 hour)
	SegmentFormat   string                // Format for local recording segments: "wav", "flac", "ogg" (default: "wav")
}

// Manager manages a single audio stream's lifecycle.
//
// State Machine:
//
//	idle → starting → running ⟲
//	                    ↓
//	                  failed → (backoff) → starting
//	                    ↓
//	                  stopped (terminal)
type Manager struct {
	cfg *ManagerConfig

	// State management
	state   atomic.Value // State
	mu      sync.RWMutex // Protects cmd, lock, startTime, logWriter
	cmd     *exec.Cmd
	lock    *lock.FileLock
	backoff *Backoff

	// Log rotation for FFmpeg stderr
	logWriter io.WriteCloser

	// Resource monitoring for FFmpeg process
	resourceMonitor *ResourceMonitor
	monitorCancel   context.CancelFunc

	// Metrics
	startTime time.Time
	attempts  atomic.Int32
	failures  atomic.Int32
}

// NewManager creates a new stream manager.
func NewManager(cfg *ManagerConfig) (*Manager, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	mgr := &Manager{
		cfg:     cfg,
		backoff: cfg.Backoff,
	}

	mgr.state.Store(StateIdle)

	// Create rotating log writer if LogDir is specified
	if cfg.LogDir != "" {
		logWriter, err := newManagerLogWriter(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create log writer: %w", err)
		}
		mgr.logWriter = logWriter
	}

	// Create resource monitor if monitoring is enabled
	if cfg.MonitorInterval > 0 {
		mgr.resourceMonitor = NewResourceMonitor(
			WithLogger(cfg.Logger),
		)
	}

	return mgr, nil
}

// newManagerLogWriter builds the rotating FFmpeg-stderr writer for cfg.
// Shared by NewManager and ensureLogWriter so the two paths cannot drift.
func newManagerLogWriter(cfg *ManagerConfig) (io.WriteCloser, error) {
	return LogWriter(cfg.LogDir, cfg.StreamName,
		WithMaxSize(DefaultMaxLogSize),
		WithMaxFiles(DefaultMaxLogFiles),
		WithCompression(true),
		// Surface rotation/compression failures (e.g. a full log disk)
		// instead of silently discarding them; without a logger the
		// RotatingWriter swallows these errors (M-3), hiding disk problems
		// on an unattended device. nil logger is fine (no-op).
		WithRotateLogger(cfg.Logger))
}

// ensureLogWriter recreates the rotating FFmpeg log writer if it was closed.
//
// streamService closes the manager after EVERY Run return, and the supervisor
// re-runs the SAME service after a failure (e.g. a lock-acquire timeout while
// a competing process holds the device lock at startup). Without reopening
// here, every FFmpeg run after the first supervisor re-run would silently
// discard its stderr — an unattended device could stream for months with
// empty logs, leaving nothing to diagnose a later failure with.
//
// Unlike NewManager, a failure here is logged and tolerated: mid-life,
// keeping the audio stream alive matters more than its log file (a full log
// disk must not take down the stream).
func (m *Manager) ensureLogWriter() {
	if m.cfg.LogDir == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.logWriter != nil {
		return
	}

	logWriter, err := newManagerLogWriter(m.cfg)
	if err != nil {
		if m.cfg.Logger != nil {
			m.cfg.Logger.Warn("failed to reopen FFmpeg log writer; continuing without FFmpeg logging",
				"device", m.cfg.DeviceName, "error", err)
		}
		return
	}
	m.logWriter = logWriter
}

// logf writes a formatted info-level log message if Logger is configured.
func (m *Manager) logf(format string, args ...interface{}) {
	if m.cfg.Logger != nil {
		m.cfg.Logger.Info(fmt.Sprintf(format, args...), "device", m.cfg.DeviceName)
	}
}

// logError writes an error-level log message if Logger is configured.
func (m *Manager) logError(format string, args ...interface{}) {
	if m.cfg.Logger != nil {
		m.cfg.Logger.Error(fmt.Sprintf(format, args...), "device", m.cfg.DeviceName)
	}
}

// logStructuredEvent emits a structured log event with machine-parseable fields
// for post-hoc failure analysis from journald or log aggregation (H-4 fix).
func (m *Manager) logStructuredEvent(event string, attrs ...interface{}) {
	if m.cfg.Logger != nil {
		allAttrs := make([]interface{}, 0, len(attrs)+4)
		allAttrs = append(allAttrs, "event", event, "device", m.cfg.DeviceName, "stream", m.cfg.StreamName)
		allAttrs = append(allAttrs, attrs...)
		m.cfg.Logger.Info("stream_event", allAttrs...)
	}
}

// Run starts the stream manager's main loop.
//
// This function:
//  1. Acquires file lock for the device
//  2. Starts FFmpeg process
//  3. Monitors process health
//  4. Restarts on failure with exponential backoff
//  5. Releases lock and stops on context cancellation
//
// The function blocks until:
//   - Context is cancelled (graceful shutdown)
//   - Max restart attempts exceeded (failure)
func (m *Manager) Run(ctx context.Context) error {
	// Acquire lock (context-aware for graceful shutdown)
	if err := m.acquireLock(ctx); err != nil {
		m.logf("Failed to acquire lock: %v", err)
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer m.releaseLock()
	m.logf("Lock acquired, starting main loop")

	// Reopen the FFmpeg log writer if a previous Run/Close cycle closed it
	// (the supervisor re-runs a failed service on the same manager).
	m.ensureLogWriter()

	// Main restart loop
	for {
		select {
		case <-ctx.Done():
			m.stop()
			m.setState(StateStopped)
			return ctx.Err()
		default:
		}

		// Check max attempts
		if m.backoff.Attempts() >= m.backoff.MaxAttempts() {
			m.setState(StateFailed)
			return fmt.Errorf("max restart attempts (%d) exceeded", m.backoff.MaxAttempts())
		}

		// Start stream
		m.setState(StateStarting)
		m.attempts.Add(1)
		m.logf("Attempt %d: Starting FFmpeg", m.attempts.Load())

		startTime := time.Now()
		err := m.startFFmpeg(ctx)
		runTime := time.Since(startTime)
		m.logf("FFmpeg exited after %v (err=%v)", runTime, err)

		// Handle result
		if err != nil {
			if errors.Is(err, context.Canceled) {
				m.logf("Context cancelled, stopping")
				m.setState(StateStopped)
				return err
			}

			m.failures.Add(1)
			m.setState(StateFailed)
			m.logStructuredEvent("stream_failure",
				"error", err.Error(),
				"attempt", m.attempts.Load(),
				"failures", m.failures.Load(),
				"run_duration", runTime.String(),
				"next_backoff", m.backoff.CurrentDelay().String(),
			)
			m.logError("FFmpeg failed: %v (failures=%d, next-backoff=%v)", err, m.failures.Load(), m.backoff.CurrentDelay())

			if waitErr := m.backoff.WaitContext(ctx); waitErr != nil {
				m.setState(StateStopped)
				return waitErr
			}
			m.backoff.RecordFailure()
			continue
		}

		// FFmpeg exited cleanly - check if it was a "successful" run
		successThreshold := m.backoff.SuccessThreshold()
		if runTime < successThreshold {
			m.failures.Add(1)
			m.setState(StateFailed)
			m.logStructuredEvent("stream_short_run_failure",
				"run_duration", runTime.String(),
				"threshold", successThreshold.String(),
				"attempt", m.attempts.Load(),
				"failures", m.failures.Load(),
			)
			m.logError("FFmpeg ran for %v (< %v threshold), treating as failure", runTime, successThreshold)

			if waitErr := m.backoff.WaitContext(ctx); waitErr != nil {
				m.setState(StateStopped)
				return waitErr
			}
			m.backoff.RecordFailure()
			continue
		}

		m.logf("FFmpeg ran successfully for %v", runTime)
		m.logStructuredEvent("stream_recovery",
			"run_duration", runTime.String(),
			"attempt", m.attempts.Load(),
			"total_failures", m.failures.Load(),
		)

		// Long successful run - reset backoff
		m.backoff.RecordSuccess(runTime)

		select {
		case <-ctx.Done():
			m.setState(StateStopped)
			return ctx.Err()
		default:
			continue
		}
	}
}

// Close releases resources held by the manager.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.logWriter != nil {
		if err := m.logWriter.Close(); err != nil {
			return fmt.Errorf("failed to close log writer: %w", err)
		}
		m.logWriter = nil
	}

	return nil
}

// acquireLock acquires the file lock for this device.
func (m *Manager) acquireLock(ctx context.Context) error {
	lockPath := filepath.Join(m.cfg.LockDir, m.cfg.DeviceName+".lock")
	fl, err := lock.NewFileLock(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create lock: %w", err)
	}

	if err := fl.AcquireContext(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	m.mu.Lock()
	m.lock = fl
	m.mu.Unlock()

	return nil
}

// releaseLock releases the file lock.
func (m *Manager) releaseLock() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lock != nil {
		if err := m.lock.Release(); err != nil {
			m.logf("Warning: failed to release lock: %v", err)
		}
		m.lock = nil
	}
}
