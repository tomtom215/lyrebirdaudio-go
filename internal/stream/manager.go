package stream

import (
	"context"
	"fmt"
	"io"
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
	DeviceName   string    // Sanitized device name (e.g., "blue_yeti")
	ALSADevice   string    // ALSA device identifier (e.g., "hw:0,0") or lavfi source
	InputFormat  string    // Input format: "alsa" or "lavfi" (default: "alsa")
	StreamName   string    // Stream name for MediaMTX path
	SampleRate   int       // Sample rate in Hz
	Channels     int       // Number of channels
	Bitrate      string    // Bitrate (e.g., "128k")
	Codec        string    // Codec ("opus" or "aac")
	ThreadQueue  int       // FFmpeg thread queue size (optional)
	RTSPURL      string    // Full RTSP URL or file path for output
	OutputFormat string    // Output format: "rtsp", "null", or empty for auto-detect (default: "rtsp")
	LockDir      string    // Directory for lock files
	FFmpegPath   string    // Path to ffmpeg binary
	Backoff      *Backoff  // Backoff policy for restarts
	Logger       io.Writer // Optional logger for debug output (nil = no logging)
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
	mu      sync.RWMutex // Protects cmd, lock, startTime
	cmd     *exec.Cmd
	lock    *lock.FileLock
	backoff *Backoff

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

	return mgr, nil
}

// logf writes a formatted log message if Logger is configured.
func (m *Manager) logf(format string, args ...interface{}) {
	if m.cfg.Logger != nil {
		fmt.Fprintf(m.cfg.Logger, "[Manager %s] "+format+"\n", append([]interface{}{m.cfg.DeviceName}, args...)...)
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
	// Acquire lock
	if err := m.acquireLock(); err != nil {
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
			if err == context.Canceled {
				m.logf("Context cancelled, stopping")
				m.setState(StateStopped)
				return err
			}

			// FFmpeg failed
			m.failures.Add(1)
			m.setState(StateFailed)
			m.backoff.RecordFailure()
			m.logf("FFmpeg failed: %v (failures=%d, backoff=%v)", err, m.failures.Load(), m.backoff.CurrentDelay())

			// Wait for backoff (or context cancellation)
			if err := m.backoff.WaitContext(ctx); err != nil {
				m.setState(StateStopped)
				return err // Context cancelled during backoff
			}

			// Continue to retry
			continue
		}

		// FFmpeg exited cleanly - check if it was a "successful" run
		if runTime < 300*time.Second {
			// Short run - treat as failure
			m.failures.Add(1)
			m.backoff.RecordSuccess(runTime)

			m.setState(StateFailed)
			m.logf("FFmpeg ran for %v (< 300s threshold), treating as failure", runTime)

			// Wait for backoff
			if err := m.backoff.WaitContext(ctx); err != nil {
				m.setState(StateStopped)
				return err
			}

			// Continue to retry
			continue
		}

		m.logf("FFmpeg ran successfully for %v", runTime)

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
func (m *Manager) State() State {
	return m.state.Load().(State)
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
func (m *Manager) Metrics() Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var uptime time.Duration
	if !m.startTime.IsZero() {
		uptime = time.Since(m.startTime)
	}

	return Metrics{
		DeviceName: m.cfg.DeviceName,
		StreamName: m.cfg.StreamName,
		State:      m.State(),
		StartTime:  m.startTime,
		Uptime:     uptime,
		Attempts:   m.Attempts(),
		Failures:   m.Failures(),
	}
}

// setState atomically updates the manager state.
func (m *Manager) setState(s State) {
	m.state.Store(s)
}

// acquireLock acquires the file lock for this device.
func (m *Manager) acquireLock() error {
	lockPath := filepath.Join(m.cfg.LockDir, m.cfg.DeviceName+".lock")
	fl, err := lock.NewFileLock(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create lock: %w", err)
	}

	// Try to acquire lock with timeout
	if err := fl.Acquire(30 * time.Second); err != nil {
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
		_ = m.lock.Release()
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
	// Build command with context for automatic cancellation
	cmd := buildFFmpegCommand(ctx, m.cfg)

	m.mu.Lock()
	m.cmd = cmd
	m.startTime = time.Now()
	m.mu.Unlock()

	// Start process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Transition to running
	m.setState(StateRunning)

	// Wait for exit (or context cancellation)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Context cancelled - stop FFmpeg gracefully
		m.stop()
		<-done // Wait for process to exit
		return context.Canceled

	case err := <-done:
		// FFmpeg exited
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
		// Send SIGINT for graceful shutdown
		_ = cmd.Process.Signal(os.Interrupt)

		// Wait up to 5 seconds for graceful shutdown
		time.Sleep(5 * time.Second)

		// Force kill if still running
		_ = cmd.Process.Kill()
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
// Command format:
//
//	ffmpeg -f alsa -i hw:X,Y \
//	  -ar RATE -ac CHANNELS \
//	  -c:a CODEC -b:a BITRATE \
//	  [-thread_queue_size SIZE] \
//	  -f [rtsp|null] RTSP_URL
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

	// Output format and URL
	if outputFormat != "" {
		args = append(args, "-f", outputFormat, cfg.RTSPURL)
	} else {
		// No format specified - let FFmpeg auto-detect
		args = append(args, cfg.RTSPURL)
	}

	// #nosec G204 - FFmpegPath is from validated configuration, not user input
	cmd := exec.CommandContext(ctx, cfg.FFmpegPath, args...)

	// Redirect stderr for logging (optional)
	// cmd.Stderr = os.Stderr

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
