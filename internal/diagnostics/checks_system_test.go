// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCollectSystemInfoLinux(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	info := runner.collectSystemInfo()

	// On Linux, these should all be populated
	if info.OS != "linux" {
		t.Errorf("expected OS 'linux', got %q", info.OS)
	}
	if info.Hostname == "" {
		t.Error("expected non-empty Hostname on Linux")
	}
	if info.Kernel == "" {
		t.Error("expected non-empty Kernel on Linux (from /proc/version)")
	}
	if info.Memory <= 0 {
		t.Error("expected positive Memory on Linux (from /proc/meminfo)")
	}
	if info.Uptime == "" {
		t.Error("expected non-empty Uptime on Linux (from /proc/uptime)")
	}
	if info.CPUs <= 0 {
		t.Errorf("expected positive CPUs, got %d", info.CPUs)
	}
	if info.GoVersion == "" {
		t.Error("expected non-empty GoVersion")
	}
}

func TestCheckPrerequisitesCategories(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkPrerequisites(context.Background())

	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}

	switch result.Status {
	case StatusOK:
		if !strings.Contains(result.Message, "All required tools available") {
			t.Errorf("unexpected OK message: %q", result.Message)
		}
	case StatusWarning:
		if !strings.Contains(result.Message, "Missing optional tools") {
			t.Errorf("unexpected Warning message: %q", result.Message)
		}
	case StatusCritical:
		if !strings.Contains(result.Message, "Missing required tools") {
			t.Errorf("unexpected Critical message: %q", result.Message)
		}
		if len(result.Suggestions) == 0 {
			t.Error("expected suggestions when critical tools are missing")
		}
	default:
		t.Errorf("unexpected status %s for prerequisites", result.Status)
	}
}

func TestCheckVersionsAlwaysOK(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkVersions(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
	if result.Message != "Version information collected" {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestCheckSystemInfoAlwaysOK(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkSystemInfo(context.Background())

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
	if result.Message != "System information collected" {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestCheckTimeSynchronizationSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkTimeSynchronization(ctx)

	if result.Name != "Time Sync" {
		t.Errorf("expected Name 'Time Sync', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
}

func TestCheckSystemdServicesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkSystemdServices(ctx)

	if result.Name != "Systemd Services" {
		t.Errorf("expected Name 'Systemd Services', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckProcessStabilitySetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkProcessStability(ctx)

	if result.Name != "Process Stability" {
		t.Errorf("expected Name 'Process Stability', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckUdevRulesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUdevRules(context.Background())

	if result.Name != "udev Rules" {
		t.Errorf("expected Name 'udev Rules', got %q", result.Name)
	}
	if result.Category != "Config" {
		t.Errorf("expected Category 'Config', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
	if result.Status == StatusWarning {
		if len(result.Suggestions) == 0 {
			t.Error("expected suggestions when udev rules not found")
		}
	}
}
