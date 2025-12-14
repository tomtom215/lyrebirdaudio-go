package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:9997")
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.baseURL != "http://localhost:9997" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://localhost:9997")
	}
}

func TestNewClientWithOptions(t *testing.T) {
	client := NewClient("http://localhost:9997",
		WithTimeout(10*time.Second),
	)
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want %v", client.httpClient.Timeout, 10*time.Second)
	}
}

func TestListPaths(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/paths/list" {
			http.NotFound(w, r)
			return
		}

		resp := PathList{
			PageCount: 1,
			ItemCount: 2,
			Items: []Path{
				{Name: "stream1", Ready: true, BytesReceived: 1000},
				{Name: "stream2", Ready: false, BytesReceived: 0},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	paths, err := client.ListPaths(context.Background())
	if err != nil {
		t.Fatalf("ListPaths() error: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("ListPaths() returned %d paths, want 2", len(paths))
	}

	if paths[0].Name != "stream1" {
		t.Errorf("paths[0].Name = %q, want %q", paths[0].Name, "stream1")
	}
}

func TestListPathsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.ListPaths(context.Background())
	if err == nil {
		t.Error("ListPaths() expected error for 500 response")
	}
}

func TestGetPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/paths/get/test_stream" {
			http.NotFound(w, r)
			return
		}

		resp := Path{
			Name:          "test_stream",
			Ready:         true,
			BytesReceived: 5000,
			Tracks: []Track{
				{Type: "audio", Codec: "opus", SampleRate: 48000},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	path, err := client.GetPath(context.Background(), "test_stream")
	if err != nil {
		t.Fatalf("GetPath() error: %v", err)
	}

	if path.Name != "test_stream" {
		t.Errorf("path.Name = %q, want %q", path.Name, "test_stream")
	}
	if !path.Ready {
		t.Error("path.Ready should be true")
	}
	if len(path.Tracks) != 1 {
		t.Errorf("len(path.Tracks) = %d, want 1", len(path.Tracks))
	}
}

func TestGetPathNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetPath(context.Background(), "nonexistent")
	if err == nil {
		t.Error("GetPath() expected error for 404 response")
	}
}

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

func TestPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PathList{})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Ping(context.Background())
	if err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

func TestPingError(t *testing.T) {
	client := NewClient("http://localhost:1") // Invalid port
	err := client.Ping(context.Background())
	if err == nil {
		t.Error("Ping() expected error for unreachable server")
	}
}

func TestGetStreamStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Path{
			Name:          "test_stream",
			Ready:         true,
			ReadyTime:     "2025-12-14T10:00:00Z",
			BytesReceived: 10000,
			BytesSent:     5000,
			Tracks: []Track{
				{Type: "audio", Codec: "opus", SampleRate: 48000, Channels: 2},
			},
			Readers: []Reader{
				{Type: "rtsp", ID: "reader1"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	stats, err := client.GetStreamStats(context.Background(), "test_stream")
	if err != nil {
		t.Fatalf("GetStreamStats() error: %v", err)
	}

	if stats.Name != "test_stream" {
		t.Errorf("stats.Name = %q, want %q", stats.Name, "test_stream")
	}
	if stats.ReaderCount != 1 {
		t.Errorf("stats.ReaderCount = %d, want 1", stats.ReaderCount)
	}
	if stats.AudioCodec != "opus" {
		t.Errorf("stats.AudioCodec = %q, want %q", stats.AudioCodec, "opus")
	}
	if stats.SampleRate != 48000 {
		t.Errorf("stats.SampleRate = %d, want 48000", stats.SampleRate)
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

func TestWaitForStreamTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return not ready
		resp := Path{Name: "test", Ready: false, BytesReceived: 0}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.WaitForStream(context.Background(), "test", 100*time.Millisecond)
	if err == nil {
		t.Error("WaitForStream() expected timeout error")
	}
}

func TestWaitForStreamSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp Path
		if callCount >= 2 {
			resp = Path{Name: "test", Ready: true, BytesReceived: 100}
		} else {
			resp = Path{Name: "test", Ready: false, BytesReceived: 0}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.WaitForStream(context.Background(), "test", 5*time.Second)
	if err != nil {
		t.Errorf("WaitForStream() error: %v", err)
	}
}
