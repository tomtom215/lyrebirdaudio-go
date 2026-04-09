package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestPingServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Ping(context.Background())
	if err == nil {
		t.Error("Ping() expected error for 503 response")
	}
}
