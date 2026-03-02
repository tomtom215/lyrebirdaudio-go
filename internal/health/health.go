// SPDX-License-Identifier: MIT

// Package health provides an HTTP health check endpoint for the lyrebird-stream daemon.
//
// The health check exposes service status at /healthz as JSON, suitable for
// systemd watchdog, load balancer probes, or monitoring systems.
//
// A Prometheus-compatible /metrics endpoint is also served, providing per-stream
// uptime, restart counts, failure counts, and disk space gauges for fleet
// monitoring via Grafana/Prometheus (GAP-6 / Phase C-1).
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ServiceInfo describes the health state of a single stream service.
type ServiceInfo struct {
	Name     string        `json:"name"`
	State    string        `json:"state"`
	Uptime   time.Duration `json:"uptime_ns"`
	Healthy  bool          `json:"healthy"`
	Error    string        `json:"error,omitempty"`
	Restarts int           `json:"restarts,omitempty"` // total supervisor restarts
	Failures int           `json:"failures,omitempty"` // FFmpeg-level failures from manager
}

// SystemInfo contains system-level health data included in the health response.
// GAP-7: NTP/clock sync status surfaced for bioacoustics timestamp accuracy.
// GAP-1d: Disk space surfaced for proactive ENOSPC warning.
type SystemInfo struct {
	DiskFreeBytes  uint64 `json:"disk_free_bytes"`
	DiskTotalBytes uint64 `json:"disk_total_bytes"`
	DiskLowWarning bool   `json:"disk_low_warning,omitempty"`
	NTPSynced      bool   `json:"ntp_synced"`
	NTPMessage     string `json:"ntp_message,omitempty"`
}

// StatusProvider returns the current health status of all services.
// The daemon implements this interface to supply live data.
type StatusProvider interface {
	Services() []ServiceInfo
}

// SystemInfoProvider returns system-level health data.
// The daemon implements this interface to supply disk space and NTP info.
type SystemInfoProvider interface {
	SystemInfo() SystemInfo
}

// Response is the JSON body returned by the health endpoint.
type Response struct {
	Status    string        `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Services  []ServiceInfo `json:"services"`
	System    *SystemInfo   `json:"system,omitempty"`
}

// Handler serves the /healthz and /metrics endpoints.
type Handler struct {
	provider    StatusProvider
	sysProvider SystemInfoProvider
}

// NewHandler creates a health check HTTP handler.
func NewHandler(provider StatusProvider) *Handler {
	return &Handler{provider: provider}
}

// WithSystemInfo attaches an optional system info provider to the handler.
// When set, disk space and NTP status are included in /healthz responses and
// /metrics output (GAP-7, GAP-1d, GAP-6).
func (h *Handler) WithSystemInfo(p SystemInfoProvider) *Handler {
	h.sysProvider = p
	return h
}

// ServeHTTP implements http.Handler, routing to /healthz and /metrics.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/metrics":
		h.serveMetrics(w, r)
	default:
		h.serveHealth(w, r)
	}
}

func (h *Handler) serveHealth(w http.ResponseWriter, r *http.Request) {
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

	// GAP-7 / GAP-1d: include system info when provider is wired.
	if h.sysProvider != nil {
		si := h.sysProvider.SystemInfo()
		resp.System = &si
		if si.DiskLowWarning {
			resp.Status = "degraded"
			healthy = false
		}
		if !si.NTPSynced {
			// NTP desync is a warning, not a hard failure — keep status as-is
			// but ensure the degraded state is visible in the JSON body.
			if resp.Status == "healthy" {
				resp.Status = "degraded"
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if healthy && resp.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// serveMetrics writes a Prometheus text-format metrics response (GAP-6 / C-1).
// This implements a minimal subset of the exposition format without any
// external dependency — no prometheus/client_golang import required.
func (h *Handler) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var sb strings.Builder

	var services []ServiceInfo
	if h.provider != nil {
		services = h.provider.Services()
	}

	// Per-stream metrics.
	if len(services) > 0 {
		fmt.Fprintln(&sb, "# HELP lyrebird_stream_healthy Is the stream currently healthy (1=healthy, 0=not).")
		fmt.Fprintln(&sb, "# TYPE lyrebird_stream_healthy gauge")
		for _, svc := range services {
			v := 0
			if svc.Healthy {
				v = 1
			}
			fmt.Fprintf(&sb, "lyrebird_stream_healthy{stream=%q} %d\n", svc.Name, v)
		}

		fmt.Fprintln(&sb, "# HELP lyrebird_stream_uptime_seconds Seconds since stream last started.")
		fmt.Fprintln(&sb, "# TYPE lyrebird_stream_uptime_seconds gauge")
		for _, svc := range services {
			secs := svc.Uptime.Seconds()
			fmt.Fprintf(&sb, "lyrebird_stream_uptime_seconds{stream=%q} %.3f\n", svc.Name, secs)
		}

		fmt.Fprintln(&sb, "# HELP lyrebird_stream_restarts_total Total supervisor restarts for stream.")
		fmt.Fprintln(&sb, "# TYPE lyrebird_stream_restarts_total counter")
		for _, svc := range services {
			fmt.Fprintf(&sb, "lyrebird_stream_restarts_total{stream=%q} %d\n", svc.Name, svc.Restarts)
		}

		fmt.Fprintln(&sb, "# HELP lyrebird_stream_failures_total Total FFmpeg-level failures for stream.")
		fmt.Fprintln(&sb, "# TYPE lyrebird_stream_failures_total counter")
		for _, svc := range services {
			fmt.Fprintf(&sb, "lyrebird_stream_failures_total{stream=%q} %d\n", svc.Name, svc.Failures)
		}
	}

	// System metrics.
	if h.sysProvider != nil {
		si := h.sysProvider.SystemInfo()

		fmt.Fprintln(&sb, "# HELP lyrebird_disk_free_bytes Free bytes on the recording filesystem.")
		fmt.Fprintln(&sb, "# TYPE lyrebird_disk_free_bytes gauge")
		fmt.Fprintf(&sb, "lyrebird_disk_free_bytes %d\n", si.DiskFreeBytes)

		fmt.Fprintln(&sb, "# HELP lyrebird_disk_total_bytes Total bytes on the recording filesystem.")
		fmt.Fprintln(&sb, "# TYPE lyrebird_disk_total_bytes gauge")
		fmt.Fprintf(&sb, "lyrebird_disk_total_bytes %d\n", si.DiskTotalBytes)

		diskLow := 0
		if si.DiskLowWarning {
			diskLow = 1
		}
		fmt.Fprintln(&sb, "# HELP lyrebird_disk_low_warning 1 when free disk is below configured threshold.")
		fmt.Fprintln(&sb, "# TYPE lyrebird_disk_low_warning gauge")
		fmt.Fprintf(&sb, "lyrebird_disk_low_warning %d\n", diskLow)

		ntpSynced := 0
		if si.NTPSynced {
			ntpSynced = 1
		}
		fmt.Fprintln(&sb, "# HELP lyrebird_ntp_synced 1 when system clock is NTP-synchronized.")
		fmt.Fprintln(&sb, "# TYPE lyrebird_ntp_synced gauge")
		fmt.Fprintf(&sb, "lyrebird_ntp_synced %d\n", ntpSynced)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
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
