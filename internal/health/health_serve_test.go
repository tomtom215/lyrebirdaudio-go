package health

import (
	"context"
	"testing"
	"time"
)

func TestListenAndServe(t *testing.T) {
	h := NewHandler(&mockProvider{
		services: []ServiceInfo{{Name: "x", State: "running", Healthy: true}},
	})

	// Use port 0 to get a random available port
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServe(ctx, "127.0.0.1:0", h)
	}()

	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}
}

// TestListenAndServeReady verifies the ready channel is closed when the server
// is bound and accepting connections.
func TestListenAndServeReady(t *testing.T) {
	h := NewHandler(&mockProvider{
		services: []ServiceInfo{{Name: "x", State: "running", Healthy: true}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeReady(ctx, "127.0.0.1:0", h, ready)
	}()

	// Wait for ready signal
	select {
	case <-ready:
		// Server is bound and listening - success
	case <-time.After(2 * time.Second):
		t.Fatal("H-3: ready channel was not closed within timeout")
	}

	cancel()
	<-errCh
}

// TestListenAndServeReadyBindFailure verifies that bind failures are returned
// as errors instead of being silently swallowed.
func TestListenAndServeReadyBindFailure(t *testing.T) {
	h := NewHandler(&mockProvider{})

	// First, bind a port so the second attempt fails
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	ready1 := make(chan struct{})
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- ListenAndServeReady(ctx1, "127.0.0.1:19998", h, ready1)
	}()

	select {
	case <-ready1:
		// First server is listening
	case <-time.After(2 * time.Second):
		t.Fatal("first server did not start")
	}

	// Try to bind the same port - should fail immediately
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	err := ListenAndServeReady(ctx2, "127.0.0.1:19998", h, nil)
	if err == nil {
		t.Fatal("H-3: expected bind error for duplicate port, got nil")
	}

	cancel1()
	<-errCh1
}

// TestListenAndServeReadyNilReady verifies backward compatibility when
// ready channel is nil.
func TestListenAndServeReadyNilReady(t *testing.T) {
	h := NewHandler(&mockProvider{
		services: []ServiceInfo{{Name: "x", State: "running", Healthy: true}},
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServeReady(ctx, "127.0.0.1:0", h, nil)
	}()

	// Give the server time to start
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServeReady returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not return after cancel")
	}
}
