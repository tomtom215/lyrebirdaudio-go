package supervisor

import (
	"context"
	"sync/atomic"
	"time"
)

// mockService is a test service that can be controlled.
type mockService struct {
	name       string
	runCount   atomic.Int32
	shouldFail bool
	failErr    error
	runDelay   time.Duration
	started    chan struct{}
	stopped    chan struct{}
}

func newMockService(name string) *mockService {
	return &mockService{
		name:    name,
		started: make(chan struct{}, 10),
		stopped: make(chan struct{}, 10),
	}
}

func (m *mockService) Name() string {
	return m.name
}

func (m *mockService) Run(ctx context.Context) error {
	m.runCount.Add(1)
	m.started <- struct{}{}

	defer func() {
		m.stopped <- struct{}{}
	}()

	if m.shouldFail {
		return m.failErr
	}

	if m.runDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.runDelay):
			return nil
		}
	}

	<-ctx.Done()
	return ctx.Err()
}

// slowStopService takes a configurable time to stop for testing M-4.
type slowStopService struct {
	name    string
	started chan struct{}
	stopped chan struct{}
}

func (s *slowStopService) Name() string {
	return s.name
}

func (s *slowStopService) Run(ctx context.Context) error {
	s.started <- struct{}{}
	<-ctx.Done()
	// Simulate slow cleanup (e.g., FFmpeg flushing buffers)
	time.Sleep(200 * time.Millisecond)
	s.stopped <- struct{}{}
	return ctx.Err()
}
