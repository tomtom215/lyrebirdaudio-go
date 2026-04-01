// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// invalidBaseURL contains a space which causes http.NewRequestWithContext to fail.
const invalidBaseURL = "http://invalid host"

// TestListPathsInvalidURL covers the http.NewRequestWithContext error path in
// ListPaths when the base URL is malformed.
func TestListPathsInvalidURL(t *testing.T) {
	client := NewClient(invalidBaseURL)
	_, err := client.ListPaths(context.Background())
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

// TestGetPathInvalidURL covers the http.NewRequestWithContext error path in
// GetPath when the base URL is malformed.
func TestGetPathInvalidURL(t *testing.T) {
	client := NewClient(invalidBaseURL)
	_, err := client.GetPath(context.Background(), "mypath")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

// TestPingInvalidURL covers the http.NewRequestWithContext error path in
// Ping when the base URL is malformed.
func TestPingInvalidURL(t *testing.T) {
	client := NewClient(invalidBaseURL)
	err := client.Ping(context.Background())
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

// TestHealthCheckListPathsError covers the ListPaths error path in HealthCheck
// that is hit when Ping succeeds but ListPaths fails. We set up a server that
// returns 200 for Ping but a different status for ListPaths after the first call.
func TestHealthCheckListPathsError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call (Ping): return 200
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"pageCount":0,"items":[]}`))
		} else {
			// Second call (ListPaths in HealthCheck): return 500
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal"}`))
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	status, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck returned unexpected error: %v", err)
	}
	// HealthCheck should return non-nil status with an error message (not return an error itself).
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Error == "" {
		t.Error("expected status.Error to be set when ListPaths fails")
	}
}

// TestListPathsNon200WithBody covers the non-200 status path in ListPaths
// by returning a 500 response with a body (ensuring the ReadAll branch is taken,
// not the hard-to-trigger ReadAll error branch).
func TestListPathsNon200WithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"service unavailable"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.ListPaths(context.Background())
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

// TestGetPathNon200NotFoundOrOK covers the non-200/non-404 path in GetPath.
func TestGetPathNon200NotFoundOrOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.GetPath(context.Background(), "mypath")
	if err == nil {
		t.Error("expected error for 503 response, got nil")
	}
}

// TestPingNon200WithBody covers the non-200 status path in Ping.
func TestPingNon200WithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.Ping(context.Background())
	if err == nil {
		t.Error("expected error for 403 response, got nil")
	}
}
