//go:build linux

package diagnostics

import (
	"context"
	"testing"
	"time"
)

func TestCheckFFmpeg(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkFFmpeg(ctx)
	if result.Name != "FFmpeg" {
		t.Errorf("expected Name 'FFmpeg', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckALSA(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkALSA(ctx)
	if result.Name != "ALSA" {
		t.Errorf("expected Name 'ALSA', got %q", result.Name)
	}
}

func TestCheckMediaMTXService(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXService(ctx)
	if result.Name != "MediaMTX Service" {
		t.Errorf("expected Name 'MediaMTX Service', got %q", result.Name)
	}
}

func TestCheckMediaMTXAPI(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXAPI(ctx)
	if result.Name != "MediaMTX API" {
		t.Errorf("expected Name 'MediaMTX API', got %q", result.Name)
	}
}

func TestCheckTimeSynchronization(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkTimeSynchronization(ctx)
	if result.Name != "Time Sync" {
		t.Errorf("expected Name 'Time Sync', got %q", result.Name)
	}
}

func TestCheckSystemdServices(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkSystemdServices(ctx)
	if result.Name != "Systemd Services" {
		t.Errorf("expected Name 'Systemd Services', got %q", result.Name)
	}
}

func TestCheckProcessStability(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkProcessStability(ctx)
	if result.Name != "Process Stability" {
		t.Errorf("expected Name 'Process Stability', got %q", result.Name)
	}
}

func TestCheckAudioConflicts(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioConflicts(ctx)
	if result.Name != "Audio Conflicts" {
		t.Errorf("expected Name 'Audio Conflicts', got %q", result.Name)
	}
}

func TestCheckInotifyLimits(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkInotifyLimits(context.Background())
	if result.Name != "inotify Limits" {
		t.Errorf("expected Name 'inotify Limits', got %q", result.Name)
	}
}

func TestCheckTCPResources(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkTCPResources(context.Background())
	if result.Name != "TCP Resources" {
		t.Errorf("expected Name 'TCP Resources', got %q", result.Name)
	}
}

func TestCheckEntropy(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkEntropy(context.Background())
	if result.Name != "Entropy" {
		t.Errorf("expected Name 'Entropy', got %q", result.Name)
	}
}

func TestCheckFileDescriptors(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkFileDescriptors(context.Background())
	if result.Name != "File Descriptors" {
		t.Errorf("expected Name 'File Descriptors', got %q", result.Name)
	}
}

func TestCheckAudioCapabilities(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioCapabilities(ctx)
	if result.Name != "Audio Capabilities" {
		t.Errorf("expected Name 'Audio Capabilities', got %q", result.Name)
	}
}
