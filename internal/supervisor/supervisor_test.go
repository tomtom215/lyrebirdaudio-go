package supervisor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
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

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "default config",
			cfg:  DefaultConfig(),
		},
		{
			name: "custom timeout",
			cfg: Config{
				ShutdownTimeout: 5 * time.Second,
			},
		},
		{
			name: "zero timeout uses default",
			cfg:  Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sup := New(tt.cfg)
			if sup == nil {
				t.Fatal("New returned nil")
			}
			if sup.services == nil {
				t.Error("services map not initialized")
			}
		})
	}
}

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

func TestSupervisor_ServiceRestart(t *testing.T) {
	var buf bytes.Buffer
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
		Logger:          &buf,
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
	if !strings.Contains(logOutput, "failed") {
		t.Error("expected failure to be logged")
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

func TestServiceState_String(t *testing.T) {
	tests := []struct {
		state    ServiceState
		expected string
	}{
		{ServiceStateIdle, "idle"},
		{ServiceStateRunning, "running"},
		{ServiceStateStopping, "stopping"},
		{ServiceStateFailed, "failed"},
		{ServiceStateStopped, "stopped"},
		{ServiceState(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 10*time.Second)
	}
}
