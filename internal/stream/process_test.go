// SPDX-License-Identifier: MIT

package stream

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestBuildFFmpegCommandCodecs verifies codec-to-encoder mapping.
func TestBuildFFmpegCommandCodecs(t *testing.T) {
	tests := []struct {
		name        string
		codec       string
		wantEncoder string
	}{
		{"opus codec maps to libopus", "opus", "libopus"},
		{"aac codec maps to aac", "aac", "aac"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice: "hw:0,0",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      tt.codec,
				RTSPURL:    "rtsp://localhost:8554/test",
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			foundEncoder := false
			for i, arg := range cmd.Args {
				if arg == "-c:a" && i+1 < len(cmd.Args) {
					if cmd.Args[i+1] == tt.wantEncoder {
						foundEncoder = true
					} else {
						t.Errorf("encoder = %q, want %q", cmd.Args[i+1], tt.wantEncoder)
					}
					break
				}
			}
			if !foundEncoder {
				t.Errorf("encoder %q not found in command args: %v", tt.wantEncoder, cmd.Args)
			}
		})
	}
}

// TestBuildFFmpegCommandAllOutputFormats exercises every output format branch.
func TestBuildFFmpegCommandAllOutputFormats(t *testing.T) {
	tests := []struct {
		name           string
		rtspURL        string
		outputFormat   string
		localRecordDir string
		streamName     string
		wantInArgs     []string
		wantNotInArgs  []string
	}{
		{
			name:       "explicit rtsp with reconnect flags",
			rtspURL:    "rtsp://localhost:8554/stream",
			wantInArgs: []string{"-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "30", "-f", "rtsp"},
		},
		{
			name:         "explicit null format",
			rtspURL:      "/dev/null",
			outputFormat: "null",
			wantInArgs:   []string{"-f", "null"},
		},
		{
			name:          "file path with no format (auto-detect empty)",
			rtspURL:       "/tmp/output.ogg",
			wantNotInArgs: []string{"-f"},
		},
		{
			name:       "pipe URL auto-detect null",
			rtspURL:    "pipe:1",
			wantInArgs: []string{"-f", "null"},
		},
		{
			name:       "dash URL auto-detect null",
			rtspURL:    "-",
			wantInArgs: []string{"-f", "null"},
		},
		{
			name:       "non-slash non-rtsp URL defaults to rtsp",
			rtspURL:    "some_destination",
			wantInArgs: []string{"-f", "rtsp"},
		},
		{
			name:           "tee muxer with custom segment settings",
			rtspURL:        "rtsp://localhost:8554/dev",
			localRecordDir: "/recordings",
			streamName:     "blue_yeti",
			wantInArgs:     []string{"-f", "tee"},
		},
		{
			name:           "tee muxer default segment duration and format",
			rtspURL:        "rtsp://localhost:8554/dev",
			localRecordDir: "/recordings",
			streamName:     "mic",
			wantInArgs:     []string{"-f", "tee"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:     "hw:0,0",
				SampleRate:     48000,
				Channels:       2,
				Bitrate:        "128k",
				Codec:          "opus",
				RTSPURL:        tt.rtspURL,
				OutputFormat:   tt.outputFormat,
				LocalRecordDir: tt.localRecordDir,
				StreamName:     tt.streamName,
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)
			argsStr := strings.Join(cmd.Args, " ")

			for _, want := range tt.wantInArgs {
				if !strings.Contains(argsStr, want) {
					t.Errorf("expected %q in args: %s", want, argsStr)
				}
			}
			for _, notWant := range tt.wantNotInArgs {
				// For "-f", only check the output-related portion (skip input format)
				// The first -f is the input format, which is always present.
				// Check there's only one -f (the input one).
				if notWant == "-f" {
					count := 0
					for _, arg := range cmd.Args {
						if arg == "-f" {
							count++
						}
					}
					// There should be exactly 1 -f (the input format); no output format
					if count > 1 {
						t.Errorf("expected only 1 -f flag (input), got %d: %s", count, argsStr)
					}
				}
			}
		})
	}
}

// TestBuildFFmpegCommandTeeMuxerSegmentDefaults verifies default segment
// duration (3600) and format (wav) when not explicitly configured.
func TestBuildFFmpegCommandTeeMuxerSegmentDefaults(t *testing.T) {
	cfg := &ManagerConfig{
		ALSADevice:     "hw:0,0",
		SampleRate:     48000,
		Channels:       2,
		Bitrate:        "128k",
		Codec:          "opus",
		RTSPURL:        "rtsp://localhost:8554/test",
		LocalRecordDir: "/tmp/recordings",
		StreamName:     "dev",
	}

	cmd := buildFFmpegCommand(context.Background(), cfg)
	lastArg := cmd.Args[len(cmd.Args)-1]

	if !strings.Contains(lastArg, "segment_time=3600") {
		t.Errorf("expected default segment_time=3600, got: %s", lastArg)
	}
	if !strings.Contains(lastArg, ".wav") {
		t.Errorf("expected default .wav format, got: %s", lastArg)
	}
}

// TestBuildFFmpegCommandTeeMuxerCustomSegment verifies custom segment settings.
func TestBuildFFmpegCommandTeeMuxerCustomSegment(t *testing.T) {
	cfg := &ManagerConfig{
		ALSADevice:      "hw:0,0",
		SampleRate:      48000,
		Channels:        2,
		Bitrate:         "128k",
		Codec:           "aac",
		RTSPURL:         "rtsp://localhost:8554/test",
		LocalRecordDir:  "/var/audio",
		StreamName:      "mic1",
		SegmentDuration: 900,
		SegmentFormat:   "ogg",
	}

	cmd := buildFFmpegCommand(context.Background(), cfg)
	lastArg := cmd.Args[len(cmd.Args)-1]

	if !strings.Contains(lastArg, "segment_time=900") {
		t.Errorf("expected segment_time=900, got: %s", lastArg)
	}
	if !strings.Contains(lastArg, ".ogg") {
		t.Errorf("expected .ogg format, got: %s", lastArg)
	}
	if !strings.Contains(lastArg, "mic1_") {
		t.Errorf("expected stream name in segment pattern, got: %s", lastArg)
	}
}

// TestBuildFFmpegCommandWithThreadQueue verifies thread queue value is correct.
func TestBuildFFmpegCommandWithThreadQueue(t *testing.T) {
	cfg := &ManagerConfig{
		ALSADevice:  "hw:0,0",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 4096,
		RTSPURL:     "rtsp://localhost:8554/test",
	}

	cmd := buildFFmpegCommand(context.Background(), cfg)
	found := false
	for i, arg := range cmd.Args {
		if arg == "-thread_queue_size" && i+1 < len(cmd.Args) {
			if cmd.Args[i+1] != "4096" {
				t.Errorf("thread_queue_size = %s, want 4096", cmd.Args[i+1])
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("thread_queue_size flag not found")
	}
}

// TestStopMonitoringNilCancel verifies stopMonitoring is safe when monitorCancel is nil.
func TestStopMonitoringNilCancel(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test",
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

	// stopMonitoring with nil monitorCancel should not panic
	mgr.stopMonitoring()
}

// TestStopMonitoringWithCancel verifies stopMonitoring calls and nils the cancel func.
func TestStopMonitoringWithCancel(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test",
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

	// Set up a cancel function
	called := false
	mgr.mu.Lock()
	mgr.monitorCancel = func() { called = true }
	mgr.mu.Unlock()

	mgr.stopMonitoring()

	if !called {
		t.Error("stopMonitoring should call monitorCancel")
	}

	mgr.mu.RLock()
	isNil := mgr.monitorCancel == nil
	mgr.mu.RUnlock()
	if !isNil {
		t.Error("stopMonitoring should set monitorCancel to nil")
	}
}

// TestManagerStateNilManager verifies State() returns StateIdle for nil manager.
func TestManagerStateNilManager(t *testing.T) {
	var mgr *Manager
	if mgr.State() != StateIdle {
		t.Errorf("nil Manager.State() = %v, want StateIdle", mgr.State())
	}
}

// TestManagerStateUninitializedAtomicValue verifies State() returns StateIdle
// when the atomic.Value has not been initialized (e.g., via &Manager{}).
func TestManagerStateUninitializedAtomicValue(t *testing.T) {
	mgr := &Manager{}
	if mgr.State() != StateIdle {
		t.Errorf("uninitialized Manager.State() = %v, want StateIdle", mgr.State())
	}
}

// TestManagerMetricsNilManager verifies Metrics() returns zero-value for nil manager.
func TestManagerMetricsNilManager(t *testing.T) {
	var mgr *Manager
	metrics := mgr.Metrics()
	if metrics.State != StateIdle {
		t.Errorf("nil Manager.Metrics().State = %v, want StateIdle", metrics.State)
	}
	if metrics.DeviceName != "" {
		t.Errorf("nil Manager.Metrics().DeviceName = %q, want empty", metrics.DeviceName)
	}
}

// TestManagerMetricsWithNilConfig verifies Metrics() handles nil cfg gracefully.
func TestManagerMetricsWithNilConfig(t *testing.T) {
	mgr := &Manager{}
	mgr.state.Store(StateFailed)

	metrics := mgr.Metrics()
	if metrics.State != StateFailed {
		t.Errorf("Metrics().State = %v, want StateFailed", metrics.State)
	}
	if metrics.DeviceName != "" {
		t.Errorf("Metrics().DeviceName = %q, want empty", metrics.DeviceName)
	}
}

// TestManagerMetricsWithStartTime verifies uptime calculation.
func TestManagerMetricsWithStartTime(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test",
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

	// Set startTime to 1 second ago
	mgr.mu.Lock()
	mgr.startTime = time.Now().Add(-1 * time.Second)
	mgr.mu.Unlock()
	mgr.attempts.Store(3)
	mgr.failures.Store(1)

	metrics := mgr.Metrics()
	if metrics.Uptime < 900*time.Millisecond {
		t.Errorf("Metrics().Uptime = %v, want >= 900ms", metrics.Uptime)
	}
	if metrics.Attempts != 3 {
		t.Errorf("Metrics().Attempts = %d, want 3", metrics.Attempts)
	}
	if metrics.Failures != 1 {
		t.Errorf("Metrics().Failures = %d, want 1", metrics.Failures)
	}
}

// TestManagerLogError verifies logError method.
func TestManagerLogError(t *testing.T) {
	tests := []struct {
		name      string
		hasLogger bool
	}{
		{"with logger", true},
		{"without logger", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				DeviceName: "test",
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

			if tt.hasLogger {
				cfg.Logger = discardLogger()
			}

			mgr, err := NewManager(cfg)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}

			// Should not panic regardless of logger presence
			mgr.logError("test error %d", 42)
		})
	}
}

// TestNewManagerWithMonitorInterval verifies resource monitor creation.
func TestNewManagerWithMonitorInterval(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName:      "test",
		ALSADevice:      "hw:0,0",
		StreamName:      "stream",
		SampleRate:      48000,
		Channels:        2,
		Bitrate:         "128k",
		Codec:           "opus",
		RTSPURL:         "rtsp://localhost:8554/test",
		LockDir:         t.TempDir(),
		FFmpegPath:      "/usr/bin/ffmpeg",
		Backoff:         NewBackoff(1*time.Second, 10*time.Second, 5),
		MonitorInterval: 5 * time.Second,
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if mgr.resourceMonitor == nil {
		t.Error("resourceMonitor should be set when MonitorInterval > 0")
	}
}

// TestNewManagerWithInvalidLogDir verifies error when LogDir is invalid.
func TestNewManagerWithInvalidLogDir(t *testing.T) {
	cfg := &ManagerConfig{
		DeviceName: "test",
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
		LogDir:     "/\x00invalid/path",
	}

	_, err := NewManager(cfg)
	if err == nil {
		t.Error("NewManager() should fail with invalid LogDir")
	}
	if !strings.Contains(err.Error(), "log writer") {
		t.Errorf("error = %q, want error about log writer", err.Error())
	}
}

// discardLogger returns a logger that writes to nowhere (for tests that
// just need a non-nil logger).
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(devNull{}, nil))
}

type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }
