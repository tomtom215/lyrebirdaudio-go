// SPDX-License-Identifier: MIT

// Package health provides an HTTP health check endpoint for the lyrebird-stream daemon.
//
// The health check exposes service status at /healthz as JSON, suitable for
// systemd watchdog, load balancer probes, or monitoring systems.
package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

// ServiceInfo describes the health state of a single stream service.
type ServiceInfo struct {
	Name    string        `json:"name"`
	State   string        `json:"state"`
	Uptime  time.Duration `json:"uptime_ns"`
	Healthy bool          `json:"healthy"`
	Error   string        `json:"error,omitempty"`
}

// StatusProvider returns the current health status of all services.
// The daemon implements this interface to supply live data.
type StatusProvider interface {
	Services() []ServiceInfo
}

// Response is the JSON body returned by the health endpoint.
type Response struct {
	Status    string        `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Services  []ServiceInfo `json:"services"`
}

// Handler serves the /healthz endpoint.
type Handler struct {
	provider StatusProvider
}

// NewHandler creates a health check HTTP handler.
func NewHandler(provider StatusProvider) *Handler {
	return &Handler{provider: provider}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	resp := Response{
		Timestamp: time.Now(),
	}

	var services []ServiceInfo
	if h.provider != nil {
		services = h.provider.Services()
	}
	resp.Services = services

	healthy := len(services) > 0
	for _, svc := range services {
		if !svc.Healthy {
			healthy = false
			break
		}
	}

	if healthy {
		resp.Status = "healthy"
	} else {
		resp.Status = "unhealthy"
	}

	w.Header().Set("Content-Type", "application/json")
	if healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// ListenAndServe starts the health check HTTP server on the given address.
// It shuts down gracefully when ctx is cancelled.
//
// H-3 fix: The function now binds the listener synchronously before returning
// to the caller via the ready channel, so bind failures (e.g., port already in
// use) are detected immediately rather than being silently swallowed in a
// goroutine. If ready is nil, the function blocks as before.
func ListenAndServe(ctx context.Context, addr string, handler http.Handler) error {
	return ListenAndServeReady(ctx, addr, handler, nil)
}

// ListenAndServeReady starts the health check HTTP server and signals readiness.
//
// H-3 fix: Binds the listener synchronously. If the bind fails, the error is
// returned immediately. Once the server is listening, the ready channel is
// closed (if non-nil) to signal that the endpoint is available. This allows
// the daemon to verify the health endpoint is actually listening before
// completing initialization.
func ListenAndServeReady(ctx context.Context, addr string, handler http.Handler, ready chan<- struct{}) error {
	// H-3 fix: Bind synchronously so port-in-use errors are detected before
	// the goroutine is launched. Previously, ListenAndServe ran in a goroutine
	// and bind errors were only visible after ctx.Done(), making them invisible.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// ME-9: cap per-request time to prevent slow clients from holding goroutines.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Signal readiness now that we're bound to the port.
	if ready != nil {
		close(ready)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return <-errCh
}
