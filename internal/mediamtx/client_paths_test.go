package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestListPathsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return invalid JSON
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.ListPaths(context.Background())
	if err == nil {
		t.Error("ListPaths() expected error for invalid JSON")
	}
}

func TestGetPathDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return invalid JSON
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetPath(context.Background(), "test")
	if err == nil {
		t.Error("GetPath() expected error for invalid JSON")
	}
}

func TestGetPathServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetPath(context.Background(), "test")
	if err == nil {
		t.Error("GetPath() expected error for 500 response")
	}
}

func TestListPathsNetworkError(t *testing.T) {
	client := NewClient("http://localhost:1") // Invalid port
	_, err := client.ListPaths(context.Background())
	if err == nil {
		t.Error("ListPaths() expected error for network failure")
	}
}

func TestGetPathNetworkError(t *testing.T) {
	client := NewClient("http://localhost:1") // Invalid port
	_, err := client.GetPath(context.Background(), "test")
	if err == nil {
		t.Error("GetPath() expected error for network failure")
	}
}
