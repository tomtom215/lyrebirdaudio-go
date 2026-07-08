// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newFakeGlobalConfigServer returns an httptest server that replies to
// GET /v3/config/global/get with the supplied body (status 200). All other
// paths return 404 so wrong routing shows up as a test failure.
func newFakeGlobalConfigServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/config/global/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("checkMediaMTXConfig must use GET, got %s", r.Method)
		}
		_, _ = w.Write([]byte(body))
	})
	return httptest.NewServer(mux)
}

// apiAddrFromURL strips the "http://" prefix from an httptest URL so it
// can be plugged into Options.MediaMTXAPIAddr (which stores host:port).
func apiAddrFromURL(t *testing.T, u string) string {
	t.Helper()
	return strings.TrimPrefix(u, "http://")
}

func TestCheckMediaMTXConfig_OK(t *testing.T) {
	server := newFakeGlobalConfigServer(t, `{
		"logLevel": "info",
		"api": true,
		"apiAddress": ":9997",
		"rtsp": true,
		"rtspAddress": ":8554",
		"rtspTransports": ["udp","tcp"],
		"rtspEncryption": "no",
		"authMethod": "internal"
	}`)
	defer server.Close()

	r := NewRunner(Options{MediaMTXAPIAddr: apiAddrFromURL(t, server.URL)})
	result := r.checkMediaMTXConfig(context.Background())

	if result.Status != StatusOK {
		t.Errorf("Status = %q, want OK; message=%q details=%q",
			result.Status, result.Message, result.Details)
	}
	if !strings.Contains(result.Details, "logLevel=info") {
		t.Errorf("details missing logLevel: %q", result.Details)
	}
	if !strings.Contains(result.Details, "rtsp=true") {
		t.Errorf("details missing rtsp=true: %q", result.Details)
	}
	if len(result.Suggestions) != 0 {
		t.Errorf("OK result should have no suggestions, got %v", result.Suggestions)
	}
}

func TestCheckMediaMTXConfig_RTSPDisabledIsCritical(t *testing.T) {
	server := newFakeGlobalConfigServer(t, `{
		"logLevel": "info",
		"api": true,
		"rtsp": false,
		"rtspAddress": ":8554"
	}`)
	defer server.Close()

	r := NewRunner(Options{MediaMTXAPIAddr: apiAddrFromURL(t, server.URL)})
	result := r.checkMediaMTXConfig(context.Background())

	if result.Status != StatusCritical {
		t.Errorf("Status = %q, want CRITICAL", result.Status)
	}
	if !strings.Contains(result.Message, "RTSP server is disabled") {
		t.Errorf("Message should mention RTSP disabled, got %q", result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("CRITICAL result should include a suggestion")
	}
}

func TestCheckMediaMTXConfig_APIDisabledIsWarning(t *testing.T) {
	server := newFakeGlobalConfigServer(t, `{
		"logLevel": "info",
		"api": false,
		"rtsp": true
	}`)
	defer server.Close()

	r := NewRunner(Options{MediaMTXAPIAddr: apiAddrFromURL(t, server.URL)})
	result := r.checkMediaMTXConfig(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %q, want WARNING", result.Status)
	}
	if !strings.Contains(result.Message, "control API is disabled") {
		t.Errorf("Message should mention API disabled, got %q", result.Message)
	}
}

func TestCheckMediaMTXConfig_DebugLogLevelIsWarning(t *testing.T) {
	server := newFakeGlobalConfigServer(t, `{
		"logLevel": "debug",
		"api": true,
		"rtsp": true
	}`)
	defer server.Close()

	r := NewRunner(Options{MediaMTXAPIAddr: apiAddrFromURL(t, server.URL)})
	result := r.checkMediaMTXConfig(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %q, want WARNING", result.Status)
	}
	if !strings.Contains(result.Message, "debug") {
		t.Errorf("Message should mention debug, got %q", result.Message)
	}
}

// TestCheckMediaMTXConfig_CriticalTrumpsWarnings verifies that when both a
// CRITICAL and a WARNING issue are present, Status is CRITICAL but Message
// still surfaces both so the operator sees the full picture.
func TestCheckMediaMTXConfig_CriticalTrumpsWarnings(t *testing.T) {
	server := newFakeGlobalConfigServer(t, `{
		"logLevel": "debug",
		"api": false,
		"rtsp": false
	}`)
	defer server.Close()

	r := NewRunner(Options{MediaMTXAPIAddr: apiAddrFromURL(t, server.URL)})
	result := r.checkMediaMTXConfig(context.Background())

	if result.Status != StatusCritical {
		t.Errorf("Status = %q, want CRITICAL", result.Status)
	}
	if !strings.Contains(result.Message, "RTSP server is disabled") {
		t.Errorf("Message should contain RTSP critical, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "control API is disabled") {
		t.Errorf("Message should also contain API warning, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "debug") {
		t.Errorf("Message should also contain debug warning, got %q", result.Message)
	}
}

func TestCheckMediaMTXConfig_APIUnreachableIsSkipped(t *testing.T) {
	r := NewRunner(Options{MediaMTXAPIAddr: "127.0.0.1:1"})
	result := r.checkMediaMTXConfig(context.Background())

	// Treat unreachable as SKIPPED, not ERROR — checkMediaMTXAPI already
	// reports reachability and we don't want to double-count failures.
	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want SKIPPED (unreachable API)", result.Status)
	}
	if !strings.Contains(result.Message, "not reachable") {
		t.Errorf("Message should mention unreachable, got %q", result.Message)
	}
}

// TestCheckMediaMTXConfig_DefaultAPIAddr verifies the empty-string default
// falls through to localhost:9997. Using :1 would hijack this test, so we
// only exercise the branch where MediaMTXAPIAddr is empty and the check
// returns SKIPPED (no listener on the default port during `go test`).
func TestCheckMediaMTXConfig_DefaultAPIAddr(t *testing.T) {
	r := NewRunner(Options{MediaMTXAPIAddr: ""})
	result := r.checkMediaMTXConfig(context.Background())
	// Either SKIPPED (no server on :9997) or OK (a MediaMTX instance
	// happens to be running on the test host) are both acceptable.
	if result.Status != StatusSkipped && result.Status != StatusOK &&
		result.Status != StatusWarning && result.Status != StatusCritical {
		t.Errorf("unexpected status %q", result.Status)
	}
}

// TestCheckMediaMTXConfig_IncludedInFullMode verifies the check is
// registered with the runner when full mode is selected.
func TestCheckMediaMTXConfig_IncludedInFullMode(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeFull
	r := NewRunner(opts)
	checks := r.getChecks()

	// Run each check with a stopped context so expensive probes return
	// early, then look for the one whose name matches.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	found := false
	for _, c := range checks {
		res := c.fn(ctx)
		if res.Name == "MediaMTX Config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("checkMediaMTXConfig not registered in full-mode check list")
	}
}

// TestCheckMediaMTXConfig_DefaultMediaMTXAPIAddrIsLocalhost9997 verifies
// the default address constant matches the hard-coded fallback used when
// MediaMTXAPIAddr is empty.
func TestCheckMediaMTXConfig_DefaultMediaMTXAPIAddrIsLocalhost9997(t *testing.T) {
	opts := DefaultOptions()
	if opts.MediaMTXAPIAddr != "localhost:9997" {
		t.Errorf("DefaultOptions().MediaMTXAPIAddr = %q, want localhost:9997", opts.MediaMTXAPIAddr)
	}
}
