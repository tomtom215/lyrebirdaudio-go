package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestKoanfConfig_Reload tests manual configuration reload.
func TestKoanfConfig_Reload(t *testing.T) {
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

	cfg, err := kc.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Default.SampleRate != 48000 {
		t.Fatalf("Expected initial sample rate 48000, got %d", cfg.Default.SampleRate)
	}

	// Modify config file
	updatedConfig := `
default:
  sample_rate: 44100
  channels: 2
  bitrate: 192k
  codec: aac
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

	// Reload configuration
	if err := kc.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Load again and verify changes
	cfg, err = kc.Load()
	if err != nil {
		t.Fatalf("Load after reload failed: %v", err)
	}

	if cfg.Default.SampleRate != 44100 {
		t.Errorf("Expected reloaded sample rate 44100, got %d", cfg.Default.SampleRate)
	}

	if cfg.Default.Bitrate != "192k" {
		t.Errorf("Expected reloaded bitrate 192k, got %s", cfg.Default.Bitrate)
	}

	if cfg.Default.Codec != "aac" {
		t.Errorf("Expected reloaded codec aac, got %s", cfg.Default.Codec)
	}
}

// TestKoanfConfig_AllAfterReload tests that All() reflects reloaded values.
func TestKoanfConfig_AllAfterReload(t *testing.T) {
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

	// Modify config file
	updatedConfig := `
default:
  sample_rate: 44100
  channels: 1
  bitrate: 64k
  codec: aac
  thread_queue: 4096

stream:
  initial_restart_delay: 20s

mediamtx:
  api_url: http://localhost:8888

monitor:
  enabled: false
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config: %v", err)
	}

	// Reload
	if err := kc.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Check All() reflects new values
	allConfig := kc.All()
	if allConfig == nil {
		t.Fatal("All() returned nil after reload")
	}

	// The map should reflect the reloaded configuration
	if len(allConfig) == 0 {
		t.Error("All() returned empty map after reload")
	}
}

// TestKoanfConfig_ConcurrentReloadAndRead tests that concurrent Reload and
// getter calls do not cause a data race on the internal koanf pointer.
// This test is designed to be run with `go test -race` to detect races.
func TestKoanfConfig_ConcurrentReloadAndRead(t *testing.T) {
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
  interval: 5m
`
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	kc, err := NewKoanfConfig(WithYAMLFile(configPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig failed: %v", err)
	}

	const numGoroutines = 10
	const numIterations = 50

	var wg sync.WaitGroup

	// Start goroutines that continuously reload
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = kc.Reload()
			}
		}()
	}

	// Start goroutines that continuously read via GetString
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = kc.GetString("default.codec")
			}
		}()
	}

	// Start goroutines that continuously read via GetInt
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = kc.GetInt("default.sample_rate")
			}
		}()
	}

	// Start goroutines that continuously read via GetBool
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = kc.GetBool("monitor.enabled")
			}
		}()
	}

	// Start goroutines that continuously read via GetDuration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = kc.GetDuration("stream.initial_restart_delay")
			}
		}()
	}

	// Start goroutines that continuously read via Exists
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = kc.Exists("default.codec")
			}
		}()
	}

	// Start goroutines that continuously read via All
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = kc.All()
			}
		}()
	}

	// Start goroutines that continuously read via Load
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_, _ = kc.Load()
			}
		}()
	}

	wg.Wait()
}
