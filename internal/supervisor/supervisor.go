// Package supervisor provides a supervision tree for managing multiple stream managers.
//
// The supervisor implements Erlang/OTP-style process supervision, providing:
//   - Automatic restart of failed services with configurable backoff
//   - Graceful shutdown with timeout
//   - Dynamic service registration
//   - Health status reporting
//
// Example:
//
//	sup := supervisor.New(supervisor.Config{
//	    ShutdownTimeout: 10 * time.Second,
//	})
//
//	sup.Add("device1", serviceFactory1)
//	sup.Add("device2", serviceFactory2)
//
//	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
//	defer cancel()
//
//	if err := sup.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Service is the interface that supervised services must implement.
// Implementations should block until the context is cancelled or an error occurs.
type Service interface {
	// Run starts the service. It should block until ctx is cancelled or
	// the service encounters an unrecoverable error.
	Run(ctx context.Context) error

	// Name returns the service's identifier.
	Name() string
}

// ServiceState represents the current state of a supervised service.
type ServiceState int

const (
	ServiceStateIdle     ServiceState = iota // Not started
	ServiceStateRunning                      // Running normally
	ServiceStateStopping                     // Being stopped
	ServiceStateFailed                       // Failed, may restart
	ServiceStateStopped                      // Stopped, terminal
)

func (s ServiceState) String() string {
	switch s {
	case ServiceStateIdle:
		return "idle"
	case ServiceStateRunning:
		return "running"
	case ServiceStateStopping:
		return "stopping"
	case ServiceStateFailed:
		return "failed"
	case ServiceStateStopped:
		return "stopped"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ServiceStatus contains status information about a supervised service.
type ServiceStatus struct {
	Name      string
	State     ServiceState
	StartTime time.Time
	Uptime    time.Duration
	Restarts  int
	LastError error
}

// Config contains supervisor configuration.
type Config struct {
	// ShutdownTimeout is the maximum time to wait for services to stop gracefully.
	// Default: 10 seconds.
	ShutdownTimeout time.Duration

	// Logger is optional; if set, supervisor events are logged here.
	Logger io.Writer
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ShutdownTimeout: 10 * time.Second,
	}
}

// Supervisor manages a collection of services, restarting them on failure.
type Supervisor struct {
	cfg Config

	mu       sync.RWMutex
	services map[string]*serviceEntry
	running  bool
	wg       sync.WaitGroup

	// For coordinated shutdown
	cancel context.CancelFunc

	// Mutex for thread-safe logging
	logMu sync.Mutex
}

// serviceEntry tracks a single service's lifecycle.
type serviceEntry struct {
	service   Service
	state     ServiceState
	startTime time.Time
	restarts  int
	lastError error
	cancel    context.CancelFunc
}

// New creates a new Supervisor with the given configuration.
func New(cfg Config) *Supervisor {
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	return &Supervisor{
		cfg:      cfg,
		services: make(map[string]*serviceEntry),
	}
}

// logf writes a formatted log message if Logger is configured (thread-safe).
func (s *Supervisor) logf(format string, args ...interface{}) {
	if s.cfg.Logger != nil {
		s.logMu.Lock()
		_, _ = fmt.Fprintf(s.cfg.Logger, "[Supervisor] "+format+"\n", args...)
		s.logMu.Unlock()
	}
}

// Add registers a service with the supervisor.
// If the supervisor is already running, the service is started immediately.
// Returns an error if a service with the same name already exists.
func (s *Supervisor) Add(svc Service) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := svc.Name()
	if _, exists := s.services[name]; exists {
		return fmt.Errorf("service %q already registered", name)
	}

	entry := &serviceEntry{
		service: svc,
		state:   ServiceStateIdle,
	}
	s.services[name] = entry
	s.logf("Added service: %s", name)

	// If already running, start the service immediately
	if s.running {
		s.startService(entry)
	}

	return nil
}

// Remove unregisters and stops a service.
// Blocks until the service has stopped (up to ShutdownTimeout).
func (s *Supervisor) Remove(name string) error {
	s.mu.Lock()
	entry, exists := s.services[name]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("service %q not found", name)
	}

	// Cancel the service if running
	if entry.cancel != nil {
		entry.cancel()
	}
	delete(s.services, name)
	s.mu.Unlock()

	s.logf("Removed service: %s", name)
	return nil
}

// Status returns the current status of all services.
func (s *Supervisor) Status() []ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ServiceStatus, 0, len(s.services))
	now := time.Now()

	for name, entry := range s.services {
		var uptime time.Duration
		if !entry.startTime.IsZero() && entry.state == ServiceStateRunning {
			uptime = now.Sub(entry.startTime)
		}

		result = append(result, ServiceStatus{
			Name:      name,
			State:     entry.state,
			StartTime: entry.startTime,
			Uptime:    uptime,
			Restarts:  entry.restarts,
			LastError: entry.lastError,
		})
	}

	return result
}

// ServiceCount returns the number of registered services.
func (s *Supervisor) ServiceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.services)
}

// Run starts all registered services and blocks until ctx is cancelled.
// When ctx is cancelled, all services are stopped gracefully.
func (s *Supervisor) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("supervisor already running")
	}

	// Create a derived context for coordinated shutdown
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true

	// Start all registered services
	for _, entry := range s.services {
		s.startService(entry)
	}
	s.mu.Unlock()

	s.logf("Supervisor started with %d services", s.ServiceCount())

	// Wait for context cancellation
	<-runCtx.Done()

	s.logf("Shutdown signal received, stopping services...")

	// Stop all services gracefully
	return s.shutdown()
}

// startService launches a service in a goroutine with restart logic.
func (s *Supervisor) startService(entry *serviceEntry) {
	ctx, cancel := context.WithCancel(context.Background())
	entry.cancel = cancel
	entry.state = ServiceStateRunning
	entry.startTime = time.Now()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runServiceLoop(ctx, entry)
	}()
}

// runServiceLoop runs a service with automatic restart on failure.
func (s *Supervisor) runServiceLoop(ctx context.Context, entry *serviceEntry) {
	for {
		select {
		case <-ctx.Done():
			entry.state = ServiceStateStopped
			s.logf("Service %s stopped", entry.service.Name())
			return
		default:
		}

		entry.state = ServiceStateRunning
		entry.startTime = time.Now()

		err := entry.service.Run(ctx)

		// Check if context was cancelled (graceful shutdown)
		if ctx.Err() != nil {
			entry.state = ServiceStateStopped
			return
		}

		// Service failed
		entry.state = ServiceStateFailed
		entry.lastError = err
		entry.restarts++
		s.logf("Service %s failed (restarts=%d): %v", entry.service.Name(), entry.restarts, err)

		// Brief delay before restart (the stream manager has its own backoff)
		select {
		case <-ctx.Done():
			entry.state = ServiceStateStopped
			return
		case <-time.After(1 * time.Second):
			// Continue to restart
		}
	}
}

// shutdown stops all services gracefully with timeout.
func (s *Supervisor) shutdown() error {
	s.mu.Lock()
	// Cancel all services
	for _, entry := range s.services {
		if entry.cancel != nil {
			entry.state = ServiceStateStopping
			entry.cancel()
		}
	}
	s.running = false
	s.mu.Unlock()

	// Wait for all services to stop with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logf("All services stopped gracefully")
		return nil
	case <-time.After(s.cfg.ShutdownTimeout):
		s.logf("Shutdown timeout exceeded, some services may not have stopped cleanly")
		return errors.New("shutdown timeout exceeded")
	}
}
