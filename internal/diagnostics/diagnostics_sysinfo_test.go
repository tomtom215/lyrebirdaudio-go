//go:build linux

package diagnostics

import (
	"testing"
)

func TestCollectSystemInfo(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	info := runner.collectSystemInfo()

	if info == nil {
		t.Fatal("expected system info to be non-nil")
	}

	if info.OS == "" {
		t.Error("expected OS to be non-empty")
	}

	if info.Architecture == "" {
		t.Error("expected Architecture to be non-empty")
	}

	if info.CPUs <= 0 {
		t.Error("expected CPUs to be positive")
	}

	if info.GoVersion == "" {
		t.Error("expected GoVersion to be non-empty")
	}
}

func TestSystemInfoFields(t *testing.T) {
	info := &SystemInfo{
		Hostname:     "test",
		OS:           "linux",
		Kernel:       "5.4.0",
		Architecture: "amd64",
		CPUs:         4,
		Memory:       8 * 1024 * 1024 * 1024,
		Uptime:       "1 day",
		GoVersion:    "go1.23",
	}

	if info.Hostname != "test" {
		t.Errorf("expected Hostname to be 'test', got %q", info.Hostname)
	}
	if info.OS != "linux" {
		t.Errorf("expected OS to be 'linux', got %q", info.OS)
	}
	if info.CPUs != 4 {
		t.Errorf("expected CPUs to be 4, got %d", info.CPUs)
	}
	if info.Memory != 8*1024*1024*1024 {
		t.Errorf("expected Memory to be 8GB, got %d", info.Memory)
	}
}
