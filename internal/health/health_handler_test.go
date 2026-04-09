package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
