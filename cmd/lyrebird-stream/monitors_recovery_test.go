// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

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
