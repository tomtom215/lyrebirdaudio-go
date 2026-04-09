// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/mediamtx"
)

// kickFakeServer wires up a fake MediaMTX API that answers
// /v3/rtspsessions/list with the supplied body and records every
// /v3/rtspsessions/kick/{id} call in the returned map.
type kickFakeServer struct {
	*httptest.Server
	mu      sync.Mutex
	kicks   []string
	kickErr map[string]int // id -> HTTP status to return
}

func newKickFakeServer(listBody string) *kickFakeServer {
	k := &kickFakeServer{kickErr: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/rtspsessions/list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, listBody)
	})
	mux.HandleFunc("/v3/rtspsessions/kick/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v3/rtspsessions/kick/")
		k.mu.Lock()
		defer k.mu.Unlock()
		if status, ok := k.kickErr[id]; ok {
			w.WriteHeader(status)
			return
		}
		k.kicks = append(k.kicks, id)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	k.Server = httptest.NewServer(mux)
	return k
}

func (k *kickFakeServer) kicksSnapshot() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]string, len(k.kicks))
	copy(out, k.kicks)
	return out
}

func (k *kickFakeServer) setKickStatus(id string, status int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.kickErr[id] = status
}

// newTestLogger returns a slog.Logger writing to a buffer the caller can
// inspect. The handler runs at debug level so we can assert on debug messages.
func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// TestKickStalledPathReaders_KicksMatchingReaders verifies that only sessions
// whose path matches and whose state is "read" are kicked. The publisher
// (state=="publish") must never be kicked — the supervisor owns its lifecycle.
func TestKickStalledPathReaders_KicksMatchingReaders(t *testing.T) {
	const readerA = "11111111-aaaa-bbbb-cccc-dddddddddddd"
	const readerB = "22222222-aaaa-bbbb-cccc-dddddddddddd"
	const wrongPath = "33333333-aaaa-bbbb-cccc-dddddddddddd"
	const publisher = "44444444-aaaa-bbbb-cccc-dddddddddddd"

	body := `{
		"pageCount": 1,
		"itemCount": 4,
		"items": [
			{"id":"` + readerA + `","state":"read","path":"mic1","remoteAddr":"10.0.0.1:1"},
			{"id":"` + readerB + `","state":"read","path":"mic1","remoteAddr":"10.0.0.2:2"},
			{"id":"` + wrongPath + `","state":"read","path":"mic2","remoteAddr":"10.0.0.3:3"},
			{"id":"` + publisher + `","state":"publish","path":"mic1","remoteAddr":"127.0.0.1:10"}
		]
	}`
	srv := newKickFakeServer(body)
	defer srv.Close()

	logger, _ := newTestLogger()
	client := mediamtx.NewClient(srv.URL)

	kickStalledPathReaders(context.Background(), logger, client, "mic1")

	got := srv.kicksSnapshot()
	if len(got) != 2 {
		t.Fatalf("kicked %d sessions, want 2: %v", len(got), got)
	}
	hasA, hasB := false, false
	for _, id := range got {
		switch id {
		case readerA:
			hasA = true
		case readerB:
			hasB = true
		case wrongPath:
			t.Errorf("wrong-path reader was kicked: %s", id)
		case publisher:
			t.Errorf("publisher was kicked: %s", id)
		}
	}
	if !hasA || !hasB {
		t.Errorf("expected both matching readers kicked: hasA=%v hasB=%v", hasA, hasB)
	}
}

// TestKickStalledPathReaders_SessionNotFoundIsSilentlySwallowed verifies
// that a 404 returned by kick (the "raced, already gone" case) does not
// count as a failure.
func TestKickStalledPathReaders_SessionNotFoundIsSilentlySwallowed(t *testing.T) {
	const raceID = "11111111-aaaa-bbbb-cccc-dddddddddddd"
	body := `{"items":[{"id":"` + raceID + `","state":"read","path":"mic1"}]}`
	srv := newKickFakeServer(body)
	defer srv.Close()
	srv.setKickStatus(raceID, http.StatusNotFound)

	logger, buf := newTestLogger()
	client := mediamtx.NewClient(srv.URL)

	kickStalledPathReaders(context.Background(), logger, client, "mic1")

	// The kick handler returns 404, so kicksSnapshot stays empty.
	if len(srv.kicksSnapshot()) != 0 {
		t.Errorf("no session should be recorded as successfully kicked")
	}
	logs := buf.String()
	if strings.Contains(logs, "session cleanup complete") {
		// A completely silent race has no trailing summary line because
		// both counters stay zero. That's the expected log output.
		t.Errorf("no summary line expected when everything 404s, got: %s", logs)
	}
	if strings.Contains(logs, "kick session failed") {
		t.Errorf("404 must not be logged as an error, got: %s", logs)
	}
}

// TestKickStalledPathReaders_ServerErrorLoggedAndCounted verifies that
// non-404 errors bump the failed counter and are logged at debug level,
// but iteration continues for other sessions.
func TestKickStalledPathReaders_ServerErrorLoggedAndCounted(t *testing.T) {
	const good = "11111111-aaaa-bbbb-cccc-dddddddddddd"
	const bad = "22222222-aaaa-bbbb-cccc-dddddddddddd"
	body := `{"items":[
		{"id":"` + good + `","state":"read","path":"mic1","remoteAddr":"10.0.0.1:1"},
		{"id":"` + bad + `","state":"read","path":"mic1","remoteAddr":"10.0.0.2:2"}
	]}`
	srv := newKickFakeServer(body)
	defer srv.Close()
	srv.setKickStatus(bad, http.StatusInternalServerError)

	logger, buf := newTestLogger()
	client := mediamtx.NewClient(srv.URL)

	kickStalledPathReaders(context.Background(), logger, client, "mic1")

	got := srv.kicksSnapshot()
	if len(got) != 1 || got[0] != good {
		t.Errorf("expected only %s kicked, got %v", good, got)
	}

	logs := buf.String()
	if !strings.Contains(logs, "kick session failed") {
		t.Errorf("expected 'kick session failed' debug log, got: %s", logs)
	}
	if !strings.Contains(logs, "kicked=1") {
		t.Errorf("expected summary line with kicked=1, got: %s", logs)
	}
	if !strings.Contains(logs, "failed=1") {
		t.Errorf("expected summary line with failed=1, got: %s", logs)
	}
}

// TestKickStalledPathReaders_ListError verifies that a list-sessions
// failure short-circuits cleanly — the helper logs and returns without
// attempting any kicks.
func TestKickStalledPathReaders_ListError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/rtspsessions/list", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	var kickCalled atomic.Bool
	mux.HandleFunc("/v3/rtspsessions/kick/", func(w http.ResponseWriter, r *http.Request) {
		kickCalled.Store(true)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	logger, buf := newTestLogger()
	client := mediamtx.NewClient(server.URL)

	kickStalledPathReaders(context.Background(), logger, client, "mic1")

	if kickCalled.Load() {
		t.Error("kick endpoint must not be called when list fails")
	}
	if !strings.Contains(buf.String(), "list sessions failed") {
		t.Errorf("expected 'list sessions failed' debug log, got: %s", buf.String())
	}
}

// TestKickStalledPathReaders_EmptySessions verifies a clean no-op when
// MediaMTX reports no active sessions at all.
func TestKickStalledPathReaders_EmptySessions(t *testing.T) {
	srv := newKickFakeServer(`{"items":[]}`)
	defer srv.Close()

	logger, buf := newTestLogger()
	client := mediamtx.NewClient(srv.URL)
	kickStalledPathReaders(context.Background(), logger, client, "mic1")

	if len(srv.kicksSnapshot()) != 0 {
		t.Error("no kicks should be issued for an empty session list")
	}
	if strings.Contains(buf.String(), "session cleanup complete") {
		t.Error("summary line should not appear when nothing was kicked")
	}
}

// TestKickStalledPathReaders_RespectsTimeout verifies that a wedged
// MediaMTX API cannot stall recovery: the helper uses its own short
// timeout context, so a hanging handler returns promptly.
func TestKickStalledPathReaders_RespectsTimeout(t *testing.T) {
	// This test relies on kickRecoveryTimeout being short (3s). We use
	// a handler that blocks until its request context is canceled and
	// assert the helper returns in < 5s regardless.
	started := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/rtspsessions/list", func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	logger, _ := newTestLogger()
	client := mediamtx.NewClient(server.URL)

	done := make(chan struct{})
	go func() {
		kickStalledPathReaders(context.Background(), logger, client, "mic1")
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never received request")
	}

	select {
	case <-done:
	case <-time.After(kickRecoveryTimeout + 2*time.Second):
		t.Fatalf("kickStalledPathReaders did not return within %v", kickRecoveryTimeout+2*time.Second)
	}
}
