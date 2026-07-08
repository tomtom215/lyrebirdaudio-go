// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestStartReloadHandlerLoadErrorPath covers the reload handler's rejection of
// a semantically-invalid hot reload. We overwrite the config file with valid
// YAML containing an invalid value (sample_rate: -1). reload() now validates
// before swapping, so Reload() itself fails and the handler logs "failed to
// reload configuration" while keeping the last-known-good config live.
func TestStartReloadHandlerLoadErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"
	writeTestConfig(t, cfgPath, minimalConfig())

	koanfCfg, _, err := loadConfigurationKoanf(cfgPath)
	if err != nil || koanfCfg == nil {
		t.Fatalf("loadConfigurationKoanf: err=%v koanfCfg=%v", err, koanfCfg)
	}

	// Overwrite with valid YAML but semantically invalid config.
	// sample_rate: -1 passes YAML parsing but fails Validate().
	invalidConfig := `default:
  sample_rate: -1
  channels: 2
  bitrate: "128k"
  codec: opus
`
	if err := os.WriteFile(cfgPath, []byte(invalidConfig), 0640); err != nil { //#nosec G304 -- test helper
		t.Fatalf("WriteFile invalid config: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := supervisor.New(supervisor.Config{})
	reloadCh := make(chan struct{}, 1)
	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)
	registerDevices := func(c *config.Config) int { return 0 }

	done := make(chan struct{})
	go func() {
		startReloadHandler(ctx, logger, reloadCh, koanfCfg, sup, &mu, services, hashes, registerDevices)
		close(done)
	}()

	reloadCh <- struct{}{}
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startReloadHandler did not exit after context cancel")
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("failed to reload configuration")) {
		t.Errorf("expected 'failed to reload configuration' warning, got: %s", logBuf.String())
	}
}

// TestStartReloadHandlerRemoveErrorPath covers monitors.go:124-127 — the
// sup.Remove() error branch. The device is in registeredServices with a stale
// hash so the handler detects a config change and calls sup.Remove(). The
// service was never added to the supervisor, so Remove() returns "not found".
func TestStartReloadHandlerRemoveErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"
	writeTestConfig(t, cfgPath, minimalConfig())

	koanfCfg, _, err := loadConfigurationKoanf(cfgPath)
	if err != nil || koanfCfg == nil {
		t.Fatalf("loadConfigurationKoanf: err=%v koanfCfg=%v", err, koanfCfg)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Do NOT add any service to the supervisor.
	sup := supervisor.New(supervisor.Config{})

	reloadCh := make(chan struct{}, 1)
	var mu sync.RWMutex
	devName := "phantom_device"
	// Register a device with a stale hash so the hash-check detects a change.
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: "stale-hash-never-matches"}
	registerDevices := func(c *config.Config) int { return 0 }

	done := make(chan struct{})
	go func() {
		startReloadHandler(ctx, logger, reloadCh, koanfCfg, sup, &mu, services, hashes, registerDevices)
		close(done)
	}()

	reloadCh <- struct{}{}
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startReloadHandler did not exit after context cancel")
	}

	// The handler should have logged the Remove error.
	if !bytes.Contains(logBuf.Bytes(), []byte("failed to remove service for restart")) {
		t.Errorf("expected 'failed to remove service for restart' warning, got: %s", logBuf.String())
	}
}

// TestStartDevicePollerKoanfNonNilRegistersDevices covers monitors.go:36-43 —
// the koanfCfg != nil branch in the ticker loop. A non-nil koanfCfg is
// passed; after the 10-second ticker fires, Load() is called and the result
// is passed to registerDevices. The callback returns 1 to also cover the
// n > 0 logging path (monitors.go:48-50).
func TestStartDevicePollerKoanfNonNilRegistersDevices(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"
	writeTestConfig(t, cfgPath, minimalConfig())

	koanfCfg, _, err := loadConfigurationKoanf(cfgPath)
	if err != nil || koanfCfg == nil {
		t.Fatalf("loadConfigurationKoanf: err=%v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())

	called := make(chan *config.Config, 1)
	registerDevices := func(c *config.Config) int {
		select {
		case called <- c:
		default:
		}
		cancel() // cancel after first call so the function exits
		return 1 // n > 0 triggers the "discovered new devices" log
	}

	done := make(chan struct{})
	go func() {
		startDevicePoller(ctx, logger, koanfCfg, nil, registerDevices)
		close(done)
	}()

	select {
	case <-called:
	case <-time.After(15 * time.Second):
		cancel()
		t.Fatal("registerDevices was not called within 15s (tick interval is 10s)")
	}

	<-done

	if !bytes.Contains(logBuf.Bytes(), []byte("discovered new devices")) {
		t.Errorf("expected 'discovered new devices' log, got: %s", logBuf.String())
	}
}

// TestStartDevicePollerKoanfLoadError covers monitors.go:40-43 — the
// loadErr != nil branch when koanfCfg.Load() fails each tick. NewKoanfConfig's
// initial load does not pre-validate (there is nothing to preserve), so a
// semantically-invalid config file constructs successfully but the poller's
// Load()->Validate() fails on every tick; the poller logs a warning and
// continues. (A hot reload of an invalid config is instead rejected at Reload()
// time — see TestStartReloadHandlerLoadErrorPath.) The context is cancelled
// ~12s in (after the first 10s tick fires).
func TestStartDevicePollerKoanfLoadError(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"

	// A semantically-invalid config (negative sample rate).
	invalidCfg := `default:
  sample_rate: -1
  channels: 2
  bitrate: "128k"
  codec: opus
`
	if err := os.WriteFile(cfgPath, []byte(invalidCfg), 0640); err != nil { //#nosec G304 -- test helper
		t.Fatalf("WriteFile: %v", err)
	}
	koanfCfg, err := config.NewKoanfConfig(config.WithYAMLFile(cfgPath), config.WithEnvPrefix("LYREBIRD"))
	if err != nil {
		t.Fatalf("NewKoanfConfig: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tickCount := 0
	registerDevices := func(c *config.Config) int {
		tickCount++
		cancel()
		return 0
	}

	done := make(chan struct{})
	go func() {
		startDevicePoller(ctx, logger, koanfCfg, nil, registerDevices)
		close(done)
	}()

	// Cancel after ~12s so the first tick (10s interval) fires and the error
	// path executes before we force-exit.
	go func() {
		time.Sleep(12 * time.Second)
		cancel()
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("startDevicePoller did not exit within 20s")
	}

	// registerDevices should NOT have been called (Load error → continue).
	if tickCount > 0 {
		t.Errorf("registerDevices should not be called on Load error, called %d times", tickCount)
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("failed to load config for device scan")) {
		t.Errorf("expected 'failed to load config for device scan' warning, got: %s", logBuf.String())
	}
}
