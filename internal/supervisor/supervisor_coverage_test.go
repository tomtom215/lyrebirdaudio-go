// SPDX-License-Identifier: MIT

package supervisor

import (
	"context"
	"testing"
	"time"

	"github.com/thejerf/suture/v4"
)

// cleanExitService returns nil from Run() after a short delay without
// waiting for context cancellation. This exercises the else branch in
// serviceWrapper.Serve() at supervisor.go:213-218 — the "service exited
// unexpectedly (clean exit)" path.
type cleanExitService struct {
	name    string
	started chan struct{}
	delay   time.Duration
}

func newCleanExitService(name string, delay time.Duration) *cleanExitService {
	return &cleanExitService{
		name:    name,
		started: make(chan struct{}, 20),
		delay:   delay,
	}
}

func (s *cleanExitService) Name() string { return s.name }

func (s *cleanExitService) Run(ctx context.Context) error {
	select {
	case s.started <- struct{}{}:
	default:
	}
	select {
	case <-time.After(s.delay):
		// Return nil without context cancellation → triggers the else branch.
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestServiceCleanExitHitsElseBranch covers supervisor.go:213-218 — the
// else branch where a service's Serve() returns nil while the supervisor
// context is still active (clean but unexpected exit). The second restart
// proves the first invocation hit the else branch.
func TestServiceCleanExitHitsElseBranch(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout:   2 * time.Second,
		RestartDelay:      10 * time.Millisecond,
		MaxRestartDelay:   100 * time.Millisecond,
		RestartMultiplier: 1.5,
	})

	svc := newCleanExitService("clean-exit-svc", 30*time.Millisecond)
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for at least 2 starts. The second start proves the first run
	// triggered the else branch (clean exit → state=Failed → restart).
	for i := 0; i < 2; i++ {
		select {
		case <-svc.started:
			// OK
		case <-time.After(3 * time.Second):
			t.Fatalf("service did not start for iteration %d", i+1)
		}
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run: unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

// terminateSvc is a service whose Run() returns suture.ErrTerminateSupervisorTree,
// causing the suture supervisor to halt. The Supervisor.Run() then receives a
// non-nil, non-context.Canceled error, exercising the `return err` at
// supervisor.go:449.
type terminateSvc struct {
	name    string
	started chan struct{}
}

func (s *terminateSvc) Name() string { return s.name }

func (s *terminateSvc) Run(_ context.Context) error {
	select {
	case s.started <- struct{}{}:
	default:
	}
	return suture.ErrTerminateSupervisorTree
}

// TestSupervisorRunReturnsNonCanceledError covers supervisor.go:449 — the
// `return err` branch in Run() reached when suture.Serve returns something
// other than context.Canceled (here: ErrTerminateSupervisorTree).
func TestSupervisorRunReturnsNonCanceledError(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
	})

	svc := &terminateSvc{
		name:    "terminate-svc",
		started: make(chan struct{}, 1),
	}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(context.Background())
	}()

	select {
	case err := <-errCh:
		// Run() must return a non-nil error; ErrTerminateSupervisorTree is
		// propagated through suture.Serve and is not context.Canceled.
		if err == nil {
			t.Error("expected non-nil error from Run when service returns ErrTerminateSupervisorTree, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop after ErrTerminateSupervisorTree")
	}
}
