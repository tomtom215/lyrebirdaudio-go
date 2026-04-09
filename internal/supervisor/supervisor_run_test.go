package supervisor

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSupervisor_Run(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
	})

	svc := newMockService("service1")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
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

	// Verify service is running
	if got := svc.runCount.Load(); got != 1 {
		t.Errorf("runCount = %d, want 1", got)
	}

	// Cancel to trigger shutdown
	cancel()

	// Wait for supervisor to stop
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run: unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop in time")
	}

	// Wait for service to stop
	select {
	case <-svc.stopped:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("service did not stop in time")
	}
}

func TestSupervisor_Run_AlreadyRunning(t *testing.T) {
	sup := New(DefaultConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start supervisor
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = sup.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Try to run again - should fail
	err := sup.Run(ctx)
	if err == nil {
		t.Error("Run twice: expected error, got nil")
	}

	cancel()
	wg.Wait()
}

func TestSupervisor_GracefulShutdown(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 5 * time.Second,
	})

	// Add multiple services
	services := make([]*mockService, 3)
	for i := range services {
		services[i] = newMockService(string(rune('a' + i)))
		if err := sup.Add(services[i]); err != nil {
			t.Fatalf("Add service %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for all services to start
	for i, svc := range services {
		select {
		case <-svc.started:
			// OK
		case <-time.After(2 * time.Second):
			t.Fatalf("service %d did not start in time", i)
		}
	}

	// Trigger shutdown
	cancel()

	// Wait for supervisor
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run: unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("supervisor did not stop in time")
	}

	// Verify all services stopped
	for i, svc := range services {
		select {
		case <-svc.stopped:
			// OK
		case <-time.After(1 * time.Second):
			t.Errorf("service %d did not stop", i)
		}
	}
}

// Test that services with long-running operations can be properly stopped
func TestSupervisor_LongRunningServiceShutdown(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
	})

	svc := newMockService("long-running")
	// Service will run for 1 hour unless cancelled
	svc.runDelay = 1 * time.Hour

	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
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

	// Cancel immediately
	cancel()

	// Should stop within shutdown timeout
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run: unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop within timeout")
	}

	// Service should have stopped
	select {
	case <-svc.stopped:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("service did not stop")
	}
}

// TestSupervisor_StopServeCancelRace is the C-4 regression test.
//
// suture may call Stop() on a serviceWrapper concurrently with (or just
// before) Serve() assigns the cancel function.  Without the mutex in
// serviceWrapper, this is a data race.  This test exercises rapid
// add-and-remove cycles under the race detector.
func TestSupervisor_StopServeCancelRace(t *testing.T) {
	const iterations = 50

	for i := 0; i < iterations; i++ {
		sup := New(Config{
			ShutdownTimeout: 2 * time.Second,
		})

		svc := newMockService("race-test")

		if err := sup.Add(svc); err != nil {
			t.Fatalf("iteration %d: Add: %v", i, err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		errCh := make(chan error, 1)
		go func() {
			errCh <- sup.Run(ctx)
		}()

		// Cancel almost immediately to force Serve/Stop overlap.
		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("iteration %d: Run: unexpected error: %v", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d: supervisor did not stop", i)
		}
	}
}

// TestSupervisor_StopCoverage verifies that Stop() is exercised under the
// race detector when Remove() is called while the service is running (L-1).
func TestSupervisor_StopCoverage(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
	})

	svc := newMockService("stop-coverage")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for the service to start so that Stop() has a non-nil cancel to call.
	select {
	case <-svc.started:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not start")
	}

	// Remove while running: suture calls Stop() on the wrapper.
	if err := sup.Remove("stop-coverage"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	select {
	case <-svc.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not stop after Remove")
	}

	cancel()
	<-errCh
}
