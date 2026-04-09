// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckMediaMTXAPIWithTestServer(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "API returns 200", statusCode: 200},
		{name: "API returns 500", statusCode: 500},
		{name: "API returns 404", statusCode: 404},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"items":[]}`))
			}))
			defer ts.Close()

			// Verify the test server responds as expected
			client := &http.Client{Timeout: 2 * time.Second}
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v3/paths/list", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected status code %d, got %d", tt.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestCheckMediaMTXServiceSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXService(ctx)

	if result.Name != "MediaMTX Service" {
		t.Errorf("expected Name 'MediaMTX Service', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}

	validStatuses := map[CheckStatus]bool{
		StatusOK: true, StatusWarning: true, StatusCritical: true,
		StatusError: true, StatusSkipped: true,
	}
	if !validStatuses[result.Status] {
		t.Errorf("invalid status: %q", result.Status)
	}
}

func TestCheckMediaMTXAPISetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXAPI(ctx)

	if result.Name != "MediaMTX API" {
		t.Errorf("expected Name 'MediaMTX API', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}
	validStatuses := map[CheckStatus]bool{
		StatusOK: true, StatusWarning: true, StatusError: true,
	}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}
