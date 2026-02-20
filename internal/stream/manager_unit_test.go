package stream

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestValidateConfig verifies configuration validation.
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ManagerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: false,
		},
		{
			name: "empty device name",
			cfg: &ManagerConfig{
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "device name cannot be empty",
		},
		{
			name: "empty ALSA device",
			cfg: &ManagerConfig{
				DeviceName: "test",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "ALSA device cannot be empty",
		},
		{
			name: "invalid sample rate",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: -1,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "sample rate must be positive",
		},
		{
			name: "invalid channels - zero",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   0,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "channels must be between 1 and 32",
		},
		{
			name: "invalid channels - too many",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   33,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "channels must be between 1 and 32",
		},
		{
			name: "invalid codec",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "mp3",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "codec must be opus or aac",
		},
		{
			name: "missing backoff",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    nil,
			},
			wantErr: true,
			errMsg:  "backoff policy cannot be nil",
		},
		{
			name: "empty stream name",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "stream name cannot be empty",
		},
		{
			name: "empty bitrate",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "bitrate cannot be empty",
		},
		{
			name: "empty RTSP URL",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "RTSP URL cannot be empty",
		},
		{
			name: "empty lock directory",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "lock directory cannot be empty",
		},
		{
			name: "empty FFmpeg path",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "FFmpeg path cannot be empty",
		},
		{
			name: "aac codec valid",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "aac",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("validateConfig() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("validateConfig() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateConfig() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestNewManager verifies manager creation with validation.
func TestNewManager(t *testing.T) {
	validCfg := &ManagerConfig{
		DeviceName: "test",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    "/tmp",
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(validCfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if mgr == nil {
		t.Fatal("NewManager() returned nil manager")
	}

	if mgr.State() != StateIdle {
		t.Errorf("Initial state = %v, want StateIdle", mgr.State())
	}

	if mgr.Attempts() != 0 {
		t.Errorf("Initial attempts = %d, want 0", mgr.Attempts())
	}

	if mgr.Failures() != 0 {
		t.Errorf("Initial failures = %d, want 0", mgr.Failures())
	}
}

// TestStateString verifies State.String() method.
func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateIdle, "idle"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopping, "stopping"},
		{StateFailed, "failed"},
		{StateStopped, "stopped"},
		{State(999), "unknown(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("State.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestManagerSetState verifies state transitions.
func TestManagerSetState(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    "/tmp",
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Test state transitions
	states := []State{StateStarting, StateRunning, StateFailed, StateStopped}
	for _, state := range states {
		mgr.setState(state)
		if mgr.State() != state {
			t.Errorf("setState(%v) failed, got %v", state, mgr.State())
		}
	}
}

// TestBuildFFmpegCommandThreadQueue verifies thread queue handling.
func TestBuildFFmpegCommandThreadQueue(t *testing.T) {
	tests := []struct {
		name        string
		threadQueue int
		wantArg     bool
	}{
		{"with thread queue", 8192, true},
		{"without thread queue", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:  "hw:0,0",
				SampleRate:  48000,
				Channels:    2,
				Bitrate:     "128k",
				Codec:       "opus",
				ThreadQueue: tt.threadQueue,
				RTSPURL:     "rtsp://localhost:8554/test",
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			hasThreadQueue := false
			for _, arg := range cmd.Args {
				if arg == "-thread_queue_size" {
					hasThreadQueue = true
					break
				}
			}

			if hasThreadQueue != tt.wantArg {
				t.Errorf("thread_queue_size in args = %v, want %v", hasThreadQueue, tt.wantArg)
			}
		})
	}
}

// TestBuildFFmpegCommandInputFormat verifies input format handling.
func TestBuildFFmpegCommandInputFormat(t *testing.T) {
	tests := []struct {
		name        string
		inputFormat string
		wantFormat  string
	}{
		{"alsa format", "alsa", "alsa"},
		{"lavfi format", "lavfi", "lavfi"},
		{"empty defaults to alsa", "", "alsa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:  "hw:0,0",
				InputFormat: tt.inputFormat,
				SampleRate:  48000,
				Channels:    2,
				Bitrate:     "128k",
				Codec:       "opus",
				RTSPURL:     "rtsp://localhost:8554/test",
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			// Find -f flag
			foundFormat := false
			for i, arg := range cmd.Args {
				if arg == "-f" && i+1 < len(cmd.Args) {
					if cmd.Args[i+1] == tt.wantFormat {
						foundFormat = true
					} else {
						t.Errorf("input format = %q, want %q", cmd.Args[i+1], tt.wantFormat)
					}
					break
				}
			}

			if !foundFormat {
				t.Errorf("input format %q not found in command", tt.wantFormat)
			}
		})
	}
}

// TestBuildFFmpegCommandOutputFormat verifies output format handling.
func TestBuildFFmpegCommandOutputFormat(t *testing.T) {
	tests := []struct {
		name         string
		rtspURL      string
		outputFormat string
		wantFormat   string
	}{
		{"rtsp URL auto-detect", "rtsp://localhost:8554/test", "", "rtsp"},
		{"pipe URL auto-detect", "pipe:1", "", "null"},
		{"stdout auto-detect", "-", "", "null"},
		{"devnull auto-detect", "/dev/null", "", "null"},
		{"explicit rtsp format", "rtsp://localhost:8554/test", "rtsp", "rtsp"},
		{"explicit null format", "/dev/null", "null", "null"},
		{"file path auto-detect", "/tmp/test.ogg", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:   "hw:0,0",
				SampleRate:   48000,
				Channels:     2,
				Bitrate:      "128k",
				Codec:        "opus",
				RTSPURL:      tt.rtspURL,
				OutputFormat: tt.outputFormat,
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			if tt.wantFormat == "" {
				// Verify no -f flag before the URL (auto-detect)
				// The URL should be the last argument
				if cmd.Args[len(cmd.Args)-1] != tt.rtspURL {
					t.Errorf("expected URL %q as last arg, got %q", tt.rtspURL, cmd.Args[len(cmd.Args)-1])
				}
				// Verify no -f immediately before URL
				if len(cmd.Args) >= 2 && cmd.Args[len(cmd.Args)-2] == "-f" {
					t.Error("expected no -f flag for file path (auto-detect)")
				}
			} else {
				// Find the output format in args
				foundFormat := false
				for i := len(cmd.Args) - 3; i >= 0; i-- {
					if cmd.Args[i] == "-f" && i+1 < len(cmd.Args) {
						// Check if this is the output format (not input format)
						if i+2 < len(cmd.Args) && cmd.Args[i+2] == tt.rtspURL {
							if cmd.Args[i+1] == tt.wantFormat {
								foundFormat = true
							} else {
								t.Errorf("output format = %q, want %q", cmd.Args[i+1], tt.wantFormat)
							}
							break
						}
					}
				}

				if !foundFormat {
					t.Errorf("output format %q not found in command", tt.wantFormat)
				}
			}
		})
	}
}

// TestManagerMetricsInitialState verifies initial metrics state.
func TestManagerMetricsInitialState(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test_device",
		ALSADevice: "hw:0,0",
		StreamName: "test_stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    "/tmp",
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	metrics := mgr.Metrics()

	if metrics.DeviceName != "test_device" {
		t.Errorf("Metrics.DeviceName = %q, want \"test_device\"", metrics.DeviceName)
	}

	if metrics.StreamName != "test_stream" {
		t.Errorf("Metrics.StreamName = %q, want \"test_stream\"", metrics.StreamName)
	}

	if metrics.State != StateIdle {
		t.Errorf("Metrics.State = %v, want StateIdle", metrics.State)
	}

	if !metrics.StartTime.IsZero() {
		t.Error("Metrics.StartTime should be zero initially")
	}

	if metrics.Uptime != 0 {
		t.Errorf("Metrics.Uptime = %v, want 0", metrics.Uptime)
	}

	if metrics.Attempts != 0 {
		t.Errorf("Metrics.Attempts = %d, want 0", metrics.Attempts)
	}

	if metrics.Failures != 0 {
		t.Errorf("Metrics.Failures = %d, want 0", metrics.Failures)
	}
}

// TestManagerLogf verifies logging functionality.
func TestManagerLogf(t *testing.T) {
	tests := []struct {
		name        string
		hasLogger   bool
		format      string
		args        []interface{}
		wantContain string // Structured log contains the message
		wantEmpty   bool
	}{
		{
			name:        "with logger",
			hasLogger:   true,
			format:      "test message %d",
			args:        []interface{}{42},
			wantContain: "test message 42",
		},
		{
			name:        "with logger no args",
			hasLogger:   true,
			format:      "simple message",
			args:        []interface{}{},
			wantContain: "simple message",
		},
		{
			name:      "without logger",
			hasLogger: false,
			format:    "test message",
			args:      []interface{}{},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture log output
			var buf bytes.Buffer

			cfg := &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			}

			if tt.hasLogger {
				cfg.Logger = slog.New(slog.NewTextHandler(&buf, nil))
			}

			mgr, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			mgr.logf(tt.format, tt.args...)

			got := buf.String()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("logf() output = %q, want empty", got)
				}
			} else if !strings.Contains(got, tt.wantContain) {
				t.Errorf("logf() output = %q, want it to contain %q", got, tt.wantContain)
			}
		})
	}
}

// TestManagerAcquireLock verifies lock acquisition and release.
func TestManagerAcquireLock(t *testing.T) {
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName: "test_lock",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    lockDir,
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Test acquire lock
	err = mgr.acquireLock(context.Background())
	if err != nil {
		t.Fatalf("acquireLock() error = %v", err)
	}

	// Verify lock file exists
	lockPath := filepath.Join(lockDir, "test_lock.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Errorf("Lock file was not created at %s", lockPath)
	}

	// Test release lock
	mgr.releaseLock()

	// Verify lock is released (file may still exist but should be unlockable)
	// We can verify by trying to acquire again
	err = mgr.acquireLock(context.Background())
	if err != nil {
		t.Errorf("Failed to re-acquire lock after release: %v", err)
	}
	mgr.releaseLock()
}

// TestManagerStop verifies stop behavior.
func TestManagerStop(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test_stop",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    t.TempDir(),
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Call stop() when no process is running - should not panic
	mgr.stop()

	// Verify state changed to stopping
	if mgr.State() != StateStopping {
		t.Errorf("State after stop() = %v, want StateStopping", mgr.State())
	}
}

// TestManagerForceStop verifies forceStop behavior.
func TestManagerForceStop(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test_force_stop",
		ALSADevice: "hw:0,0",
		StreamName: "stream",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    t.TempDir(),
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Call forceStop() when no process is running - should return error
	err = mgr.forceStop()
	if err == nil {
		t.Error("forceStop() with no process should return error")
	}
}

// BenchmarkNewManager measures manager creation performance.
func BenchmarkNewManager(b *testing.B) {
	cfg := &ManagerConfig{
		DeviceName: "bench",
		ALSADevice: "hw:0,0",
		StreamName: "bench",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/bench",
		LockDir:    "/tmp",
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewManager(cfg)
	}
}

// BenchmarkBuildFFmpegCommand measures command building performance.
func BenchmarkBuildFFmpegCommand(b *testing.B) {
	cfg := &ManagerConfig{
		ALSADevice:  "hw:0,0",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 8192,
		RTSPURL:     "rtsp://localhost:8554/bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildFFmpegCommand(context.Background(), cfg)
	}
}

// ==========================================================================
// Production readiness fix tests
// ==========================================================================

// TestBuildFFmpegCommandReconnectFlags verifies C-2 fix: RTSP reconnect flags.
func TestBuildFFmpegCommandReconnectFlags(t *testing.T) {
	tests := []struct {
		name           string
		rtspURL        string
		outputFormat   string
		localRecordDir string
		wantReconnect  bool
	}{
		{
			name:          "rtsp auto-detect adds reconnect flags",
			rtspURL:       "rtsp://localhost:8554/test",
			wantReconnect: true,
		},
		{
			name:          "null format does not add reconnect flags",
			rtspURL:       "/dev/null",
			outputFormat:  "null",
			wantReconnect: false,
		},
		{
			name:          "file output does not add reconnect flags",
			rtspURL:       "/tmp/test.ogg",
			wantReconnect: false,
		},
		{
			name:           "tee muxer with local recording includes reconnect in tee options",
			rtspURL:        "rtsp://localhost:8554/test",
			localRecordDir: "/tmp/recordings",
			wantReconnect:  false, // reconnect is inside tee option string, not as top-level arg
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:     "hw:0,0",
				StreamName:     "test",
				SampleRate:     48000,
				Channels:       2,
				Bitrate:        "128k",
				Codec:          "opus",
				RTSPURL:        tt.rtspURL,
				OutputFormat:   tt.outputFormat,
				LocalRecordDir: tt.localRecordDir,
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			hasReconnect := false
			for _, arg := range cmd.Args {
				if arg == "-reconnect" {
					hasReconnect = true
					break
				}
			}

			if hasReconnect != tt.wantReconnect {
				t.Errorf("-reconnect flag present = %v, want %v\nArgs: %v",
					hasReconnect, tt.wantReconnect, cmd.Args)
			}
		})
	}
}

// TestBuildFFmpegCommandTeeMuxer verifies C-1 fix: tee muxer with local recording.
func TestBuildFFmpegCommandTeeMuxer(t *testing.T) {
	t.Run("tee muxer enabled with local record dir", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice:      "hw:0,0",
			StreamName:      "blue_yeti",
			SampleRate:      48000,
			Channels:        2,
			Bitrate:         "128k",
			Codec:           "opus",
			RTSPURL:         "rtsp://localhost:8554/blue_yeti",
			LocalRecordDir:  "/var/audio/recordings",
			SegmentDuration: 1800,
			SegmentFormat:   "flac",
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)

		// Should use -f tee
		hasTee := false
		for i, arg := range cmd.Args {
			if arg == "-f" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "tee" {
				hasTee = true
				break
			}
		}
		if !hasTee {
			t.Errorf("C-1: expected -f tee in args, got: %v", cmd.Args)
		}

		// Last arg should be the tee output string containing both RTSP and segment
		lastArg := cmd.Args[len(cmd.Args)-1]
		if !strings.Contains(lastArg, "[f=rtsp") {
			t.Errorf("C-1: tee output should contain [f=rtsp], got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "[f=segment") {
			t.Errorf("C-1: tee output should contain [f=segment], got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "segment_time=1800") {
			t.Errorf("C-1: tee output should contain segment_time=1800, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, ".flac") {
			t.Errorf("C-1: tee output should contain .flac extension, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "blue_yeti_") {
			t.Errorf("C-1: tee output should contain stream name, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "reconnect=1") {
			t.Errorf("C-2: tee RTSP output should contain reconnect options, got: %s", lastArg)
		}
	})

	t.Run("tee muxer defaults", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice:     "hw:0,0",
			StreamName:     "test",
			SampleRate:     48000,
			Channels:       2,
			Bitrate:        "128k",
			Codec:          "opus",
			RTSPURL:        "rtsp://localhost:8554/test",
			LocalRecordDir: "/tmp/recordings",
			// SegmentDuration and SegmentFormat unset - should use defaults
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)
		lastArg := cmd.Args[len(cmd.Args)-1]

		if !strings.Contains(lastArg, "segment_time=3600") {
			t.Errorf("C-1: default segment_time should be 3600, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, ".wav") {
			t.Errorf("C-1: default segment format should be wav, got: %s", lastArg)
		}
	})

	t.Run("no tee when local record dir empty", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice: "hw:0,0",
			StreamName: "test",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			// LocalRecordDir unset
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)

		for _, arg := range cmd.Args {
			if arg == "tee" {
				t.Error("C-1: should not use tee when LocalRecordDir is empty")
			}
		}
	})

	t.Run("no tee for non-rtsp output", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice:     "hw:0,0",
			StreamName:     "test",
			SampleRate:     48000,
			Channels:       2,
			Bitrate:        "128k",
			Codec:          "opus",
			RTSPURL:        "/dev/null",
			OutputFormat:   "null",
			LocalRecordDir: "/tmp/recordings",
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)

		for _, arg := range cmd.Args {
			if arg == "tee" {
				t.Error("C-1: should not use tee for null output format")
			}
		}
	})
}

// TestStopTimeoutConfigurable verifies H-1 fix: configurable stop timeout.
func TestStopTimeoutConfigurable(t *testing.T) {
	t.Run("default stop timeout is 5s", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName: "test",
			ALSADevice: "hw:0,0",
			StreamName: "stream",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    "/tmp",
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			// StopTimeout not set - should default to 5s
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		if mgr.cfg.StopTimeout != 0 {
			t.Errorf("default StopTimeout should be 0 (manager uses 5s default), got %v", mgr.cfg.StopTimeout)
		}
	})

	t.Run("custom stop timeout accepted", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName:  "test",
			ALSADevice:  "hw:0,0",
			StreamName:  "stream",
			SampleRate:  48000,
			Channels:    2,
			Bitrate:     "128k",
			Codec:       "opus",
			RTSPURL:     "rtsp://localhost:8554/test",
			LockDir:     "/tmp",
			FFmpegPath:  "/usr/bin/ffmpeg",
			Backoff:     NewBackoff(1*time.Second, 10*time.Second, 5),
			StopTimeout: 10 * time.Second,
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		if mgr.cfg.StopTimeout != 10*time.Second {
			t.Errorf("StopTimeout = %v, want 10s", mgr.cfg.StopTimeout)
		}
	})
}

// TestLogStructuredEvent verifies H-4 fix: structured failure event logging.
func TestLogStructuredEvent(t *testing.T) {
	t.Run("structured event logged with all fields", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := &ManagerConfig{
			DeviceName: "test_device",
			ALSADevice: "hw:0,0",
			StreamName: "test_stream",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    "/tmp",
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			Logger:     slog.New(slog.NewTextHandler(&buf, nil)),
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		mgr.logStructuredEvent("stream_failure",
			"error", "test error",
			"attempt", 3,
			"failures", 2,
		)

		output := buf.String()
		for _, want := range []string{"stream_event", "stream_failure", "test_device", "test_stream", "test error"} {
			if !strings.Contains(output, want) {
				t.Errorf("H-4: structured event should contain %q, got: %s", want, output)
			}
		}
	})

	t.Run("no panic without logger", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName: "test",
			ALSADevice: "hw:0,0",
			StreamName: "stream",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    "/tmp",
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			// Logger intentionally nil
		}

		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Should not panic
		mgr.logStructuredEvent("stream_failure", "error", "test")
	})
}

// TestManagerRunLockAcquisitionFailure verifies behavior when lock acquisition fails.
// With context-aware lock acquisition, this test completes quickly (< 1 second).
func TestManagerRunLockAcquisitionFailure(t *testing.T) {
	// Create a lock dir and pre-acquire a lock to force second acquisition to fail
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "1",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep",
		Backoff:      NewBackoff(100*time.Millisecond, 1*time.Second, 3),
	}

	// First manager acquires the lock
	mgr1, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(1) error = %v", err)
	}

	if err := mgr1.acquireLock(context.Background()); err != nil {
		t.Fatalf("First lock acquisition should succeed: %v", err)
	}
	defer mgr1.releaseLock()

	// Second manager should fail quickly when context is cancelled
	mgr2, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(2) error = %v", err)
	}

	// Use a cancelled context - lock acquisition should fail immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	start := time.Now()
	err = mgr2.Run(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run() should fail when context is cancelled")
	}

	// Should fail quickly (< 1 second) due to context cancellation
	if elapsed > 1*time.Second {
		t.Errorf("Lock acquisition took %v, expected < 1s with cancelled context", elapsed)
	}

	if !strings.Contains(err.Error(), "failed to acquire lock") {
		t.Errorf("Run() error = %q, want error containing 'failed to acquire lock'", err.Error())
	}

	// Should still be in idle state (never got past lock acquisition)
	state := mgr2.State()
	if state != StateIdle {
		t.Errorf("State after lock failure = %v, expected Idle", state)
	}
}

// TestManagerRunContextCancelledImmediately verifies graceful shutdown when context cancelled immediately.
// With context-aware lock acquisition, this fails during lock acquisition (before starting).
func TestManagerRunContextCancelledImmediately(t *testing.T) {
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "10", // Numeric argument for sleep
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep",
		Backoff:      NewBackoff(100*time.Millisecond, 1*time.Second, 3),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = mgr.Run(ctx)

	// Should fail during lock acquisition with wrapped context.Canceled error
	if err == nil {
		t.Fatal("Run() should fail with cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want error wrapping context.Canceled", err)
	}

	// Should remain in idle state (never got past lock acquisition)
	if mgr.State() != StateIdle {
		t.Errorf("State after immediate cancel = %v, want StateIdle", mgr.State())
	}
}

// TestManagerRunContextCancelledDuringRun verifies graceful shutdown during execution.
func TestManagerRunContextCancelledDuringRun(t *testing.T) {
	lockDir := t.TempDir()

	// Create a script that sleeps ignoring all arguments
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\nsleep 10\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy", // Arguments don't matter - script ignores them
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(100*time.Millisecond, 1*time.Second, 3),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for manager to start
	time.Sleep(200 * time.Millisecond)

	// Verify it's running
	if mgr.State() != StateRunning {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Errorf("State before cancel = %v, want StateRunning", mgr.State())
	}

	// Cancel context
	cancel()

	// Wait for Run to complete
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not complete within timeout")
	}

	// Should be in stopped state
	if mgr.State() != StateStopped {
		t.Errorf("State after cancel = %v, want StateStopped", mgr.State())
	}
}

// TestManagerRunMaxAttemptsExceeded verifies behavior when max restart attempts exceeded.
func TestManagerRunMaxAttemptsExceeded(t *testing.T) {
	lockDir := t.TempDir()

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy", // Argument doesn't matter for /bin/false
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/false", // Always fails immediately
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 3),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	err = mgr.Run(ctx)

	if err == nil {
		t.Fatal("Run() should fail when max attempts exceeded")
	}

	if !strings.Contains(err.Error(), "max restart attempts") {
		t.Errorf("Run() error = %q, want error containing 'max restart attempts'", err.Error())
	}

	// Should be in failed state
	if mgr.State() != StateFailed {
		t.Errorf("State after max attempts = %v, want StateFailed", mgr.State())
	}

	// Verify attempts counter
	if mgr.Attempts() < 3 {
		t.Errorf("Attempts = %d, want >= 3", mgr.Attempts())
	}

	// Verify failures counter
	if mgr.Failures() < 3 {
		t.Errorf("Failures = %d, want >= 3", mgr.Failures())
	}
}

// TestManagerRunShortRunTreatedAsFailure verifies FFmpeg runs < 300s are treated as failures.
func TestManagerRunShortRunTreatedAsFailure(t *testing.T) {
	lockDir := t.TempDir()

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "0.1", // Argument to sleep
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep", // Sleep for short duration
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 2),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = mgr.Run(ctx)

	// Should timeout or hit max attempts
	if err == nil {
		t.Fatal("Run() should fail for short runs")
	}

	// Verify failures were recorded
	if mgr.Failures() == 0 {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Error("Failures = 0, want > 0 for short runs")
	}
}

// TestManagerRunContextCancelledDuringBackoff verifies context cancellation during backoff wait.
func TestManagerRunContextCancelledDuringBackoff(t *testing.T) {
	lockDir := t.TempDir()

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy", // Argument doesn't matter for /bin/false
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/false",                                  // Always fails
		Backoff:      NewBackoff(5*time.Second, 10*time.Second, 10), // Long backoff
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for first failure and backoff to start
	time.Sleep(200 * time.Millisecond)

	// Verify state is failed (in backoff)
	if mgr.State() != StateFailed {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Errorf("State during backoff = %v, want StateFailed", mgr.State())
	}

	// Cancel during backoff
	cancel()

	// Should complete quickly (not wait full backoff)
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not complete quickly after cancel during backoff")
	}

	// Should be in stopped state
	if mgr.State() != StateStopped {
		t.Errorf("State after cancel during backoff = %v, want StateStopped", mgr.State())
	}
}

// TestManagerRunMetricsUpdate verifies metrics are updated correctly.
func TestManagerRunMetricsUpdate(t *testing.T) {
	lockDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "0.05", // Short sleep
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   "/bin/sleep",
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 3),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Initial metrics
	initialMetrics := mgr.Metrics()
	if initialMetrics.Attempts != 0 {
		t.Errorf("Initial attempts = %d, want 0", initialMetrics.Attempts)
	}
	if initialMetrics.Failures != 0 {
		t.Errorf("Initial failures = %d, want 0", initialMetrics.Failures)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = mgr.Run(ctx)

	// Verify metrics were updated
	finalMetrics := mgr.Metrics()
	if finalMetrics.Attempts == 0 {
		t.Error("Attempts should be > 0 after Run()")
	}
	if finalMetrics.Failures == 0 {
		t.Error("Failures should be > 0 after short runs")
	}

	// Verify device and stream names in metrics
	if finalMetrics.DeviceName != "test" {
		t.Errorf("Metrics.DeviceName = %q, want \"test\"", finalMetrics.DeviceName)
	}
	if finalMetrics.StreamName != "test" {
		t.Errorf("Metrics.StreamName = %q, want \"test\"", finalMetrics.StreamName)
	}
}

// TestManagerRunResourceCleanup verifies lock is released on all exit paths.
func TestManagerRunResourceCleanup(t *testing.T) {
	lockDir := t.TempDir()

	tests := []struct {
		name        string
		setupCtx    func() (context.Context, context.CancelFunc)
		ffmpegPath  string
		maxAttempts int
	}{
		{
			name: "immediate cancel",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			ffmpegPath:  "/bin/sleep",
			maxAttempts: 3,
		},
		{
			name: "cancel during run",
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 200*time.Millisecond)
			},
			ffmpegPath:  "/bin/sleep",
			maxAttempts: 10,
		},
		{
			name: "max attempts exceeded",
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 1*time.Second)
			},
			ffmpegPath:  "/bin/false",
			maxAttempts: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				DeviceName:   "test_" + tt.name,
				ALSADevice:   "1",
				StreamName:   "test",
				SampleRate:   48000,
				Channels:     2,
				Bitrate:      "128k",
				Codec:        "opus",
				RTSPURL:      "/dev/null",
				OutputFormat: "null",
				LockDir:      lockDir,
				FFmpegPath:   tt.ffmpegPath,
				Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, tt.maxAttempts),
			}

			mgr, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			ctx, cancel := tt.setupCtx()
			defer cancel()

			_ = mgr.Run(ctx)

			// Verify lock was released by trying to acquire it again
			mgr2, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() for second instance error = %v", err)
			}

			err = mgr2.acquireLock(context.Background())
			if err != nil {
				t.Errorf("Lock was not released after Run(): %v", err)
			}
			mgr2.releaseLock()
		})
	}
}

// TestManagerRunStateTransitions verifies correct state transitions.
func TestManagerRunStateTransitions(t *testing.T) {
	lockDir := t.TempDir()

	// Create a script that sleeps ignoring all arguments
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\nsleep 10\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy", // Arguments don't matter - script ignores them
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 5),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Should start in idle
	if mgr.State() != StateIdle {
		t.Errorf("Initial state = %v, want StateIdle", mgr.State())
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()

	// Wait for starting/running transition
	time.Sleep(100 * time.Millisecond)

	state := mgr.State()
	if state != StateStarting && state != StateRunning {
		t.Logf("Log output:\n%s", logBuf.String())
		t.Errorf("State during execution = %v, want StateStarting or StateRunning", state)
	}

	// Cancel and wait
	cancel()
	<-errCh

	// Should end in stopped
	if mgr.State() != StateStopped {
		t.Errorf("Final state = %v, want StateStopped", mgr.State())
	}
}

// TestManagerRunWithPanicsInFFmpeg verifies recovery from command panics.
func TestManagerRunWithPanicsInFFmpeg(t *testing.T) {
	lockDir := t.TempDir()

	// Create a shell script that exits immediately (simulating crash)
	scriptPath := filepath.Join(lockDir, "crash.sh")
	scriptContent := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create crash script: %v", err)
	}

	var logBuf bytes.Buffer
	cfg := &ManagerConfig{
		DeviceName:   "test",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 2),
		Logger:       slog.New(slog.NewTextHandler(&logBuf, nil)),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = mgr.Run(ctx)

	// Should eventually fail or timeout
	if err == nil {
		t.Fatal("Run() should fail when command keeps crashing")
	}

	// Should have recorded failures
	if mgr.Failures() == 0 {
		t.Error("Failures = 0, want > 0 when command crashes")
	}
}

// TestManagerRunConcurrentCalls verifies behavior when Run() is called multiple times.
func TestManagerRunConcurrentCalls(t *testing.T) {
	lockDir := t.TempDir()

	// Create a script that sleeps ignoring all arguments
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	scriptContent := "#!/bin/sh\nsleep 60\n" // Long sleep to ensure lock is held
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_concurrent",
		ALSADevice:   "dummy", // Arguments don't matter - script ignores them
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      NewBackoff(10*time.Millisecond, 50*time.Millisecond, 5),
	}

	mgr1, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(1) error = %v", err)
	}

	mgr2, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager(2) error = %v", err)
	}

	// First manager runs for 5 seconds (long enough to hold lock)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	// Second manager tries immediately with short timeout
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	// Start first manager
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- mgr1.Run(ctx1)
	}()

	// Wait for first to acquire lock and start running
	time.Sleep(200 * time.Millisecond)

	// Try to start second manager - should fail to acquire lock after 2s timeout
	start := time.Now()
	err2 := mgr2.Run(ctx2)
	elapsed := time.Since(start)

	if err2 == nil {
		t.Fatal("Second Run() should fail due to lock or timeout")
	}

	// Should fail relatively quickly (within context timeout + a bit)
	if elapsed > 35*time.Second {
		t.Errorf("Second Run() took %v, expected < 35s (lock timeout is 30s)", elapsed)
	}

	// Cancel first manager
	cancel1()

	// Wait for first to complete
	<-errCh1
}
