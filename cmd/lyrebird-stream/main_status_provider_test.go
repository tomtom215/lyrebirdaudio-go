package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/health"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestSupervisorStatusProvider_NoServices verifies the P-4 fix:
// supervisorStatusProvider returns empty services when supervisor has none.
func TestSupervisorStatusProvider_NoServices(t *testing.T) {
	sup := supervisor.New(supervisor.Config{
		ShutdownTimeout: 5 * time.Second,
	})

	provider := &supervisorStatusProvider{sup: sup}
	services := provider.Services()

	if len(services) != 0 {
		t.Errorf("Services() returned %d services, want 0", len(services))
	}
}

// TestSupervisorStatusProvider_WithServices verifies the P-4 fix:
// supervisorStatusProvider correctly maps supervisor state to health.ServiceInfo.
func TestSupervisorStatusProvider_WithServices(t *testing.T) {
	sup := supervisor.New(supervisor.Config{
		ShutdownTimeout: 5 * time.Second,
	})

	// Add a mock service that blocks until context is cancelled.
	svc := &mockService{name: "test_device"}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	provider := &supervisorStatusProvider{sup: sup}
	services := provider.Services()

	if len(services) != 1 {
		t.Fatalf("Services() returned %d services, want 1", len(services))
	}

	if services[0].Name != "test_device" {
		t.Errorf("Services()[0].Name = %q, want %q", services[0].Name, "test_device")
	}
}

// TestSupervisorStatusProvider_HealthyMapping verifies that running services
// are mapped to healthy=true in the health endpoint response.
func TestSupervisorStatusProvider_HealthyMapping(t *testing.T) {
	sup := supervisor.New(supervisor.Config{
		ShutdownTimeout: 5 * time.Second,
	})

	svc := &mockService{name: "running_device"}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	// Start the supervisor so the service transitions to running.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = sup.Run(ctx)
	}()
	// Give the service time to start.
	time.Sleep(100 * time.Millisecond)

	provider := &supervisorStatusProvider{sup: sup}
	services := provider.Services()

	cancel()

	if len(services) != 1 {
		t.Fatalf("Services() returned %d services, want 1", len(services))
	}

	if services[0].State != "running" {
		t.Errorf("Services()[0].State = %q, want %q", services[0].State, "running")
	}
	if !services[0].Healthy {
		t.Error("Services()[0].Healthy = false, want true for running service")
	}
}

// TestSupervisorStatusProvider_FailedService verifies that failed services
// include the error message and healthy=false.
func TestSupervisorStatusProvider_FailedService(t *testing.T) {
	sup := supervisor.New(supervisor.Config{
		ShutdownTimeout: 5 * time.Second,
	})

	svc := &mockService{name: "failing_device", err: errors.New("device disconnected")}
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	// Start supervisor and let the service fail.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = sup.Run(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	provider := &supervisorStatusProvider{sup: sup}
	services := provider.Services()

	cancel()

	if len(services) != 1 {
		t.Fatalf("Services() returned %d services, want 1", len(services))
	}

	if services[0].Healthy {
		t.Error("Services()[0].Healthy = true, want false for failed service")
	}
	if services[0].Error == "" {
		t.Error("Services()[0].Error should be non-empty for failed service")
	}
}

// TestSupervisorStatusProvider_ImplementsInterface verifies that
// supervisorStatusProvider satisfies health.StatusProvider at compile time.
func TestSupervisorStatusProvider_ImplementsInterface(t *testing.T) {
	sup := supervisor.New(supervisor.Config{})
	var _ health.StatusProvider = &supervisorStatusProvider{sup: sup}
}

// mockService is a minimal supervisor.Service for testing.
type mockService struct {
	name string
	err  error
}

func (m *mockService) Name() string { return m.name }

func (m *mockService) Run(ctx context.Context) error {
	if m.err != nil {
		return m.err
	}
	<-ctx.Done()
	return ctx.Err()
}
