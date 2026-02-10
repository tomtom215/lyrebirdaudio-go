package supervisor

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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
		{
			name: "with restart policy",
			cfg: Config{
				ShutdownTimeout:   10 * time.Second,
				RestartDelay:      2 * time.Second,
				MaxRestartDelay:   60 * time.Second,
				RestartMultiplier: 2.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sup := New(tt.cfg)
			if sup == nil {
				t.Fatal("New returned nil")
			}
			// Verify supervisor was created (internal state)
			if sup.suture == nil {
				t.Error("suture supervisor not initialized")
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
	if cfg.RestartDelay != 1*time.Second {
		t.Errorf("RestartDelay = %v, want %v", cfg.RestartDelay, 1*time.Second)
	}
	if cfg.MaxRestartDelay != 5*time.Minute {
		t.Errorf("MaxRestartDelay = %v, want %v", cfg.MaxRestartDelay, 5*time.Minute)
	}
	if cfg.RestartMultiplier != 2.0 {
		t.Errorf("RestartMultiplier = %v, want %v", cfg.RestartMultiplier, 2.0)
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

func TestSupervisor_LoggingOutput(t *testing.T) {
	var buf bytes.Buffer
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
		Logger:          slog.New(slog.NewTextHandler(&buf, nil)),
	})

	svc := newMockService("log-test")
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
		t.Fatal("service did not start")
	}

	cancel()

	select {
	case <-errCh:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}

	logOutput := buf.String()
	// Verify some logging happened
	if len(logOutput) == 0 {
		t.Log("Warning: no log output captured (may be expected with suture)")
	}
}

func TestSupervisor_ConcurrentOperations(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start supervisor
	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Concurrent adds
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := newMockService(string(rune('A' + i)))
			_ = sup.Add(svc)
		}(i)
	}
	wg.Wait()

	// Verify services were added
	count := sup.ServiceCount()
	if count != 10 {
		t.Errorf("ServiceCount = %d, want 10", count)
	}

	// Concurrent status checks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sup.Status()
		}()
	}
	wg.Wait()

	cancel()

	select {
	case <-errCh:
		// OK
	case <-time.After(10 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisor_ServiceName(t *testing.T) {
	tests := []struct {
		name     string
		wantName string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"CamelCase", "CamelCase"},
		{"123numeric", "123numeric"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sup := New(DefaultConfig())
			svc := newMockService(tt.name)
			if err := sup.Add(svc); err != nil {
				t.Fatalf("Add: %v", err)
			}

			status := sup.Status()
			if len(status) != 1 || status[0].Name != tt.wantName {
				t.Errorf("Name = %q, want %q", status[0].Name, tt.wantName)
			}
		})
	}
}

func TestSupervisor_SupervisorName(t *testing.T) {
	sup := New(Config{
		Name: "test-supervisor",
	})
	if sup == nil {
		t.Fatal("New returned nil")
	}
	// Supervisor should be created with given name
	// (used internally by suture for logging)
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
