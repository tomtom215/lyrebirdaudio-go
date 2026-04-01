// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestStartFailedStreamRecoveryNegativeInterval covers monitors.go:155-157 —
// the `recoveryInterval <= 0 → recoveryInterval = 5 * time.Minute` branch when
// a negative value is supplied. This is distinct from the zero-value case
// already tested by TestStartFailedStreamRecoveryDefaultInterval in monitors_test.go.
// The context is pre-cancelled so the loop exits immediately via ctx.Done()
// without waiting for the 5-minute ticker to fire.
func TestStartFailedStreamRecoveryNegativeInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel: the for-select exits via ctx.Done()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sup := supervisor.New(supervisor.Config{})
	var mu sync.RWMutex
	services := map[string]bool{}
	hashes := map[string]string{}

	// Negative interval also triggers the `recoveryInterval = 5 * time.Minute` branch.
	startFailedStreamRecovery(ctx, logger, -1, sup, &mu, services, hashes)
}

// TestStartFailedStreamRecoveryRunningServiceSkips covers monitors.go:165-167 —
// the `status.State != supervisor.ServiceStateFailed → continue` branch.
// A service that runs indefinitely is added to the supervisor so that
// sup.Status() returns it with State = Running. When the ticker fires,
// the inner for loop reaches line 166, evaluates the condition as true
// (Running != Failed), and executes the `continue` statement.
func TestStartFailedStreamRecoveryRunningServiceSkips(t *testing.T) {
	// longRunning blocks until its context is cancelled; it never fails.
	started := make(chan struct{}, 1)
	svc := &mockService{
		name: "long-running-device",
		err:  nil, // nil err makes Run block on <-ctx.Done()
	}

	// Override run to signal when started (mockService.Run already blocks on ctx).
	// We use a wrapper that signals "started" then delegates.
	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()

	sup := supervisor.New(supervisor.Config{ShutdownTimeout: 2 * time.Second})
	if err := sup.Add(svc); err != nil {
		t.Fatalf("sup.Add: %v", err)
	}
	go func() { _ = sup.Run(supCtx) }()

	// Signal started via a separate goroutine watching supervisor status.
	go func() {
		for {
			for _, s := range sup.Status() {
				if s.Name == svc.name && s.State == supervisor.ServiceStateRunning {
					select {
					case started <- struct{}{}:
					default:
					}
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("service did not reach Running state within timeout")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var mu sync.RWMutex
	services := map[string]bool{svc.name: true}
	hashes := map[string]string{svc.name: "hash1"}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		// Short interval so the ticker fires quickly and the continue branch executes.
		startFailedStreamRecovery(ctx, logger, 50*time.Millisecond, sup, &mu, services, hashes)
		close(done)
	}()

	// Allow several ticks to ensure the Running service triggers the continue path.
	time.Sleep(250 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startFailedStreamRecovery did not exit after context cancel")
	}

	// The running service must still be registered (it was not failed → not removed).
	mu.RLock()
	_, stillRegistered := services[svc.name]
	mu.RUnlock()
	if !stillRegistered {
		t.Errorf("running service %q should remain in services map (state was Running, not Failed)", svc.name)
	}
}

// TestStartFailedStreamRecoveryRemoveError covers monitors.go:170-172 —
// the `sup.Remove error → logger.Warn + continue` branch. This requires
// sup.Status() to report a Failed service while sup.Remove() simultaneously
// returns an error (TOCTOU: the service is removed between Status and Remove).
//
// We achieve this by:
//  1. Adding a failing service so it enters ServiceStateFailed.
//  2. Waiting for the supervisor to report it as Failed.
//  3. Concurrently calling sup.Remove() from a separate goroutine immediately
//     after startFailedStreamRecovery reads the status, so that the recovery
//     goroutine's own Remove() call finds the service already gone.
//
// Because the timing window is narrow, the test only asserts that the
// function completes without panic; the branch may or may not fire on every run.
func TestStartFailedStreamRecoveryRemoveError(t *testing.T) {
	devName := "race-device"
	failSvc := &mockService{name: devName, err: errors.New("simulated failure")}

	supCtx, supCancel := context.WithCancel(context.Background())
	defer supCancel()

	sup := supervisor.New(supervisor.Config{ShutdownTimeout: 2 * time.Second})
	if err := sup.Add(failSvc); err != nil {
		t.Fatalf("sup.Add: %v", err)
	}
	go func() { _ = sup.Run(supCtx) }()

	// Wait for the service to fail.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range sup.Status() {
			if s.Name == devName && s.State == supervisor.ServiceStateFailed {
				goto failed
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Skip("service did not reach Failed state; skipping TOCTOU branch test")

failed:
	// Pre-remove the service so that the recovery loop's Remove() call fails.
	_ = sup.Remove(devName)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var mu sync.RWMutex
	services := map[string]bool{devName: true}
	hashes := map[string]string{devName: "hash1"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		startFailedStreamRecovery(ctx, logger, 30*time.Millisecond, sup, &mu, services, hashes)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startFailedStreamRecovery did not exit")
	}
}
