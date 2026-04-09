package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKoanfConfig_Watch tests configuration file watching.
func TestKoanfConfig_Watch(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write initial config
	initialConfig := `
default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	// Channel to signal when watch callback is called
	watchCalled := make(chan string, 1)

	// Start watching in background
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = kc.Watch(ctx, func(event string, err error) {
			if err != nil {
				watchCalled <- "error: " + err.Error()
				return
			}
			watchCalled <- event
		})
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Modify config file
	updatedConfig := `
default:
  sample_rate: 44100
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config: %v", err)
	}

	// Wait for watch callback
	select {
	case event := <-watchCalled:
		if event != "config reloaded" {
			t.Errorf("Expected event 'config reloaded', got %s", event)
		}
	case <-time.After(2 * time.Second):
		t.Error("Watch callback not called within timeout")
	}

	// Verify config was reloaded
	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load after watch failed: %v", err)
	}

	if cfg.Default.SampleRate != 44100 {
		t.Errorf("Expected watched sample rate 44100, got %d", cfg.Default.SampleRate)
	}
}

// TestKoanfConfig_WatchNoFile tests Watch with no file specified.
func TestKoanfConfig_WatchNoFile(t *testing.T) {
	// Load config without file
	kc, err := NewKoanfConfig(WithEnvPrefix("LYREBIRD"))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	// Watch should return an error when no file path is specified
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = kc.Watch(ctx, func(event string, watchErr error) {
		t.Error("Callback should not be called when no file is set")
	})

	if err == nil {
		t.Error("Watch without file should return an error")
	}

	// Verify the error message is appropriate
	if err != nil && !strings.Contains(err.Error(), "no file path specified") {
		t.Errorf("Expected error about no file path, got: %v", err)
	}
}

// TestKoanfConfig_WatchContextCancellation tests Watch with context cancellation.
func TestKoanfConfig_WatchContextCancellation(t *testing.T) {
	// Create temp directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	testConfig := `
default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

stream:
  initial_restart_delay: 10s

mediamtx:
  api_url: http://localhost:9997

monitor:
  enabled: true
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	// Create context that will be cancelled quickly
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = kc.Watch(ctx, func(event string, err error) {})
		close(done)
	}()

	// Cancel context after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Watch should exit when context is cancelled
	select {
	case <-done:
		// Success - Watch returned when context was cancelled
	case <-time.After(2 * time.Second):
		t.Error("Watch did not return when context was cancelled")
	}
}
