package stream

import (
	"testing"
	"time"
)

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
