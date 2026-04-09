package mediamtx

import (
	"net/http"
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

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	client := NewClient("http://localhost:9997", WithHTTPClient(customClient))
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.httpClient != customClient {
		t.Error("WithHTTPClient did not set the custom client")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want %v", client.httpClient.Timeout, 30*time.Second)
	}
}
