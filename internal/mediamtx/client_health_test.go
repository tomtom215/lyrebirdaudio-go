package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsStreamHealthy(t *testing.T) {
	tests := []struct {
		name        string
		path        Path
		wantHealthy bool
	}{
		{
			name:        "ready with data",
			path:        Path{Name: "test", Ready: true, BytesReceived: 1000},
			wantHealthy: true,
		},
		{
			name:        "ready no data",
			path:        Path{Name: "test", Ready: true, BytesReceived: 0},
			wantHealthy: false,
		},
		{
			name:        "not ready with data",
			path:        Path{Name: "test", Ready: false, BytesReceived: 1000},
			wantHealthy: false,
		},
		{
			name:        "not ready no data",
			path:        Path{Name: "test", Ready: false, BytesReceived: 0},
			wantHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.path)
			}))
			defer server.Close()

			client := NewClient(server.URL)
			healthy, err := client.IsStreamHealthy(context.Background(), "test")
			if err != nil {
				t.Fatalf("IsStreamHealthy() error: %v", err)
			}
			if healthy != tt.wantHealthy {
				t.Errorf("IsStreamHealthy() = %v, want %v", healthy, tt.wantHealthy)
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PathList{
			Items: []Path{
				{Name: "stream1", Ready: true, BytesReceived: 1000},
				{Name: "stream2", Ready: true, BytesReceived: 500},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error: %v", err)
	}

	if !status.APIReachable {
		t.Error("status.APIReachable should be true")
	}
	if status.TotalStreams != 2 {
		t.Errorf("status.TotalStreams = %d, want 2", status.TotalStreams)
	}
	if status.HealthyStreams != 2 {
		t.Errorf("status.HealthyStreams = %d, want 2", status.HealthyStreams)
	}
	if !status.Healthy {
		t.Error("status.Healthy should be true")
	}
}

func TestHealthCheckUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PathList{
			Items: []Path{
				{Name: "stream1", Ready: true, BytesReceived: 1000},
				{Name: "stream2", Ready: false, BytesReceived: 0}, // Unhealthy
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error: %v", err)
	}

	if status.TotalStreams != 2 {
		t.Errorf("status.TotalStreams = %d, want 2", status.TotalStreams)
	}
	if status.HealthyStreams != 1 {
		t.Errorf("status.HealthyStreams = %d, want 1", status.HealthyStreams)
	}
	if status.Healthy {
		t.Error("status.Healthy should be false")
	}
}

func TestIsStreamHealthyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	healthy, err := client.IsStreamHealthy(context.Background(), "test")
	if err == nil {
		t.Error("IsStreamHealthy() expected error for 404 response")
	}
	if healthy {
		t.Error("healthy should be false when error occurs")
	}
}

func TestHealthCheckError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() should not return error: %v", err)
	}
	if status.APIReachable {
		t.Error("status.APIReachable should be false when API errors")
	}
	if status.Healthy {
		t.Error("status.Healthy should be false when API errors")
	}
}

func TestHealthCheckNoStreams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PathList{
			Items: []Path{},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error: %v", err)
	}

	if !status.APIReachable {
		t.Error("status.APIReachable should be true")
	}
	if status.TotalStreams != 0 {
		t.Errorf("status.TotalStreams = %d, want 0", status.TotalStreams)
	}
	// With no streams, Healthy depends on implementation
}
