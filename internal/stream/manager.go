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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/lock"
)

// State represents the stream manager's current state.
type State int

const (
	StateIdle     State = iota // Not started
	StateStarting              // Acquiring lock and starting FFmpeg
	StateRunning               // FFmpeg process running
	StateStopping              // Gracefully stopping FFmpeg
	StateFailed                // FFmpeg failed, waiting for backoff
	StateStopped               // Stopped (terminal state)
)

// String returns the string representation of State.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateFailed:
		return "failed"
	case StateStopped:
		return "stopped"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ManagerConfig contains configuration for a stream manager.
type ManagerConfig struct {
	DeviceName      string                // Sanitized device name (e.g., "blue_yeti")
	ALSADevice      string                // ALSA device identifier (e.g., "hw:0,0") or lavfi source
	InputFormat     string                // Input format: "alsa" or "lavfi" (default: "alsa")
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
// This is the core component that orchestrates:
//   - FFmpeg process management
//   - State machine transitions
//   - Failure recovery with exponential backoff
//   - File-based locking (one manager per device)
//   - Graceful shutdown
//
// State Machine:
//
//	idle → starting → running ⟲
//	                    ↓
//	                  failed → (backoff) → starting
//	                    ↓
//	                  stopped (terminal)
//
// Example:
//
//	cfg := &ManagerConfig{...}
//	mgr, _ := NewManager(cfg)
//	ctx := context.Background()
//	if err := mgr.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
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

// Metrics contains stream manager metrics.
type Metrics struct {
	DeviceName string
	StreamName string
	State      State
	StartTime  time.Time
	Uptime     time.Duration
	Attempts   int
	Failures   int
}

// NewManager creates a new stream manager.
//
// Parameters:
//   - cfg: Manager configuration
//
// Returns:
//   - *Manager: Initialized manager in StateIdle
//   - error: if configuration is invalid
//
// Example:
//
//	cfg := &ManagerConfig{
//	    DeviceName: "blue_yeti",
//	    ALSADevice: "hw:0,0",
//	    SampleRate: 48000,
//	    Channels: 2,
//	    Bitrate: "192k",
//	    Codec: "opus",
//	    RTSPURL: "rtsp://localhost:8554/blue_yeti",
//	    LockDir: "/var/run/lyrebird",
//	    FFmpegPath: "/usr/bin/ffmpeg",
//	    Backoff: NewBackoff(10*time.Second, 300*time.Second, 50),
//	}
//	mgr, err := NewManager(cfg)
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
		logWriter, err := LogWriter(cfg.LogDir, cfg.StreamName,
			WithMaxSize(DefaultMaxLogSize),
			WithMaxFiles(DefaultMaxLogFiles),
			WithCompression(true))
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
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - error: if startup fails or max attempts exceeded
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	errCh := make(chan error)
//	go func() {
//	    errCh <- mgr.Run(ctx)
//	}()
//
//	// ... do other work ...
//
//	cancel() // Trigger graceful shutdown
//	<-errCh  // Wait for completion
func (m *Manager) Run(ctx context.Context) error {
	// Acquire lock (context-aware for graceful shutdown)
	if err := m.acquireLock(ctx); err != nil {
		m.logf("Failed to acquire lock: %v", err)
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer m.releaseLock()
	m.logf("Lock acquired, starting main loop")

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

			// FFmpeg failed — wait with the CURRENT delay, then record the failure
			// (which doubles the delay for the next iteration).  This ensures the
			// first restart waits initialDelay, not 2×initialDelay (ME-1 fix).
			m.failures.Add(1)
			m.setState(StateFailed)
			// H-4 fix: Emit structured failure event with machine-parseable fields
			// for post-hoc analysis from journald or log aggregation systems.
			m.logStructuredEvent("stream_failure",
				"error", err.Error(),
				"attempt", m.attempts.Load(),
				"failures", m.failures.Load(),
				"run_duration", runTime.String(),
				"next_backoff", m.backoff.CurrentDelay().String(),
			)
			m.logError("FFmpeg failed: %v (failures=%d, next-backoff=%v)", err, m.failures.Load(), m.backoff.CurrentDelay())

			// Wait before recording failure so the first restart uses initialDelay.
			if waitErr := m.backoff.WaitContext(ctx); waitErr != nil {
				m.setState(StateStopped)
				return waitErr // Context cancelled during backoff
			}
			m.backoff.RecordFailure()

			// Continue to retry
			continue
		}

		// FFmpeg exited cleanly - check if it was a "successful" run
		successThreshold := m.backoff.SuccessThreshold()
		if runTime < successThreshold {
			// Short run - treat as failure (use RecordFailure, not RecordSuccess,
			// to avoid double-counting attempts since RecordSuccess also increments).
			// Same swap as above: wait first, then record.
			m.failures.Add(1)
			m.setState(StateFailed)
			// H-4 fix: structured event for short-run failure
			m.logStructuredEvent("stream_short_run_failure",
				"run_duration", runTime.String(),
				"threshold", successThreshold.String(),
				"attempt", m.attempts.Load(),
				"failures", m.failures.Load(),
			)
			m.logError("FFmpeg ran for %v (< %v threshold), treating as failure", runTime, successThreshold)

			// Wait before recording failure.
			if waitErr := m.backoff.WaitContext(ctx); waitErr != nil {
				m.setState(StateStopped)
				return waitErr
			}
			m.backoff.RecordFailure()

			// Continue to retry
			continue
		}

		m.logf("FFmpeg ran successfully for %v", runTime)

		// H-4 fix: structured recovery event
		m.logStructuredEvent("stream_recovery",
			"run_duration", runTime.String(),
			"attempt", m.attempts.Load(),
			"total_failures", m.failures.Load(),
		)

		// Long successful run - reset backoff
		m.backoff.RecordSuccess(runTime)

		// If we get here, FFmpeg exited after a long run
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			m.setState(StateStopped)
			return ctx.Err()
		default:
			// Restart immediately (no backoff)
			continue
		}
	}
}

// State returns the current manager state.
//
// Returns StateIdle if the manager was not properly initialized
// (e.g., created via &Manager{} instead of NewManager()).
// This provides safe, defensive behavior for edge cases.
func (m *Manager) State() State {
	if m == nil {
		return StateIdle
	}
	v := m.state.Load()
	if v == nil {
		return StateIdle
	}
	return v.(State)
}

// Attempts returns the total number of start attempts.
func (m *Manager) Attempts() int {
	return int(m.attempts.Load())
}

// Failures returns the total number of failures.
func (m *Manager) Failures() int {
	return int(m.failures.Load())
}

// Metrics returns current manager metrics.
//
// Returns zero-value Metrics if manager is nil.
func (m *Manager) Metrics() Metrics {
	if m == nil {
		return Metrics{State: StateIdle}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var uptime time.Duration
	if !m.startTime.IsZero() {
		uptime = time.Since(m.startTime)
	}

	var deviceName, streamName string
	if m.cfg != nil {
		deviceName = m.cfg.DeviceName
		streamName = m.cfg.StreamName
	}

	return Metrics{
		DeviceName: deviceName,
		StreamName: streamName,
		State:      m.State(),
		StartTime:  m.startTime,
		Uptime:     uptime,
		Attempts:   m.Attempts(),
		Failures:   m.Failures(),
	}
}

// Close releases resources held by the manager.
//
// This should be called after Run() returns to clean up resources such as
// the rotating log writer. It is safe to call multiple times.
//
// Returns:
//   - error: if closing the log writer fails
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

// setState atomically updates the manager state.
func (m *Manager) setState(s State) {
	m.state.Store(s)
}

// acquireLock acquires the file lock for this device.
// Respects context cancellation for graceful shutdown.
func (m *Manager) acquireLock(ctx context.Context) error {
	lockPath := filepath.Join(m.cfg.LockDir, m.cfg.DeviceName+".lock")
	fl, err := lock.NewFileLock(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create lock: %w", err)
	}

	// Try to acquire lock with timeout (context-aware for graceful shutdown)
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

// startFFmpeg starts the FFmpeg process and waits for it to exit.
//
// This function:
//  1. Builds FFmpeg command line
//  2. Starts process
//  3. Transitions to StateRunning
//  4. Waits for process to exit
//  5. Returns error if process fails
//
// Returns:
//   - nil: if FFmpeg exited cleanly
//   - error: if FFmpeg failed or context cancelled
func (m *Manager) startFFmpeg(ctx context.Context) error {
	// GAP-1a: Auto-create LocalRecordDir before FFmpeg launch.
	// buildFFmpegCommand passes the path directly to FFmpeg via the segment
	// pattern; if the directory doesn't exist FFmpeg exits immediately.
	if m.cfg.LocalRecordDir != "" {
		// #nosec G301 - recording directory needs group read for monitoring
		if err := os.MkdirAll(m.cfg.LocalRecordDir, 0750); err != nil {
			return fmt.Errorf("failed to create recording directory %q: %w", m.cfg.LocalRecordDir, err)
		}
	}

	// Build command with context for automatic cancellation
	cmd := buildFFmpegCommand(ctx, m.cfg)

	// Redirect stderr to rotating log writer if configured.
	// Do NOT assign m.cmd yet: if cmd.Start() fails, m.cmd must remain nil so
	// that a concurrent stop() does not attempt to signal a process that was
	// never started (C-5 fix).
	m.mu.Lock()
	if m.logWriter != nil {
		cmd.Stderr = m.logWriter
	}
	m.startTime = time.Now()
	m.mu.Unlock()

	// Start process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Assign m.cmd only after a successful Start so that stop() never sees a
	// Process==nil pointer from an unstarted command.
	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()

	// Transition to running
	m.setState(StateRunning)

	// Start resource monitoring if configured
	if m.resourceMonitor != nil && cmd.Process != nil && m.cfg.MonitorInterval > 0 {
		monitorCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.monitorCancel = cancel
		m.mu.Unlock()

		go m.resourceMonitor.MonitorProcess(
			monitorCtx,
			cmd.Process.Pid,
			m.cfg.MonitorInterval,
			m.cfg.AlertCallback,
		)
	}

	// Wait for exit (or context cancellation)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Context cancelled - stop FFmpeg gracefully
		m.stopMonitoring()
		m.stop()
		<-done // Wait for process to exit
		return context.Canceled

	case err := <-done:
		// FFmpeg exited - stop monitoring
		m.stopMonitoring()
		m.mu.Lock()
		m.cmd = nil
		m.mu.Unlock()

		if err != nil {
			return fmt.Errorf("ffmpeg exited with error: %w", err)
		}
		return nil
	}
}

// stop stops the FFmpeg process gracefully.
func (m *Manager) stop() {
	m.setState(StateStopping)

	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		// Capture the process pointer before launching the goroutine to avoid
		// a race where cmd.Process becomes invalid after Wait() returns.
		proc := cmd.Process

		// Send SIGINT for graceful shutdown.
		// The error is intentionally discarded: if the process has already
		// exited between the nil check above and this signal call, the kernel
		// returns ESRCH, which is an expected benign race (L-7).
		_ = proc.Signal(os.Interrupt)

		// H-1 fix: Use configurable stop timeout (default 5s, was hardcoded 2s).
		// Audio codecs like Opus need time to flush their encoder buffer; 2s was
		// insufficient for clean shutdown of buffered audio data.
		stopTimeout := m.cfg.StopTimeout
		if stopTimeout <= 0 {
			stopTimeout = 5 * time.Second
		}

		// If FFmpeg does not exit within the timeout, force-kill it.
		// The goroutine is bound to a context derived from the caller's ctx so
		// it does not outlive the manager's Run() loop (ME-3 fix).
		killCtx, killCancel := context.WithTimeout(context.Background(), stopTimeout)
		go func() {
			defer killCancel()
			<-killCtx.Done()
			if killCtx.Err() == context.DeadlineExceeded {
				// Kill on an already-reaped process is a no-op; error ignored.
				_ = proc.Kill()
			}
		}()
	}
}

// stopMonitoring stops the resource monitor goroutine.
func (m *Manager) stopMonitoring() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.monitorCancel != nil {
		m.monitorCancel()
		m.monitorCancel = nil
	}
}

// forceStop immediately kills the FFmpeg process (for testing).
func (m *Manager) forceStop() error {
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return fmt.Errorf("no process to kill")
}

// buildFFmpegCommand constructs the FFmpeg command line.
//
// Command format (simple RTSP):
//
//	ffmpeg -f alsa -i hw:X,Y \
//	  -ar RATE -ac CHANNELS \
//	  -c:a CODEC -b:a BITRATE \
//	  [-thread_queue_size SIZE] \
//	  -reconnect 1 -reconnect_streamed 1 -reconnect_delay_max 30 \
//	  -f rtsp RTSP_URL
//
// Command format (tee muxer with local recording, C-1 fix):
//
//	ffmpeg -f alsa -i hw:X,Y \
//	  -ar RATE -ac CHANNELS \
//	  -c:a CODEC -b:a BITRATE \
//	  -f tee "[f=rtsp]rtsp://...|[f=segment:segment_time=3600]dir/stream_%Y%m%d_%H%M%S.wav"
//
// Parameters:
//   - cfg: Manager configuration
//
// Returns:
//   - *exec.Cmd: Configured FFmpeg command
func buildFFmpegCommand(ctx context.Context, cfg *ManagerConfig) *exec.Cmd {
	// Determine input format (default to alsa for backward compatibility)
	inputFormat := cfg.InputFormat
	if inputFormat == "" {
		inputFormat = "alsa"
	}

	args := []string{
		"-f", inputFormat,
		"-i", cfg.ALSADevice,
		"-ar", fmt.Sprintf("%d", cfg.SampleRate),
		"-ac", fmt.Sprintf("%d", cfg.Channels),
	}

	// Add thread queue if specified
	if cfg.ThreadQueue > 0 {
		args = append(args, "-thread_queue_size", fmt.Sprintf("%d", cfg.ThreadQueue))
	}

	// Add codec
	switch cfg.Codec {
	case "opus":
		args = append(args, "-c:a", "libopus")
	case "aac":
		args = append(args, "-c:a", "aac")
	}

	// Add bitrate
	args = append(args, "-b:a", cfg.Bitrate)

	// Determine output format (default to rtsp for backward compatibility)
	outputFormat := cfg.OutputFormat
	if outputFormat == "" {
		// Auto-detect from URL
		if strings.HasPrefix(cfg.RTSPURL, "rtsp://") {
			outputFormat = "rtsp"
		} else if cfg.RTSPURL == "-" || cfg.RTSPURL == "/dev/null" || strings.HasPrefix(cfg.RTSPURL, "pipe:") {
			// Use null format for stdout/pipe/devnull (testing)
			outputFormat = "null"
		} else if strings.Contains(cfg.RTSPURL, "/") {
			// File path - let FFmpeg auto-detect format from extension
			// Don't specify -f flag
			outputFormat = ""
		} else {
			// Default to rtsp for backward compatibility
			outputFormat = "rtsp"
		}
	}

	// C-1 fix: When LocalRecordDir is set and output is RTSP, use tee muxer
	// to simultaneously stream to RTSP and record locally as segmented files.
	// This prevents audio data loss during MediaMTX outages.
	if cfg.LocalRecordDir != "" && outputFormat == "rtsp" {
		segDuration := cfg.SegmentDuration
		if segDuration <= 0 {
			segDuration = 3600 // Default: 1-hour segments
		}
		segFormat := cfg.SegmentFormat
		if segFormat == "" {
			segFormat = "wav"
		}
		segPattern := filepath.Join(cfg.LocalRecordDir, cfg.StreamName+"_%Y%m%d_%H%M%S."+segFormat)

		// C-2 fix: Include reconnect flags in the RTSP output within the tee.
		teeOutput := fmt.Sprintf(
			"[f=rtsp:reconnect=1:reconnect_streamed=1:reconnect_delay_max=30]%s|[f=segment:segment_time=%d:strftime=1]%s",
			cfg.RTSPURL, segDuration, segPattern,
		)
		args = append(args, "-f", "tee", teeOutput)
	} else if outputFormat == "rtsp" {
		// C-2 fix: Add FFmpeg RTSP reconnection flags for non-tee RTSP output.
		// Without these, FFmpeg exits immediately when the RTSP connection drops
		// (e.g., MediaMTX restart), forcing a full process restart with backoff delay.
		args = append(args,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "30",
			"-f", outputFormat, cfg.RTSPURL,
		)
	} else if outputFormat != "" {
		args = append(args, "-f", outputFormat, cfg.RTSPURL)
	} else {
		// No format specified - let FFmpeg auto-detect
		args = append(args, cfg.RTSPURL)
	}

	// #nosec G204 - FFmpegPath is from validated configuration, not user input
	cmd := exec.CommandContext(ctx, cfg.FFmpegPath, args...)

	return cmd
}

// validateConfig validates manager configuration.
func validateConfig(cfg *ManagerConfig) error {
	if cfg.DeviceName == "" {
		return fmt.Errorf("device name cannot be empty")
	}
	if cfg.ALSADevice == "" {
		return fmt.Errorf("ALSA device cannot be empty")
	}
	if cfg.StreamName == "" {
		return fmt.Errorf("stream name cannot be empty")
	}
	if cfg.SampleRate <= 0 {
		return fmt.Errorf("sample rate must be positive")
	}
	if cfg.Channels <= 0 || cfg.Channels > 32 {
		return fmt.Errorf("channels must be between 1 and 32")
	}
	if cfg.Bitrate == "" {
		return fmt.Errorf("bitrate cannot be empty")
	}
	if cfg.Codec != "opus" && cfg.Codec != "aac" {
		return fmt.Errorf("codec must be opus or aac")
	}
	if cfg.RTSPURL == "" {
		return fmt.Errorf("RTSP URL cannot be empty")
	}
	if cfg.LockDir == "" {
		return fmt.Errorf("lock directory cannot be empty")
	}
	if cfg.FFmpegPath == "" {
		return fmt.Errorf("FFmpeg path cannot be empty")
	}
	if cfg.Backoff == nil {
		return fmt.Errorf("backoff policy cannot be nil")
	}
	return nil
}
