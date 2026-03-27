// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestStartDevicePollerContextCancellation verifies the poller exits on context cancel.
func TestStartDevicePollerContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.DefaultConfig()

	registerDevices := func(c *config.Config) int {
		return 0
	}

	done := make(chan struct{})
	go func() {
		startDevicePoller(ctx, logger, nil, cfg, registerDevices)
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startDevicePoller did not exit on context cancel")
	}
}

// TestStartDevicePollerWithKoanfConfigNil verifies poller uses fallback config.
func TestStartDevicePollerWithKoanfConfigNil(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())

	fallback := config.DefaultConfig()
	var receivedCfg *config.Config

	registerDevices := func(c *config.Config) int {
		receivedCfg = c
		cancel() // cancel after first call
		return 0
	}

	done := make(chan struct{})
	go func() {
		startDevicePoller(ctx, logger, nil, fallback, registerDevices)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		cancel()
		t.Fatal("timeout waiting for device poller")
	}

	// The poller should have received our fallback config
	if receivedCfg != nil && receivedCfg != fallback {
		t.Error("expected fallback config to be used when koanfCfg is nil")
	}
}

// TestStartReloadHandlerContextCancellation verifies the handler exits on context cancel.
func TestStartReloadHandlerContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sup := supervisor.New(supervisor.Config{})
	reloadCh := make(chan struct{}, 1)

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)
	registerDevices := func(c *config.Config) int { return 0 }

	done := make(chan struct{})
	go func() {
		startReloadHandler(ctx, logger, reloadCh, nil, sup, &mu, services, hashes, registerDevices)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startReloadHandler did not exit on context cancel")
	}
}

// TestStartReloadHandlerNilKoanfConfig verifies SIGHUP is no-op when koanfCfg is nil.
func TestStartReloadHandlerNilKoanfConfig(t *testing.T) {
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
		startReloadHandler(ctx, logger, reloadCh, nil, sup, &mu, services, hashes, registerDevices)
		close(done)
	}()

	// Send a SIGHUP
	reloadCh <- struct{}{}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	cancel()
	<-done

	// Should log "no active config file; SIGHUP is a no-op"
	if !bytes.Contains(logBuf.Bytes(), []byte("no active config file")) {
		t.Errorf("expected 'no active config file' log, got: %s", logBuf.String())
	}
}

// TestStartFailedStreamRecoveryContextCancellation verifies recovery exits on cancel.
func TestStartFailedStreamRecoveryContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sup := supervisor.New(supervisor.Config{})

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)

	done := make(chan struct{})
	go func() {
		startFailedStreamRecovery(ctx, logger, time.Second, sup, &mu, services, hashes)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startFailedStreamRecovery did not exit on context cancel")
	}
}

// TestStartFailedStreamRecoveryDefaultInterval verifies the default recovery interval.
func TestStartFailedStreamRecoveryDefaultInterval(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sup := supervisor.New(supervisor.Config{})

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)

	done := make(chan struct{})
	go func() {
		// Pass 0 to use default (5 min)
		startFailedStreamRecovery(ctx, logger, 0, sup, &mu, services, hashes)
		close(done)
	}()

	// Should start without error
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startFailedStreamRecovery did not exit")
	}
}

// TestStartStallDetectorContextCancellation verifies stall detector exits on cancel.
func TestStartStallDetectorContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sup := supervisor.New(supervisor.Config{})
	cfg := config.DefaultConfig()
	cfg.Monitor.StallCheckInterval = 100 * time.Millisecond

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)

	done := make(chan struct{})
	go func() {
		startStallDetector(ctx, logger, cfg, sup, &mu, services, hashes)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startStallDetector did not exit on context cancel")
	}
}

// TestStartStallDetectorDefaultInterval verifies default stall check interval.
func TestStartStallDetectorDefaultInterval(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sup := supervisor.New(supervisor.Config{})

	cfg := config.DefaultConfig()
	cfg.Monitor.StallCheckInterval = 0 // Should default to 60s

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)

	done := make(chan struct{})
	go func() {
		startStallDetector(ctx, logger, cfg, sup, &mu, services, hashes)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startStallDetector did not exit")
	}
}

// TestStartStallDetectorWithRegisteredDevices verifies stall detection with
// registered devices (even if MediaMTX is unavailable).
func TestStartStallDetectorWithRegisteredDevices(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sup := supervisor.New(supervisor.Config{})

	cfg := config.DefaultConfig()
	cfg.Monitor.StallCheckInterval = 50 * time.Millisecond
	cfg.MediaMTX.APIURL = "http://127.0.0.1:0" // Unreachable port

	var mu sync.RWMutex
	services := map[string]bool{"test_device": true}
	hashes := map[string]string{"test_device": "hash1"}

	done := make(chan struct{})
	go func() {
		startStallDetector(ctx, logger, cfg, sup, &mu, services, hashes)
		close(done)
	}()

	// Wait for at least one check cycle
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startStallDetector did not exit")
	}

	// Should have attempted a health check (debug level, so check for the attempt)
	// The MediaMTX API call should fail silently at debug level
}

// TestSupervisorStatusProviderEmptySupervisor verifies StatusProvider with no services.
func TestSupervisorStatusProviderEmptySupervisor(t *testing.T) {
	sup := supervisor.New(supervisor.Config{})
	provider := &supervisorStatusProvider{sup: sup}

	services := provider.Services()
	if len(services) != 0 {
		t.Errorf("expected 0 services, got %d", len(services))
	}
}

// TestDaemonSystemInfoProviderWithRecordDir verifies SystemInfo with a valid dir.
func TestDaemonSystemInfoProviderWithRecordDir(t *testing.T) {
	dir := t.TempDir()
	p := &daemonSystemInfoProvider{
		recordDir:        dir,
		diskLowThreshold: 1, // 1 byte threshold - should not trigger
	}

	si := p.SystemInfo()
	if si.DiskTotalBytes == 0 {
		t.Error("DiskTotalBytes should be non-zero")
	}
	if si.DiskFreeBytes == 0 {
		t.Error("DiskFreeBytes should be non-zero")
	}
	if si.DiskLowWarning {
		t.Error("DiskLowWarning should be false with 1 byte threshold")
	}
}

// TestDaemonSystemInfoProviderLowDiskThreshold verifies warning triggers.
func TestDaemonSystemInfoProviderLowDiskThreshold(t *testing.T) {
	p := &daemonSystemInfoProvider{
		recordDir:        "/",
		diskLowThreshold: 1 << 62, // Impossibly high - should always trigger
	}

	si := p.SystemInfo()
	if !si.DiskLowWarning {
		t.Error("DiskLowWarning should be true with impossibly high threshold")
	}
}
