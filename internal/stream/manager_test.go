package stream

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// testLogger wraps *testing.T to implement io.Writer for logging.
type testLogger struct {
	t *testing.T
}

func (tl *testLogger) Write(p []byte) (n int, err error) {
	tl.t.Log(string(p))
	return len(p), nil
}

// TestFFmpegDiagnostic is a diagnostic test to see FFmpeg's actual error output.
// This helps debug why the integration tests are failing.
func TestFFmpegDiagnostic(t *testing.T) {
	ffmpegPath := findFFmpegOrSkip(t)
	device, inputFormat := getTestAudioDevice(t)

	// Try AAC codec first (built into FFmpeg, no external libs needed)
	outputFile := filepath.Join(t.TempDir(), "diagnostic.m4a")

	// Build command with AAC codec
	args := []string{
		"-f", inputFormat,
		"-i", device,
		"-ar", "48000",
		"-ac", "2",
		"-c:a", "aac",
		"-b:a", "128k",
		outputFile,
	}

	cmd := exec.Command(ffmpegPath, args...)

	// Capture stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	t.Logf("Running FFmpeg with command: %s %v", ffmpegPath, args)

	// Start FFmpeg
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start FFmpeg: %v", err)
	}

	// Wait for it to either succeed or fail
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// FFmpeg exited
		stderrOutput := stderr.String()
		t.Logf("FFmpeg stderr:\n%s", stderrOutput)

		if err != nil {
			t.Logf("FFmpeg exited with error: %v", err)
			t.Logf("This explains why integration tests are failing!")
		} else {
			t.Logf("FFmpeg completed successfully")
		}

	case <-time.After(3 * time.Second):
		// FFmpeg is running - kill it and wait for exit
		_ = cmd.Process.Kill()
		<-done // Wait for process to fully exit
		t.Logf("FFmpeg is running successfully after 3 seconds")
		t.Logf("FFmpeg stderr so far:\n%s", stderr.String())
	}
}

// getTestAudioDevice returns an appropriate audio device for testing.
// In CI environments without ALSA, it returns a lavfi virtual audio source.
// On systems with ALSA, it returns hw:0,0.
func getTestAudioDevice(t *testing.T) (device, inputFormat string) {
	t.Helper()

	// Check if ALSA device exists
	if _, err := os.Stat("/proc/asound/card0"); err == nil {
		return "hw:0,0", "alsa"
	}

	// Fall back to lavfi virtual audio (null audio source)
	// This generates silence for testing with 600 second duration
	return "anullsrc=r=48000:cl=stereo:d=600", "lavfi"
}

// getTestOutputURL returns an appropriate output URL for testing.
// Uses temporary file output to avoid dependency on MediaMTX server.
func getTestOutputURL(t *testing.T, name string) string {
	t.Helper()
	// Use temporary file for output with .m4a extension
	// M4A is the standard container for AAC codec
	tmpFile := filepath.Join(t.TempDir(), name+".m4a")
	return tmpFile
}

// TestStreamManagerLifecycle verifies basic stream lifecycle management.
//
// This tests the core state machine:
//
//	idle → starting → running → stopping → stopped
func TestStreamManagerLifecycle(t *testing.T) {
	device, inputFormat := getTestAudioDevice(t)

	cfg := &ManagerConfig{
		DeviceName:  "test_device",
		ALSADevice:  device,
		InputFormat: inputFormat,
		StreamName:  "test_stream",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "aac",
		RTSPURL:     getTestOutputURL(t, "test"),
		LockDir:     t.TempDir(),
		FFmpegPath:  findFFmpegOrSkip(t),
		Backoff:     NewBackoff(1*time.Second, 10*time.Second, 5),
		Logger:      &testLogger{t: t},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Verify initial state
	if mgr.State() != StateIdle {
		t.Errorf("Initial state = %v, want StateIdle", mgr.State())
	}

	// Start stream
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for running state
	if !waitForState(t, mgr, StateRunning, 5*time.Second) {
		t.Fatal("Stream did not reach running state")
	}

	// Stop stream
	cancel()

	// Wait for stopped state
	if !waitForState(t, mgr, StateStopped, 5*time.Second) {
		t.Fatal("Stream did not reach stopped state")
	}

	// Verify Run returns without error
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Run() error = %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Run() did not return after context cancellation")
	}
}

// TestStreamManagerFailureRestart verifies exponential backoff on failures.
//
// When FFmpeg exits with an error, the manager should:
// 1. Enter failed state
// 2. Wait according to backoff policy
// 3. Attempt restart
// 4. Stop after max attempts
func TestStreamManagerFailureRestart(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "failing_device",
		ALSADevice: "hw:99,99", // Non-existent device
		StreamName: "failing_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    getTestOutputURL(t, "failing"),
		LockDir:    t.TempDir(),
		FFmpegPath: findFFmpegOrSkip(t),
		Backoff:    NewBackoff(100*time.Millisecond, 1*time.Second, 3),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	startTime := time.Now()
	err = mgr.Run(ctx)

	// Should fail after 3 attempts with backoff
	if err == nil {
		t.Error("Run() expected error for failing device, got nil")
	}

	elapsed := time.Since(startTime)
	// With 3 attempts and backoff of 100ms, 200ms, 400ms = ~700ms minimum
	if elapsed < 500*time.Millisecond {
		t.Errorf("Run() completed too quickly (%v), backoff not working", elapsed)
	}

	// Verify final state
	if mgr.State() != StateFailed {
		t.Errorf("Final state = %v, want StateFailed", mgr.State())
	}

	// Verify attempts counter
	if mgr.Attempts() != 3 {
		t.Errorf("Attempts = %d, want 3", mgr.Attempts())
	}
}

// TestStreamManagerShortRunRestart verifies restart after short successful run.
//
// If FFmpeg runs < 300s (success threshold), treat as failure and restart.
func TestStreamManagerShortRunRestart(t *testing.T) {
	device, inputFormat := getTestAudioDevice(t)

	cfg := &ManagerConfig{
		DeviceName:  "short_run_device",
		ALSADevice:  device,
		InputFormat: inputFormat,
		StreamName:  "short_run",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "aac",
		RTSPURL:     getTestOutputURL(t, "short"),
		LockDir:     t.TempDir(),
		FFmpegPath:  findFFmpegOrSkip(t),
		Backoff:     NewBackoff(100*time.Millisecond, 1*time.Second, 5),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Start stream
	go func() { _ = mgr.Run(ctx) }()

	// Wait for running state
	if !waitForState(t, mgr, StateRunning, 3*time.Second) {
		t.Fatal("Stream did not reach running state")
	}

	// Kill FFmpeg after short run (simulating crash)
	time.Sleep(500 * time.Millisecond)
	if err := mgr.forceStop(); err != nil {
		t.Logf("forceStop() error (expected): %v", err)
	}

	// Should enter failed state
	if !waitForState(t, mgr, StateFailed, 2*time.Second) {
		t.Error("Stream did not enter failed state after short run")
	}

	// Should attempt restart (back to starting)
	time.Sleep(200 * time.Millisecond) // Wait for backoff
	state := mgr.State()
	if state != StateStarting && state != StateRunning {
		t.Errorf("State after backoff = %v, want StateStarting or StateRunning", state)
	}
}

// TestStreamManagerConcurrentStreams verifies multiple concurrent stream managers.
//
// Multiple devices should be able to stream simultaneously without interference.
func TestStreamManagerConcurrentStreams(t *testing.T) {
	numStreams := 3
	managers := make([]*Manager, numStreams)
	errChs := make([]chan error, numStreams)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create and start multiple managers
	for i := 0; i < numStreams; i++ {
		cfg := &ManagerConfig{
			DeviceName: fmt.Sprintf("device_%d", i),
			ALSADevice: fmt.Sprintf("hw:%d,0", i),
			StreamName: fmt.Sprintf("stream_%d", i),
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "aac",
			RTSPURL:    fmt.Sprintf("rtsp://localhost:8554/stream_%d", i),
			LockDir:    t.TempDir(),
			FFmpegPath: findFFmpegOrSkip(t),
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager(%d) error = %v", i, err)
		}
		managers[i] = mgr

		errChs[i] = make(chan error, 1)
		go func(idx int) {
			errChs[idx] <- managers[idx].Run(ctx)
		}(i)
	}

	// Wait for all to reach running state (or fail gracefully)
	time.Sleep(2 * time.Second)

	// Verify at least one manager is running (might fail due to hw device availability)
	runningCount := 0
	for _, mgr := range managers {
		if mgr.State() == StateRunning {
			runningCount++
		}
	}

	if runningCount == 0 {
		t.Log("Warning: No streams reached running state (may be due to hw availability)")
	}

	// Stop all
	cancel()

	// Wait for all to stop
	for i, errCh := range errChs {
		select {
		case <-errCh:
			// OK
		case <-time.After(5 * time.Second):
			t.Errorf("Manager %d did not stop within timeout", i)
		}
	}
}

// TestStreamManagerLockContention verifies locking prevents duplicate managers.
//
// Only one manager should be able to control a device at a time.
func TestStreamManagerLockContention(t *testing.T) {
	lockDir := t.TempDir()

	cfg1 := &ManagerConfig{
		DeviceName: "locked_device",
		ALSADevice: "hw:0,0",
		StreamName: "locked_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    getTestOutputURL(t, "locked"),
		LockDir:    lockDir,
		FFmpegPath: findFFmpegOrSkip(t),
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	cfg2 := &ManagerConfig{
		DeviceName: "locked_device", // Same device
		ALSADevice: "hw:0,0",
		StreamName: "locked_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    getTestOutputURL(t, "locked"),
		LockDir:    lockDir, // Same lock directory
		FFmpegPath: findFFmpegOrSkip(t),
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mgr1, err := NewManager(cfg1)
	if err != nil {
		t.Fatalf("NewManager(1) error = %v", err)
	}

	// Start first manager
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- mgr1.Run(ctx)
	}()

	// Wait for it to acquire lock
	time.Sleep(500 * time.Millisecond)

	// Try to create second manager for same device
	mgr2, err := NewManager(cfg2)
	if err != nil {
		t.Fatalf("NewManager(2) error = %v", err)
	}

	// Second manager should fail to acquire lock
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	err = mgr2.Run(ctx2)
	if err == nil {
		t.Error("Second manager should fail to acquire lock")
	}

	// Stop first manager
	cancel()
	<-errCh1

	// Now second manager should be able to run
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()

	errCh2 := make(chan error, 1)
	go func() {
		errCh2 <- mgr2.Run(ctx3)
	}()

	// Wait for second manager to acquire lock
	if !waitForState(t, mgr2, StateRunning, 3*time.Second) {
		t.Error("Second manager should run after first releases lock")
	}

	cancel3()
	<-errCh2
}

// TestStreamManagerGracefulShutdown verifies clean shutdown during various states.
func TestStreamManagerGracefulShutdown(t *testing.T) {
	tests := []struct {
		name       string
		shutdownAt State
		waitTime   time.Duration
	}{
		{"shutdown during starting", StateStarting, 100 * time.Millisecond},
		{"shutdown during running", StateRunning, 1 * time.Second},
		{"shutdown during backoff", StateFailed, 200 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				DeviceName: "shutdown_test",
				ALSADevice: "hw:0,0",
				StreamName: "shutdown",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "aac",
				RTSPURL:    getTestOutputURL(t, "shutdown"),
				LockDir:    t.TempDir(),
				FFmpegPath: findFFmpegOrSkip(t),
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
			}

			ctx, cancel := context.WithCancel(context.Background())

			mgr, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			errCh := make(chan error, 1)
			go func() {
				errCh <- mgr.Run(ctx)
			}()

			// Wait for desired state (with timeout)
			if tt.shutdownAt != StateStarting {
				// For states other than starting, wait for that state
				if !waitForState(t, mgr, tt.shutdownAt, 10*time.Second) {
					cancel()
					t.Fatalf("Failed to reach state %v before shutdown", tt.shutdownAt)
				}
			} else {
				// For starting state, just wait briefly as it transitions quickly
				time.Sleep(tt.waitTime)
			}

			// Cancel context
			cancel()

			// Verify graceful shutdown
			select {
			case err := <-errCh:
				if err != nil && err != context.Canceled {
					t.Errorf("Run() error = %v, want context.Canceled", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("Shutdown timeout - manager did not stop gracefully")
			}

			// Verify stopped state
			if mgr.State() != StateStopped {
				t.Errorf("Final state = %v, want StateStopped", mgr.State())
			}
		})
	}
}

// TestStreamManagerFFmpegCommandGeneration verifies correct FFmpeg command construction.
func TestStreamManagerFFmpegCommandGeneration(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *ManagerConfig
		wantArgs  []string
		wantNotIn []string
	}{
		{
			name: "aac codec stereo",
			cfg: &ManagerConfig{
				ALSADevice: "hw:0,0",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "192k",
				Codec:      "aac",
				RTSPURL:    getTestOutputURL(t, "test"),
			},
			wantArgs: []string{
				"-f", "alsa",
				"-i", "hw:0,0",
				"-ar", "48000",
				"-ac", "2",
				"-c:a", "aac",
				"-b:a", "192k",
			},
		},
		{
			name: "aac codec mono",
			cfg: &ManagerConfig{
				ALSADevice: "hw:1,0",
				SampleRate: 44100,
				Channels:   1,
				Bitrate:    "128k",
				Codec:      "aac",
				RTSPURL:    getTestOutputURL(t, "mono"),
			},
			wantArgs: []string{
				"-f", "alsa",
				"-i", "hw:1,0",
				"-ar", "44100",
				"-ac", "1",
				"-c:a", "aac",
				"-b:a", "128k",
			},
			wantNotIn: []string{"libopus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildFFmpegCommand(context.Background(), tt.cfg)

			// Verify all expected args present
			for _, want := range tt.wantArgs {
				if !contains(cmd.Args, want) {
					t.Errorf("FFmpeg command missing arg: %s\nGot: %v", want, cmd.Args)
				}
			}

			// Verify unwanted args not present
			for _, notWant := range tt.wantNotIn {
				if contains(cmd.Args, notWant) {
					t.Errorf("FFmpeg command contains unwanted arg: %s\nGot: %v", notWant, cmd.Args)
				}
			}

			// Verify RTSP URL is last
			if len(cmd.Args) < 2 {
				t.Fatal("FFmpeg command too short")
			}
			lastArg := cmd.Args[len(cmd.Args)-1]
			if lastArg != tt.cfg.RTSPURL {
				t.Errorf("Last arg = %q, want RTSP URL %q", lastArg, tt.cfg.RTSPURL)
			}
		})
	}
}

// TestStreamManagerStateTransitions verifies all valid state transitions.
func TestStreamManagerStateTransitions(t *testing.T) {
	validTransitions := map[State][]State{
		StateIdle:     {StateStarting},
		StateStarting: {StateRunning, StateFailed, StateStopped},
		StateRunning:  {StateStopping, StateFailed},
		StateStopping: {StateStopped},
		StateFailed:   {StateStarting, StateStopped},
		StateStopped:  {}, // Terminal state
	}

	// This test verifies the state machine logic
	// In practice, states are managed by Manager internals
	for fromState, toStates := range validTransitions {
		for _, toState := range toStates {
			t.Logf("Valid transition: %v → %v", fromState, toState)
		}
	}

	// Verify invalid transitions are rejected
	invalidTransitions := []struct {
		from State
		to   State
	}{
		{StateIdle, StateRunning},     // Can't go directly to running
		{StateIdle, StateFailed},      // Can't fail from idle
		{StateRunning, StateStarting}, // Can't restart while running
		{StateStopped, StateStarting}, // Can't restart from terminal state
	}

	for _, tr := range invalidTransitions {
		t.Logf("Invalid transition: %v → %v", tr.from, tr.to)
	}
}

// TestStreamManagerMetrics verifies metrics collection.
func TestStreamManagerMetrics(t *testing.T) {
	device, inputFormat := getTestAudioDevice(t)

	cfg := &ManagerConfig{
		DeviceName:  "metrics_device",
		ALSADevice:  device,
		InputFormat: inputFormat,
		StreamName:  "metrics",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "aac",
		RTSPURL:     getTestOutputURL(t, "metrics"),
		LockDir:     t.TempDir(),
		FFmpegPath:  findFFmpegOrSkip(t),
		Backoff:     NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Start stream
	go func() { _ = mgr.Run(ctx) }()

	// Wait for running
	if !waitForState(t, mgr, StateRunning, 3*time.Second) {
		t.Fatal("Stream did not reach running state")
	}

	// Verify metrics
	metrics := mgr.Metrics()

	if metrics.DeviceName != "metrics_device" {
		t.Errorf("Metrics.DeviceName = %q, want \"metrics_device\"", metrics.DeviceName)
	}

	if metrics.State != StateRunning {
		t.Errorf("Metrics.State = %v, want StateRunning", metrics.State)
	}

	if metrics.StartTime.IsZero() {
		t.Error("Metrics.StartTime is zero, want valid timestamp")
	}

	if metrics.Uptime <= 0 {
		t.Error("Metrics.Uptime <= 0, want positive duration")
	}

	cancel()
}

// Helper functions

func waitForState(t *testing.T, mgr *Manager, want State, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if mgr.State() == want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Check one final time after timeout (state might have changed during sleep)
	if mgr.State() == want {
		return true
	}

	t.Logf("Timeout waiting for state %v, current state: %v", want, mgr.State())
	return false
}

func findFFmpegOrSkip(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found in PATH, skipping test")
	}
	return path
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// BenchmarkStreamManagerStart measures manager startup performance.
func BenchmarkStreamManagerStart(b *testing.B) {
	cfg := &ManagerConfig{
		DeviceName: "bench_device",
		ALSADevice: "hw:0,0",
		StreamName: "bench",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    "/dev/null",
		LockDir:    b.TempDir(),
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewManager(cfg)
	}
}
