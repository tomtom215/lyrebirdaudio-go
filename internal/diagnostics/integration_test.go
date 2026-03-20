// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCheckMediaMTXAPIWithLocalServer starts an HTTP server on port 9997
// to exercise the success path of checkMediaMTXAPI.
func TestCheckMediaMTXAPIWithLocalServer(t *testing.T) {
	// Try to bind to the MediaMTX API port
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", DefaultAPIPort))
	if err != nil {
		t.Skipf("cannot bind to port %d (may be in use): %v", DefaultAPIPort, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXAPI(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK with local server, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "reachable") {
		t.Errorf("expected 'reachable' in message, got %q", result.Message)
	}
}

// TestCheckMediaMTXAPIWithNon200 starts an HTTP server on port 9997 that returns 500
// to exercise the non-200 status code path.
func TestCheckMediaMTXAPIWithNon200(t *testing.T) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", DefaultAPIPort))
	if err != nil {
		t.Skipf("cannot bind to port %d (may be in use): %v", DefaultAPIPort, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXAPI(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning with non-200 response, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "returned status") {
		t.Errorf("expected 'returned status' in message, got %q", result.Message)
	}
}

// TestCheckNetworkPortsBothOpen starts listeners on both RTSP and API ports
// to exercise the "both open" path.
func TestCheckNetworkPortsBothOpen(t *testing.T) {
	rtspListener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", DefaultRTSPPort))
	if err != nil {
		t.Skipf("cannot bind to RTSP port %d: %v", DefaultRTSPPort, err)
	}
	defer func() { _ = rtspListener.Close() }()

	apiListener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", DefaultAPIPort))
	if err != nil {
		t.Skipf("cannot bind to API port %d: %v", DefaultAPIPort, err)
	}
	defer func() { _ = apiListener.Close() }()

	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkNetworkPorts(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK with both ports open, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "RTSP") || !strings.Contains(result.Message, "API") {
		t.Errorf("expected RTSP and API in message, got %q", result.Message)
	}
}

// TestCheckNetworkPortsOneOpen starts a listener on only the RTSP port
// to exercise the "partial" path.
func TestCheckNetworkPortsOneOpen(t *testing.T) {
	rtspListener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", DefaultRTSPPort))
	if err != nil {
		t.Skipf("cannot bind to RTSP port %d: %v", DefaultRTSPPort, err)
	}
	defer func() { _ = rtspListener.Close() }()

	// Ensure API port is NOT open
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", DefaultAPIPort), 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Skip("API port is already open, cannot test partial scenario")
	}

	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkNetworkPorts(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning with one port open, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "Some ports not accessible") {
		t.Errorf("expected 'Some ports not accessible' in message, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "API") {
		t.Errorf("expected API port in partial message, got %q", result.Message)
	}
}
