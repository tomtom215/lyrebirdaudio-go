// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// withStubbedSessionFetcher swaps fetchActiveSessionsFn for the duration of
// a test and restores it afterward via t.Cleanup.
func withStubbedSessionFetcher(t *testing.T, stub func(apiURL string) []SessionInfo) {
	t.Helper()
	orig := fetchActiveSessionsFn
	fetchActiveSessionsFn = stub
	t.Cleanup(func() { fetchActiveSessionsFn = orig })
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns the
// captured output. Test helpers only — not suitable for concurrent use.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := fn()
	_ = w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	return string(out), err
}

// TestRunStatusJSONIncludesActiveSessions verifies that ActiveSessions is
// present in JSON output and populated from the injected fetcher.
func TestRunStatusJSONIncludesActiveSessions(t *testing.T) {
	withStubbedSessionFetcher(t, func(string) []SessionInfo {
		return []SessionInfo{
			{
				ID:            "11111111-2222-3333-4444-555555555555",
				RemoteAddr:    "10.0.0.5:54321",
				State:         "read",
				Path:          "blue_yeti",
				Transport:     "UDP",
				OutboundBytes: 4096,
			},
			{
				ID:           "22222222-3333-4444-5555-666666666666",
				RemoteAddr:   "127.0.0.1:33333",
				State:        "publish",
				Path:         "blue_yeti",
				Transport:    "TCP",
				InboundBytes: 1_048_576,
			},
		}
	})

	lockDir := t.TempDir()
	out, err := captureStdout(t, func() error {
		return runStatus([]string{"--lock-dir=" + lockDir, "--json"})
	})
	if err != nil {
		t.Fatalf("runStatus() error: %v", err)
	}

	var parsed StatusOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json.Unmarshal(%q) error: %v", out, err)
	}
	if parsed.ActiveSessions == nil {
		t.Fatal("ActiveSessions should never be nil in JSON output")
	}
	if len(parsed.ActiveSessions) != 2 {
		t.Fatalf("len(ActiveSessions) = %d, want 2", len(parsed.ActiveSessions))
	}
	if parsed.ActiveSessions[0].Path != "blue_yeti" || parsed.ActiveSessions[0].State != "read" {
		t.Errorf("session[0] = %+v", parsed.ActiveSessions[0])
	}
	if parsed.ActiveSessions[0].OutboundBytes != 4096 {
		t.Errorf("session[0].OutboundBytes = %d, want 4096", parsed.ActiveSessions[0].OutboundBytes)
	}
}

// TestRunStatusJSONActiveSessionsEmptyOnAPIFailure verifies fail-soft
// behaviour: when the fetcher yields an empty slice (its failure mode),
// ActiveSessions stays non-nil and no error escapes runStatus.
func TestRunStatusJSONActiveSessionsEmptyOnAPIFailure(t *testing.T) {
	withStubbedSessionFetcher(t, func(string) []SessionInfo {
		return []SessionInfo{}
	})

	lockDir := t.TempDir()
	out, err := captureStdout(t, func() error {
		return runStatus([]string{"--lock-dir=" + lockDir, "--json"})
	})
	if err != nil {
		t.Fatalf("runStatus() error: %v", err)
	}

	var parsed StatusOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json.Unmarshal error: %v; out=%s", err, out)
	}
	if parsed.ActiveSessions == nil {
		t.Error("ActiveSessions must be non-nil even on API failure")
	}
	if len(parsed.ActiveSessions) != 0 {
		t.Errorf("ActiveSessions = %v, want empty", parsed.ActiveSessions)
	}
}

// TestRunStatusTextShowsActiveReaders verifies the human-readable output
// renders an "Active Readers" section and filters out publishers.
func TestRunStatusTextShowsActiveReaders(t *testing.T) {
	withStubbedSessionFetcher(t, func(string) []SessionInfo {
		return []SessionInfo{
			{
				RemoteAddr: "192.168.1.42:55555", State: "read", Path: "mic1",
				Transport: "UDP", OutboundBytes: 1024,
			},
			{
				RemoteAddr: "127.0.0.1:60000", State: "publish", Path: "mic1",
				Transport: "TCP",
			},
		}
	})

	lockDir := t.TempDir()
	out, err := captureStdout(t, func() error {
		return runStatus([]string{"--lock-dir=" + lockDir})
	})
	if err != nil {
		t.Fatalf("runStatus() error: %v", err)
	}

	if !strings.Contains(out, "Active Readers:") {
		t.Errorf("text output missing 'Active Readers:' section:\n%s", out)
	}
	if !strings.Contains(out, "192.168.1.42:55555") {
		t.Errorf("text output missing reader IP:\n%s", out)
	}
	// Publisher (publish state) must be filtered out of the text display.
	if strings.Contains(out, "127.0.0.1:60000") {
		t.Errorf("publisher session should be filtered out of text output:\n%s", out)
	}
}

// TestRunStatusTextEmptySessionsMessage verifies the fallback message is
// shown when no readers are present.
func TestRunStatusTextEmptySessionsMessage(t *testing.T) {
	withStubbedSessionFetcher(t, func(string) []SessionInfo {
		return []SessionInfo{}
	})

	lockDir := t.TempDir()
	out, err := captureStdout(t, func() error {
		return runStatus([]string{"--lock-dir=" + lockDir})
	})
	if err != nil {
		t.Fatalf("runStatus() error: %v", err)
	}
	if !strings.Contains(out, "(no active RTSP readers") {
		t.Errorf("expected fallback message, got:\n%s", out)
	}
}

// TestDefaultFetchActiveSessionsHappyPath exercises the non-stubbed path
// against a fake HTTP server to ensure the mediamtx client is wired up.
func TestDefaultFetchActiveSessionsHappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v3/rtspsessions/list") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
			"pageCount": 1,
			"itemCount": 1,
			"items": [
				{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","remoteAddr":"10.1.1.1:1234","state":"read","path":"mic","transport":"UDP","outboundBytes":9000}
			]
		}`))
	}))
	defer server.Close()

	got := defaultFetchActiveSessions(server.URL)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Path != "mic" || got[0].State != "read" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[0].OutboundBytes != 9000 {
		t.Errorf("OutboundBytes = %d, want 9000", got[0].OutboundBytes)
	}
}

// TestDefaultFetchActiveSessionsFailSoft verifies the default fetcher
// returns an empty (non-nil) slice on every failure mode we care about.
func TestDefaultFetchActiveSessionsFailSoft(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty URL", ""},
		{"unreachable port", "http://127.0.0.1:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultFetchActiveSessions(tt.url)
			if got == nil {
				t.Fatal("default fetcher must never return nil")
			}
			if len(got) != 0 {
				t.Errorf("len(got) = %d, want 0", len(got))
			}
		})
	}
}

// TestDefaultFetchActiveSessionsServerError verifies that a 500 response is
// treated as a fail-soft empty result.
func TestDefaultFetchActiveSessionsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	got := defaultFetchActiveSessions(server.URL)
	if got == nil || len(got) != 0 {
		t.Errorf("want non-nil empty slice on 500, got %v", got)
	}
}

// TestFormatBytes covers the human-readable byte formatter used by the
// status text output.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1 << 20, "1.0 MiB"},
		{3 * (1 << 20), "3.0 MiB"},
		{1 << 30, "1.0 GiB"},
		{2*(1<<30) + (1 << 29), "2.5 GiB"},
	}
	for _, tt := range tests {
		if got := formatBytes(tt.in); got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
