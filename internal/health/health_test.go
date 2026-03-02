package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockProvider implements StatusProvider for testing.
type mockProvider struct {
	services []ServiceInfo
}

func (m *mockProvider) Services() []ServiceInfo {
	return m.services
}

func TestNewHandler(t *testing.T) {
	h := NewHandler(nil)
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestHealthy(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{
				Name:    "blue_yeti",
				State:   "running",
				Uptime:  5 * time.Minute,
				Healthy: true,
			},
		},
	}

	h := NewHandler(provider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("status = %q, want %q", resp.Status, "healthy")
	}
	if len(resp.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(resp.Services))
	}
	if resp.Services[0].Name != "blue_yeti" {
		t.Errorf("service name = %q, want %q", resp.Services[0].Name, "blue_yeti")
	}
}

func TestUnhealthy(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{
				Name:    "blue_yeti",
				State:   "failed",
				Healthy: false,
				Error:   "FFmpeg exited with code 1",
			},
		},
	}

	h := NewHandler(provider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("status = %q, want %q", resp.Status, "unhealthy")
	}
}

func TestNoServices(t *testing.T) {
	provider := &mockProvider{services: nil}

	h := NewHandler(provider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	// No services = unhealthy (daemon has nothing to do)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("status = %q, want %q", resp.Status, "unhealthy")
	}
}

func TestNilProvider(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestMixedServices(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "device_a", State: "running", Healthy: true, Uptime: time.Hour},
			{Name: "device_b", State: "failed", Healthy: false, Error: "crash"},
		},
	}

	h := NewHandler(provider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	// One unhealthy service means overall unhealthy
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("status = %q, want %q", resp.Status, "unhealthy")
	}
	if len(resp.Services) != 2 {
		t.Errorf("services = %d, want 2", len(resp.Services))
	}
}

func TestResponseContentType(t *testing.T) {
	h := NewHandler(&mockProvider{
		services: []ServiceInfo{{Name: "x", State: "running", Healthy: true}},
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := NewHandler(&mockProvider{})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/healthz", nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s: status = %d, want %d", method, rec.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

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

func TestResponseTimestamp(t *testing.T) {
	h := NewHandler(&mockProvider{
		services: []ServiceInfo{{Name: "x", State: "running", Healthy: true}},
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	before := time.Now()
	h.ServeHTTP(rec, req)
	after := time.Now()

	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Timestamp.Before(before) || resp.Timestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", resp.Timestamp, before, after)
	}
}

func TestHeadRequest(t *testing.T) {
	h := NewHandler(&mockProvider{
		services: []ServiceInfo{{Name: "x", State: "running", Healthy: true}},
	})
	req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	// HEAD should work like GET for health checks
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ==========================================================================
// H-3 fix tests: health endpoint startup probe
// ==========================================================================

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

// --- Tests for SystemInfo / WithSystemInfo (GAP-7, GAP-1d, B-2, B-3) ---

// mockSysInfoProvider implements SystemInfoProvider for testing.
type mockSysInfoProvider struct {
	info SystemInfo
}

func (m *mockSysInfoProvider) SystemInfo() SystemInfo {
	return m.info
}

func TestWithSystemInfoHealthy(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  10 * 1024 * 1024 * 1024,
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: false,
			NTPSynced:      true,
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("status = %q, want healthy", resp.Status)
	}
	if resp.System == nil {
		t.Fatal("system field should be present when sysProvider is set")
	}
	if resp.System.DiskLowWarning {
		t.Error("DiskLowWarning should be false")
	}
	if !resp.System.NTPSynced {
		t.Error("NTPSynced should be true")
	}
}

func TestWithSystemInfoDiskLowWarning(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  100 * 1024 * 1024, // 100 MB — below threshold
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: true,
			NTPSynced:      true,
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when disk is low", rec.Code)
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded", resp.Status)
	}
}

func TestWithSystemInfoNTPDesynced(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  10 * 1024 * 1024 * 1024,
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: false,
			NTPSynced:      false,
			NTPMessage:     "NTP not synchronized",
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// NTP desync downgrades to degraded
	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded when NTP not synced", resp.Status)
	}
	if resp.System.NTPSynced {
		t.Error("NTPSynced should be false")
	}
}

// --- Tests for Prometheus /metrics endpoint (GAP-6 / C-1) ---

func TestMetricsEndpoint(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true, Uptime: 3600 * time.Second, Restarts: 2, Failures: 5},
			{Name: "usb_mic", State: "failed", Healthy: false, Uptime: 0, Restarts: 10, Failures: 20},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  42 * 1024 * 1024 * 1024,
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: false,
			NTPSynced:      true,
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("metrics status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()

	wantContains := []string{
		"lyrebird_stream_healthy",
		`lyrebird_stream_healthy{stream="blue_yeti"} 1`,
		`lyrebird_stream_healthy{stream="usb_mic"} 0`,
		"lyrebird_stream_uptime_seconds",
		"lyrebird_stream_restarts_total",
		`lyrebird_stream_restarts_total{stream="blue_yeti"} 2`,
		`lyrebird_stream_restarts_total{stream="usb_mic"} 10`,
		"lyrebird_stream_failures_total",
		`lyrebird_stream_failures_total{stream="blue_yeti"} 5`,
		"lyrebird_disk_free_bytes",
		"lyrebird_disk_total_bytes",
		"lyrebird_disk_low_warning",
		"lyrebird_ntp_synced",
	}

	for _, want := range wantContains {
		if !containsStr(body, want) {
			t.Errorf("metrics body missing %q", want)
		}
	}
}

func TestMetricsEndpointNoProvider(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMetricsEndpointMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// containsStr is a helper that checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// TestServiceInfoRestarts verifies Restarts field is included in JSON response.
func TestServiceInfoRestarts(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true, Restarts: 3, Failures: 7},
		},
	}
	h := NewHandler(provider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(resp.Services))
	}
	if resp.Services[0].Restarts != 3 {
		t.Errorf("Restarts = %d, want 3", resp.Services[0].Restarts)
	}
}
