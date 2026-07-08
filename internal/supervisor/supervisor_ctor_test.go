package supervisor

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "default config",
			cfg:  DefaultConfig(),
		},
		{
			name: "custom timeout",
			cfg: Config{
				ShutdownTimeout: 5 * time.Second,
			},
		},
		{
			name: "zero timeout uses default",
			cfg:  Config{},
		},
		{
			name: "with restart policy",
			cfg: Config{
				ShutdownTimeout: 10 * time.Second,
				RestartDelay:    2 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sup := New(tt.cfg)
			if sup == nil {
				t.Fatal("New returned nil")
			}
			// Verify supervisor was created (internal state)
			if sup.suture == nil {
				t.Error("suture supervisor not initialized")
			}
		})
	}
}

func TestServiceState_String(t *testing.T) {
	tests := []struct {
		state    ServiceState
		expected string
	}{
		{ServiceStateIdle, "idle"},
		{ServiceStateRunning, "running"},
		{ServiceStateStopping, "stopping"},
		{ServiceStateFailed, "failed"},
		{ServiceStateStopped, "stopped"},
		{ServiceState(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 10*time.Second)
	}
	if cfg.RestartDelay != 1*time.Second {
		t.Errorf("RestartDelay = %v, want %v", cfg.RestartDelay, 1*time.Second)
	}
}

// TestBuildSpecMapsConfig verifies that buildSpec honors the supervisor Config
// by mapping it onto suture's restart-throttling fields. This is the observable
// contract for defect fix #2: RestartDelay must reach suture as FailureBackoff,
// ShutdownTimeout as Timeout, and jitter must be enabled so correlated restarts
// are decorrelated.
func TestBuildSpecMapsConfig(t *testing.T) {
	cfg := Config{
		Name:            "test-sup",
		ShutdownTimeout: 7 * time.Second,
		RestartDelay:    3 * time.Second,
	}

	spec := buildSpec(cfg)

	if spec.FailureBackoff != cfg.RestartDelay {
		t.Errorf("FailureBackoff = %v, want RestartDelay %v", spec.FailureBackoff, cfg.RestartDelay)
	}
	if spec.Timeout != cfg.ShutdownTimeout {
		t.Errorf("Timeout = %v, want ShutdownTimeout %v", spec.Timeout, cfg.ShutdownTimeout)
	}
	if spec.BackoffJitter == nil {
		t.Error("BackoffJitter = nil, want jitter enabled so throttled restarts are decorrelated")
	}
}
