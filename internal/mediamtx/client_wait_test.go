package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

func TestWaitForStreamContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Path{Name: "test", Ready: false, BytesReceived: 0}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := client.WaitForStream(ctx, "test", 10*time.Second)
	if err == nil {
		t.Error("WaitForStream() expected error when context cancelled")
	}
}
