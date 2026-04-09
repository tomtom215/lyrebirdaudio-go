package supervisor

import (
	"context"
	"testing"
	"time"
)

func TestSupervisor_Add(t *testing.T) {
	sup := New(DefaultConfig())

	svc1 := newMockService("service1")
	svc2 := newMockService("service2")

	// Add first service
	if err := sup.Add(svc1); err != nil {
		t.Errorf("Add first service: unexpected error: %v", err)
	}

	// Add second service
	if err := sup.Add(svc2); err != nil {
		t.Errorf("Add second service: unexpected error: %v", err)
	}

	// Service count should be 2
	if got := sup.ServiceCount(); got != 2 {
		t.Errorf("ServiceCount = %d, want 2", got)
	}

	// Adding duplicate should fail
	dup := newMockService("service1")
	if err := sup.Add(dup); err == nil {
		t.Error("Add duplicate: expected error, got nil")
	}
}

func TestSupervisor_Remove(t *testing.T) {
	sup := New(DefaultConfig())

	svc := newMockService("service1")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Remove existing service
	if err := sup.Remove("service1"); err != nil {
		t.Errorf("Remove: unexpected error: %v", err)
	}

	// Service count should be 0
	if got := sup.ServiceCount(); got != 0 {
		t.Errorf("ServiceCount = %d, want 0", got)
	}

	// Remove non-existent service should fail
	if err := sup.Remove("nonexistent"); err == nil {
		t.Error("Remove nonexistent: expected error, got nil")
	}
}

func TestSupervisor_Status(t *testing.T) {
	sup := New(DefaultConfig())

	svc := newMockService("service1")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	status := sup.Status()
	if len(status) != 1 {
		t.Fatalf("Status length = %d, want 1", len(status))
	}

	if status[0].Name != "service1" {
		t.Errorf("Name = %q, want %q", status[0].Name, "service1")
	}
	if status[0].State != ServiceStateIdle {
		t.Errorf("State = %v, want %v", status[0].State, ServiceStateIdle)
	}
}

func TestSupervisor_AddWhileRunning(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start supervisor
	go func() {
		_ = sup.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Add service while running
	svc := newMockService("late-service")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add while running: %v", err)
	}

	// Wait for service to start
	select {
	case <-svc.started:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("service did not start in time")
	}

	// Verify it's running
	status := sup.Status()
	found := false
	for _, s := range status {
		if s.Name == "late-service" && s.State == ServiceStateRunning {
			found = true
			break
		}
	}
	if !found {
		t.Error("late-service not found or not running")
	}
}

func TestSupervisor_RemoveWhileRunning(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
	})

	svc := newMockService("removeme")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start supervisor
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

	// Remove service while running
	if err := sup.Remove("removeme"); err != nil {
		t.Errorf("Remove while running: %v", err)
	}

	// Service should stop
	select {
	case <-svc.stopped:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("service did not stop after removal")
	}

	// Service count should be 0
	if got := sup.ServiceCount(); got != 0 {
		t.Errorf("ServiceCount = %d, want 0", got)
	}
}

// TestRemoveWaitsForServiceStop verifies M-4 fix: Remove() blocks until the
// service's Serve() goroutine returns, preventing races when re-registering.
func TestRemoveWaitsForServiceStop(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 5 * time.Second,
	})

	svc := &slowStopService{
		name:    "slow-stop",
		started: make(chan struct{}, 1),
		stopped: make(chan struct{}, 1),
	}

	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for service to start
	select {
	case <-svc.started:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not start")
	}

	// Remove should block until the service's Serve() returns.
	// The slowStopService takes 200ms to stop.
	start := time.Now()
	if err := sup.Remove("slow-stop"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited for service stop (at least ~200ms)
	if elapsed < 100*time.Millisecond {
		t.Errorf("M-4: Remove() returned too quickly (%v), should wait for service stop", elapsed)
	}

	// Service should be fully stopped
	select {
	case <-svc.stopped:
		// Good - service stopped
	default:
		t.Error("M-4: service Serve() did not complete before Remove() returned")
	}

	cancel()
	<-errCh
}
