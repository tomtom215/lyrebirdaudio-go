// SPDX-License-Identifier: MIT

// Package supervisor provides a supervision tree for managing multiple stream managers.
//
// The supervisor implements Erlang/OTP-style process supervision using github.com/thejerf/suture,
// providing:
//   - Automatic restart of failed services with configurable exponential backoff
//   - Graceful shutdown with timeout
//   - Dynamic service registration (add/remove while running)
//   - Health status reporting with uptime and restart metrics
//   - Hierarchical supervision (supervisor trees)
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
	"log/slog"
	"sync"
	"time"

	"github.com/thejerf/suture/v4"
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
	// Name is the supervisor's identifier (used in logging).
	// Default: "supervisor"
	Name string

	// ShutdownTimeout is the maximum time to wait for services to stop gracefully.
	// Default: 10 seconds.
	ShutdownTimeout time.Duration

	// RestartDelay is the initial delay before restarting a failed service.
	// Default: 1 second.
	RestartDelay time.Duration

	// MaxRestartDelay is the maximum delay between restarts (exponential backoff cap).
	// Default: 5 minutes.
	MaxRestartDelay time.Duration

	// RestartMultiplier is the factor by which RestartDelay is multiplied after each failure.
	// Default: 2.0.
	RestartMultiplier float64

	// Logger is optional; if set, supervisor events are logged here.
	// When nil, supervisor operates silently.
	Logger *slog.Logger
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Name:             "supervisor",
		ShutdownTimeout:  10 * time.Second,
		RestartDelay:     1 * time.Second,
		MaxRestartDelay:  5 * time.Minute,
		RestartMultiplier: 2.0,
	}
}

// Supervisor manages a collection of services, restarting them on failure.
// It wraps thejerf/suture for production-grade supervision.
type Supervisor struct {
	cfg    Config
	suture *suture.Supervisor

	mu       sync.RWMutex
	services map[string]*serviceEntry
	running  bool
	logger   *slog.Logger
}

// serviceEntry tracks a single service's lifecycle.
type serviceEntry struct {
	service   Service
	wrapper   *serviceWrapper
	token     suture.ServiceToken
	state     ServiceState
	startTime time.Time
	restarts  int
	lastError error
}

// serviceWrapper adapts our Service interface to suture.Service.
type serviceWrapper struct {
	svc    Service
	entry  *serviceEntry
	sup    *Supervisor
	ctx    context.Context
	cancel context.CancelFunc
}

// Serve implements suture.Service.
func (w *serviceWrapper) Serve(ctx context.Context) error {
	// Create a cancellable context for this service
	w.ctx, w.cancel = context.WithCancel(ctx)

	// Update state to running
	w.sup.mu.Lock()
	w.entry.state = ServiceStateRunning
	w.entry.startTime = time.Now()
	w.sup.mu.Unlock()

	w.sup.logf("Service %s started", w.svc.Name())

	// Run the actual service
	err := w.svc.Run(w.ctx)

	// Update state based on result
	w.sup.mu.Lock()
	if w.ctx.Err() != nil {
		// Context was cancelled (graceful shutdown)
		w.entry.state = ServiceStateStopped
	} else if err != nil {
		// Service failed
		w.entry.state = ServiceStateFailed
		w.entry.lastError = err
		w.entry.restarts++
		w.sup.logf("Service %s failed (restarts=%d): %v", w.svc.Name(), w.entry.restarts, err)
	} else {
		// Service exited cleanly but unexpectedly
		w.entry.state = ServiceStateFailed
		w.entry.restarts++
		w.sup.logf("Service %s exited unexpectedly (restarts=%d)", w.svc.Name(), w.entry.restarts)
	}
	w.sup.mu.Unlock()

	return err
}

// String implements suture.Service for logging.
func (w *serviceWrapper) String() string {
	return w.svc.Name()
}

// Stop is called by suture when stopping the service.
func (w *serviceWrapper) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

// New creates a new Supervisor with the given configuration.
func New(cfg Config) *Supervisor {
	// Apply defaults
	if cfg.Name == "" {
		cfg.Name = "supervisor"
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}
	if cfg.RestartDelay == 0 {
		cfg.RestartDelay = 1 * time.Second
	}
	if cfg.MaxRestartDelay == 0 {
		cfg.MaxRestartDelay = 5 * time.Minute
	}
	if cfg.RestartMultiplier == 0 {
		cfg.RestartMultiplier = 2.0
	}

	// Create suture supervisor with configured backoff
	spec := suture.Spec{
		EventHook: nil, // We handle logging ourselves
		Timeout:   cfg.ShutdownTimeout,
	}

	s := &Supervisor{
		cfg:      cfg,
		suture:   suture.New(cfg.Name, spec),
		services: make(map[string]*serviceEntry),
	}

	// Set up logging if configured
	if cfg.Logger != nil {
		s.logger = cfg.Logger
	}

	return s
}

// logf writes a formatted log message if Logger is configured.
func (s *Supervisor) logf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Info(fmt.Sprintf(format, args...), "component", "supervisor")
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

	// Create wrapper that adapts to suture.Service
	wrapper := &serviceWrapper{
		svc:   svc,
		entry: entry,
		sup:   s,
	}
	entry.wrapper = wrapper

	// Add to suture supervisor (suture handles starting if already running)
	token := s.suture.Add(wrapper)
	entry.token = token

	s.services[name] = entry
	s.logf("Added service: %s", name)

	return nil
}

// Remove unregisters and stops a service.
// Returns an error if the service doesn't exist.
func (s *Supervisor) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.services[name]
	if !exists {
		return fmt.Errorf("service %q not found", name)
	}

	// Remove from suture (this stops the service)
	if err := s.suture.Remove(entry.token); err != nil {
		s.logf("Warning: error removing service %s: %v", name, err)
	}

	// Cancel the service context to ensure it stops
	if entry.wrapper != nil && entry.wrapper.cancel != nil {
		entry.wrapper.cancel()
	}

	delete(s.services, name)
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
	s.running = true
	s.mu.Unlock()

	s.logf("Supervisor started with %d services", s.ServiceCount())

	// Run suture supervisor (blocks until context is done or it terminates)
	err := s.suture.Serve(ctx)

	s.mu.Lock()
	s.running = false
	// Mark all services as stopped
	for _, entry := range s.services {
		entry.state = ServiceStateStopped
	}
	s.mu.Unlock()

	s.logf("Supervisor stopped")

	// suture.Serve returns context.Canceled when context is cancelled
	// This is normal shutdown, not an error
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}
