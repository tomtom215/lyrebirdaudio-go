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

// TestStartReloadHandlerReloadError verifies that a Reload() failure
// logs a warning and continues (does not remove services).
func TestStartReloadHandlerReloadError(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"
	writeTestConfig(t, cfgPath, minimalConfig())

	koanfCfg, _, err := loadConfigurationKoanf(cfgPath)
	if err != nil || koanfCfg == nil {
		t.Fatalf("loadConfigurationKoanf: err=%v koanfCfg=%v", err, koanfCfg)
	}

	// Remove the config file so Reload() will fail with a missing-file error.
	if err := os.Remove(cfgPath); err != nil {
		t.Fatalf("remove config: %v", err)
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

// TestStartReloadHandlerDeviceConfigUnchanged verifies that a device whose
// config hash matches the reloaded config is NOT removed from the services map.
func TestStartReloadHandlerDeviceConfigUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"
	writeTestConfig(t, cfgPath, minimalConfig())

	koanfCfg, cfg, err := loadConfigurationKoanf(cfgPath)
	if err != nil || koanfCfg == nil || cfg == nil {
		t.Fatalf("loadConfigurationKoanf: err=%v", err)
	}

	devName := "test_device"

	// Compute the hash that the handler will compute after reload.
	devCfg := cfg.GetDeviceConfig(devName)
	rtspURL := cfg.MediaMTX.RTSPURL + "/" + devName
	correctHash := deviceConfigHash(devCfg, rtspURL, cfg.Stream)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := supervisor.New(supervisor.Config{})
	reloadCh := make(chan struct{}, 1)
	var mu sync.RWMutex
	// Register device with the correct hash so no restart is triggered.
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: correctHash}
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

	// Device should still be registered since hash matched.
	mu.RLock()
	_, stillRegistered := services[devName]
	mu.RUnlock()

	if !stillRegistered {
		t.Errorf("device %q should remain in services map when config is unchanged", devName)
	}
}

// TestStartReloadHandlerDeviceConfigChanged verifies that a device whose
// config hash differs from the reloaded config IS removed from the services map.
func TestStartReloadHandlerDeviceConfigChanged(t *testing.T) {
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
	defer cancel()

	sup := supervisor.New(supervisor.Config{})

	// Add a mock service so sup.Remove() can succeed.
	devName := "test_device"
	svc := &mockService{name: devName}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("sup.Add: %v", err)
	}

	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()
	go func() { _ = sup.Run(supCtx) }()
	time.Sleep(50 * time.Millisecond) // let the service start

	reloadCh := make(chan struct{}, 1)
	var mu sync.RWMutex
	// Register device with a deliberately wrong hash to force a "config changed" restart.
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: "stale-hash-that-will-not-match"}
	registerDevices := func(c *config.Config) int { return 0 }

	done := make(chan struct{})
	go func() {
		startReloadHandler(ctx, logger, reloadCh, koanfCfg, sup, &mu, services, hashes, registerDevices)
		close(done)
	}()

	reloadCh <- struct{}{}

	// Give the handler time to process the reload signal, call sup.Remove(),
	// and update the maps. Remove() blocks until the service stops, which is
	// quick since Stop() cancels the service context and mockService returns immediately.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startReloadHandler did not exit after context cancel")
	}

	mu.RLock()
	_, stillRegistered := services[devName]
	mu.RUnlock()

	if stillRegistered {
		t.Errorf("device %q should be removed from services map after config hash changed", devName)
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("config changed for device, restarting stream")) {
		t.Logf("log output: %s", logBuf.String())
		t.Error("expected 'config changed for device, restarting stream' log message")
	}
}

// TestStartReloadHandlerRegistersNewDevicesOnReload verifies that registerDevices
// is called after a successful reload (covers the final n > 0 log path).
func TestStartReloadHandlerRegistersNewDevicesOnReload(t *testing.T) {
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
	defer cancel()

	sup := supervisor.New(supervisor.Config{})
	reloadCh := make(chan struct{}, 1)
	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)

	registerCalled := make(chan struct{}, 1)
	registerDevices := func(c *config.Config) int {
		select {
		case registerCalled <- struct{}{}:
		default:
		}
		return 1 // report one new device to trigger "registered new/restarted devices" log
	}

	done := make(chan struct{})
	go func() {
		startReloadHandler(ctx, logger, reloadCh, koanfCfg, sup, &mu, services, hashes, registerDevices)
		close(done)
	}()

	reloadCh <- struct{}{}

	select {
	case <-registerCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("registerDevices was not called after reload")
	}

	cancel()
	<-done

	if !bytes.Contains(logBuf.Bytes(), []byte("registered new/restarted devices on reload")) {
		t.Errorf("expected 'registered new/restarted devices on reload' log, got: %s", logBuf.String())
	}
}
