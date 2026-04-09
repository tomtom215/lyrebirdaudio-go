// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// validUUID is a fixed, well-formed UUID reused across tests.
const validUUID = "11111111-2222-3333-4444-555555555555"

// --- isValidSessionID ------------------------------------------------------

func TestIsValidSessionID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{validUUID, true},
		{"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", true},
		{"", false},
		{"not-a-uuid", false},
		{"11111111-2222-3333-4444-55555555555", false},   // too short
		{"11111111-2222-3333-4444-5555555555555", false}, // too long
		{"11111111-2222-3333-4444-55555555555g", false},  // non-hex
		// Path-injection attempts.
		{"../../etc/passwd", false},
		{validUUID + "/../admin", false},
		{validUUID + "?action=kick", false},
		{validUUID + "\nInjected-Header: 1", false},
		{validUUID + " ", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := isValidSessionID(tt.id); got != tt.want {
				t.Errorf("isValidSessionID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// --- ListRTSPSessions ------------------------------------------------------

func TestListRTSPSessions_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/rtspsessions/list" {
			http.NotFound(w, r)
			return
		}
		// Convenience wrapper must NOT send query params (uses server defaults).
		if r.URL.RawQuery != "" {
			t.Errorf("ListRTSPSessions sent unexpected query %q", r.URL.RawQuery)
		}
		resp := RTSPSessionList{
			PageCount: 1,
			ItemCount: 2,
			Items: []RTSPSession{
				{
					ID: validUUID, State: "read", Path: "mic",
					RemoteAddr: "10.0.0.5:54321", InboundBytes: 0, OutboundBytes: 123,
				},
				{ID: "22222222-3333-4444-5555-666666666666", State: "publish", Path: "mic"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	sessions, err := client.ListRTSPSessions(context.Background())
	if err != nil {
		t.Fatalf("ListRTSPSessions() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].ID != validUUID {
		t.Errorf("sessions[0].ID = %q, want %q", sessions[0].ID, validUUID)
	}
	if sessions[0].State != "read" {
		t.Errorf("sessions[0].State = %q, want read", sessions[0].State)
	}
	if sessions[0].OutboundBytes != 123 {
		t.Errorf("sessions[0].OutboundBytes = %d, want 123", sessions[0].OutboundBytes)
	}
}

func TestListRTSPSessions_EmptyItemsCoercedToNonNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server returns items: null (not []).
		_, _ = w.Write([]byte(`{"pageCount":0,"itemCount":0,"items":null}`))
	}))
	defer server.Close()

	sessions, err := NewClient(server.URL).ListRTSPSessions(context.Background())
	if err != nil {
		t.Fatalf("ListRTSPSessions() error: %v", err)
	}
	if sessions == nil {
		t.Fatal("sessions should not be nil when err == nil")
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestListRTSPSessionsPage_SendsQueryParams(t *testing.T) {
	var gotPage, gotItems string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPage = r.URL.Query().Get("page")
		gotItems = r.URL.Query().Get("itemsPerPage")
		_, _ = w.Write([]byte(`{"pageCount":3,"itemCount":150,"items":[]}`))
	}))
	defer server.Close()

	list, err := NewClient(server.URL).ListRTSPSessionsPage(context.Background(), 2, 50)
	if err != nil {
		t.Fatalf("ListRTSPSessionsPage() error: %v", err)
	}
	if gotPage != "2" {
		t.Errorf("page query = %q, want 2", gotPage)
	}
	if gotItems != "50" {
		t.Errorf("itemsPerPage query = %q, want 50", gotItems)
	}
	if list.PageCount != 3 || list.ItemCount != 150 {
		t.Errorf("list totals = %d/%d, want 3/150", list.PageCount, list.ItemCount)
	}
}

func TestListRTSPSessionsPage_NegativeMeansOmit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"pageCount":1,"itemCount":0,"items":[]}`))
	}))
	defer server.Close()

	if _, err := NewClient(server.URL).ListRTSPSessionsPage(context.Background(), -1, -1); err != nil {
		t.Fatalf("ListRTSPSessionsPage() error: %v", err)
	}
}

func TestListRTSPSessions_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error","error":"internal"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).ListRTSPSessions(context.Background())
	if err == nil {
		t.Fatal("ListRTSPSessions() expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500: %v", err)
	}
}

func TestListRTSPSessions_ServerErrorUnreadableBody(t *testing.T) {
	// Handler sets a large Content-Length but sends no body, so io.ReadAll
	// returns an error. Exercises the "failed to read body" branch.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusBadGateway)
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}
	}))
	defer server.Close()

	_, err := NewClient(server.URL).ListRTSPSessions(context.Background())
	if err == nil {
		t.Fatal("expected error for truncated 502 response")
	}
}

func TestListRTSPSessions_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).ListRTSPSessions(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestListRTSPSessions_NetworkError(t *testing.T) {
	_, err := NewClient("http://127.0.0.1:1").ListRTSPSessions(context.Background())
	if err == nil {
		t.Error("expected error for network failure")
	}
}

func TestListRTSPSessions_ContextCanceled(t *testing.T) {
	// Handler blocks until the request context is canceled.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewClient(server.URL).ListRTSPSessions(ctx)
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

// --- KickRTSPSession -------------------------------------------------------

func TestKickRTSPSession_Success(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotBodyLen int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBodyLen = len(b)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	if err := NewClient(server.URL).KickRTSPSession(context.Background(), validUUID); err != nil {
		t.Fatalf("KickRTSPSession() error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v3/rtspsessions/kick/"+validUUID {
		t.Errorf("path = %q, want /v3/rtspsessions/kick/%s", gotPath, validUUID)
	}
	if gotBodyLen != 0 {
		t.Errorf("body length = %d, want 0 (kick takes no body)", gotBodyLen)
	}
}

func TestKickRTSPSession_NotFoundReturnsSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":"error","error":"session not found"}`))
	}))
	defer server.Close()

	err := NewClient(server.URL).KickRTSPSession(context.Background(), validUUID)
	if err == nil {
		t.Fatal("KickRTSPSession() expected error for 404")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("err should wrap ErrSessionNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), validUUID) {
		t.Errorf("error should include id for context: %v", err)
	}
}

func TestKickRTSPSession_InvalidIDBlocksRequest(t *testing.T) {
	// If the validator failed open, this handler would be called. We assert
	// it isn't: invalid IDs must be rejected client-side with the sentinel.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not receive a request for an invalid id: %s", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	badIDs := []string{
		"",
		"not-a-uuid",
		validUUID + "/../../etc/passwd",
		validUUID + "?x=1",
		validUUID + "\ninjected: 1",
	}
	for _, id := range badIDs {
		t.Run(id, func(t *testing.T) {
			err := client.KickRTSPSession(context.Background(), id)
			if err == nil {
				t.Fatalf("KickRTSPSession(%q) expected error", id)
			}
			if !errors.Is(err, ErrInvalidSessionID) {
				t.Errorf("err should wrap ErrInvalidSessionID: %v", err)
			}
		})
	}
}

func TestKickRTSPSession_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error","error":"boom"}`))
	}))
	defer server.Close()

	err := NewClient(server.URL).KickRTSPSession(context.Background(), validUUID)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if errors.Is(err, ErrSessionNotFound) {
		t.Error("500 must not be reported as ErrSessionNotFound")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should include server body for context: %v", err)
	}
}

func TestKickRTSPSession_BadRequestBubblesUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"error","error":"bad id"}`))
	}))
	defer server.Close()

	err := NewClient(server.URL).KickRTSPSession(context.Background(), validUUID)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if errors.Is(err, ErrSessionNotFound) {
		t.Error("400 must not be reported as ErrSessionNotFound")
	}
}

func TestKickRTSPSession_ServerErrorUnreadableBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusBadGateway)
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}
	}))
	defer server.Close()

	err := NewClient(server.URL).KickRTSPSession(context.Background(), validUUID)
	if err == nil {
		t.Fatal("expected error for truncated 502 response")
	}
}

func TestKickRTSPSession_NetworkError(t *testing.T) {
	err := NewClient("http://127.0.0.1:1").KickRTSPSession(context.Background(), validUUID)
	if err == nil {
		t.Error("expected error for network failure")
	}
}

func TestKickRTSPSession_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := NewClient(server.URL).KickRTSPSession(ctx, validUUID)
	if err == nil {
		t.Error("expected error for ctx timeout")
	}
}
