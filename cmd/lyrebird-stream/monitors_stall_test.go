// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

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

	// Read callCount under the same mutex the handler uses: after ctx is
	// cancelled the last in-flight handler goroutine may still be executing
	// callCount++ concurrently with this read, so an unsynchronized read races.
	mu.Lock()
	finalCount := callCount
	mu.Unlock()
	if finalCount == 0 {
		t.Errorf("fake MediaMTX server was never called")
	}
}
