// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// writeTestConfig writes content to path with 0640 permissions.
func writeTestConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0640); err != nil { //#nosec G304 -- test helper, path from t.TempDir()
		t.Fatalf("write config: %v", err)
	}
}

// minimalConfig returns a minimal YAML config string for testing.
func minimalConfig() string {
	return `default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
stream:
  initial_restart_delay: 1s
  max_restart_delay: 5m
`
}

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

// TestStartFailedStreamRecoveryWithFailedService verifies that a failed service
// is removed from the registered services map by the recovery goroutine.
func TestStartFailedStreamRecoveryWithFailedService(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	sup := supervisor.New(supervisor.Config{
		ShutdownTimeout: 5 * time.Second,
	})

	devName := "failing_device"
	svc := &mockService{name: devName, err: errors.New("simulated device error")}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("sup.Add: %v", err)
	}

	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()
	go func() { _ = sup.Run(supCtx) }()

	// Wait long enough for the service to fail and enter ServiceStateFailed.
	time.Sleep(300 * time.Millisecond)

	var mu sync.RWMutex
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: "hash1"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		startFailedStreamRecovery(ctx, logger, 50*time.Millisecond, sup, &mu, services, hashes)
		close(done)
	}()

	// Allow several ticks to pass, giving recovery ample time to see the failed service.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("startFailedStreamRecovery did not exit after context cancel")
	}

	mu.RLock()
	_, stillRegistered := services[devName]
	mu.RUnlock()

	if stillRegistered {
		// The service might have cycled through multiple states — log output for diagnosis.
		t.Logf("log output: %s", logBuf.String())
		t.Errorf("failed service %q should have been removed from services map", devName)
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

// mediamtxPathResponse is the minimal JSON structure returned by the fake MediaMTX API.
type mediamtxPathResponse struct {
	Name          string `json:"name"`
	Ready         bool   `json:"ready"`
	BytesReceived int64  `json:"bytesReceived"`
	BytesSent     int64  `json:"bytesSent"`
}

// newFakeMediaMTXServer returns an httptest.Server that responds to MediaMTX path
// requests. The handler calls nextStats on each request to obtain the response body.
func newFakeMediaMTXServer(t *testing.T, nextStats func(name string) mediamtxPathResponse) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/get/", func(w http.ResponseWriter, r *http.Request) {
		// Extract path name from URL: /v3/paths/get/{name}
		name := r.URL.Path[len("/v3/paths/get/"):]
		resp := nextStats(name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return httptest.NewServer(mux)
}

// TestStartStallDetectorDataStalled verifies that a stream whose byte count
// does not advance increments the stall counter and eventually triggers a restart.
func TestStartStallDetectorDataStalled(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Fake MediaMTX: always returns ready=true, bytes=1000 (never increases → stall).
	fakeServer := newFakeMediaMTXServer(t, func(name string) mediamtxPathResponse {
		return mediamtxPathResponse{Name: name, Ready: true, BytesReceived: 1000}
	})
	defer fakeServer.Close()

	sup := supervisor.New(supervisor.Config{ShutdownTimeout: 2 * time.Second})
	devName := "stalled_device"
	svc := &mockService{name: devName}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("sup.Add: %v", err)
	}
	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()
	go func() { _ = sup.Run(supCtx) }()
	time.Sleep(50 * time.Millisecond) // let service start

	cfg := config.DefaultConfig()
	cfg.MediaMTX.APIURL = fakeServer.URL
	cfg.Monitor.StallCheckInterval = 50 * time.Millisecond
	cfg.Monitor.MaxStallChecks = 2
	cfg.Monitor.RestartUnhealthy = true

	var mu sync.RWMutex
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: "hash"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		startStallDetector(ctx, logger, cfg, sup, &mu, services, hashes)
		close(done)
	}()

	// 3 ticks = 150ms: tick1 sets prevBytes, tick2 detects stall (count=1),
	// tick3 detects stall again (count=2 >= maxStallChecks=2) → restart.
	time.Sleep(400 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startStallDetector did not exit")
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("stream data stalled")) {
		t.Logf("log: %s", logBuf.String())
		t.Error("expected 'stream data stalled' log")
	}
}

// TestStartStallDetectorStreamNotReady verifies that a stream reporting not-ready
// or zero bytes increments the stall counter and triggers a restart.
func TestStartStallDetectorStreamNotReady(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Fake MediaMTX: stream not ready, no data.
	fakeServer := newFakeMediaMTXServer(t, func(name string) mediamtxPathResponse {
		return mediamtxPathResponse{Name: name, Ready: false, BytesReceived: 0}
	})
	defer fakeServer.Close()

	sup := supervisor.New(supervisor.Config{ShutdownTimeout: 2 * time.Second})
	devName := "notready_device"
	svc := &mockService{name: devName}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("sup.Add: %v", err)
	}
	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()
	go func() { _ = sup.Run(supCtx) }()
	time.Sleep(50 * time.Millisecond)

	cfg := config.DefaultConfig()
	cfg.MediaMTX.APIURL = fakeServer.URL
	cfg.Monitor.StallCheckInterval = 50 * time.Millisecond
	cfg.Monitor.MaxStallChecks = 1
	cfg.Monitor.RestartUnhealthy = true

	var mu sync.RWMutex
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: "hash"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		startStallDetector(ctx, logger, cfg, sup, &mu, services, hashes)
		close(done)
	}()

	// 2 ticks: tick1 notReady → stallCount=1 >= max=1 → restart (Remove + delete maps).
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startStallDetector did not exit")
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("stream not ready or no data")) {
		t.Logf("log: %s", logBuf.String())
		t.Error("expected 'stream not ready or no data' log")
	}
}

// TestStartStallDetectorHealthyStream verifies that a stream with increasing
// byte counts is not marked as stalled (stall counter resets to zero).
func TestStartStallDetectorHealthyStream(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	var callCount int
	var mu sync.Mutex
	// Fake MediaMTX: bytes increase with each call (healthy stream).
	fakeServer := newFakeMediaMTXServer(t, func(name string) mediamtxPathResponse {
		mu.Lock()
		callCount++
		bytes := int64(callCount) * 1000
		mu.Unlock()
		return mediamtxPathResponse{Name: name, Ready: true, BytesReceived: bytes}
	})
	defer fakeServer.Close()

	sup := supervisor.New(supervisor.Config{})
	devName := "healthy_device"
	svc := &mockService{name: devName}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("sup.Add: %v", err)
	}
	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()
	go func() { _ = sup.Run(supCtx) }()
	time.Sleep(50 * time.Millisecond)

	cfg := config.DefaultConfig()
	cfg.MediaMTX.APIURL = fakeServer.URL
	cfg.Monitor.StallCheckInterval = 50 * time.Millisecond
	cfg.Monitor.MaxStallChecks = 3
	cfg.Monitor.RestartUnhealthy = true

	var statsMu sync.RWMutex
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: "hash"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		startStallDetector(ctx, logger, cfg, sup, &statsMu, services, hashes)
		close(done)
	}()

	// Run several ticks — bytes always increase so no stall should be detected.
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startStallDetector did not exit")
	}

	// Device should still be registered (no stall restart triggered).
	statsMu.RLock()
	_, stillRegistered := services[devName]
	statsMu.RUnlock()

	if !stillRegistered {
		t.Error("healthy stream should not be restarted")
	}

	// Verify the log does NOT contain stall warnings.
	if bytes.Contains(logBuf.Bytes(), []byte("stream data stalled")) {
		t.Errorf("healthy stream should not generate stall warning, got: %s", logBuf.String())
	}

	if callCount == 0 {
		t.Errorf("fake MediaMTX server was never called")
	}
}
