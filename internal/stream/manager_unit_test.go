package stream

import (
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

			cmd := buildFFmpegCommand(cfg)

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
		_ = buildFFmpegCommand(cfg)
	}
}
