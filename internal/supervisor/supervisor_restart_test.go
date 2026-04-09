package supervisor

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSupervisor_ServiceRestart(t *testing.T) {
	var buf bytes.Buffer
	sup := New(Config{
		ShutdownTimeout:   2 * time.Second,
		Logger:            slog.New(slog.NewTextHandler(&buf, nil)),
		RestartDelay:      50 * time.Millisecond, // Fast restart for testing
		MaxRestartDelay:   200 * time.Millisecond,
		RestartMultiplier: 1.5,
	})

	svc := newMockService("failing-service")
	svc.shouldFail = true
	svc.failErr = errors.New("intentional failure")

	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for a few restarts
	restartCount := 0
	timeout := time.After(5 * time.Second)
	for restartCount < 3 {
		select {
		case <-svc.started:
			restartCount++
		case <-timeout:
			t.Fatalf("service only started %d times, want at least 3", restartCount)
		}
	}

	// Verify restarts happened
	if got := svc.runCount.Load(); got < 3 {
		t.Errorf("runCount = %d, want >= 3", got)
	}

	cancel()

	// Wait for supervisor to fully stop before reading the buffer
	select {
	case <-errCh:
		// OK, supervisor stopped
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}

	// Now safe to read the buffer
	logOutput := buf.String()
	if !strings.Contains(logOutput, "failing-service") {
		t.Errorf("expected service name to be logged, got: %s", logOutput)
	}
}

func TestSupervisor_ServiceStatusUpdates(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
	})

	svc := newMockService("status-test")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Before running, status should be idle
	status := sup.Status()
	if len(status) != 1 || status[0].State != ServiceStateIdle {
		t.Errorf("Before run: State = %v, want %v", status[0].State, ServiceStateIdle)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for service to start
	select {
	case <-svc.started:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("service did not start in time")
	}

	// Allow a moment for state update
	time.Sleep(50 * time.Millisecond)

	// While running, status should be running
	status = sup.Status()
	if len(status) != 1 || status[0].State != ServiceStateRunning {
		t.Errorf("During run: State = %v, want %v", status[0].State, ServiceStateRunning)
	}

	// Verify uptime is being tracked
	if status[0].Uptime <= 0 {
		t.Errorf("Uptime should be > 0, got %v", status[0].Uptime)
	}

	cancel()

	// Wait for supervisor to stop
	select {
	case <-errCh:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisor_RestartCounter(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout:   2 * time.Second,
		RestartDelay:      10 * time.Millisecond,
		MaxRestartDelay:   50 * time.Millisecond,
		RestartMultiplier: 1.5,
	})

	svc := newMockService("restart-counter")
	svc.shouldFail = true
	svc.failErr = errors.New("test error")

	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for multiple restarts
	for i := 0; i < 5; i++ {
		select {
		case <-svc.started:
			// OK
		case <-time.After(2 * time.Second):
			t.Fatalf("restart %d did not happen", i)
		}
	}

	// Check restart counter
	status := sup.Status()
	if len(status) != 1 {
		t.Fatalf("Status length = %d, want 1", len(status))
	}
	if status[0].Restarts < 4 {
		t.Errorf("Restarts = %d, want >= 4", status[0].Restarts)
	}
	if status[0].LastError == nil || status[0].LastError.Error() != "test error" {
		t.Errorf("LastError = %v, want 'test error'", status[0].LastError)
	}

	cancel()

	select {
	case <-errCh:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}
