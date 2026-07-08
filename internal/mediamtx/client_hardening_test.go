// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetPathEscapesName verifies a path name containing URL-significant
// characters (space, '#') is percent-encoded on the wire and round-trips to the
// server unchanged. Without escaping, http.NewRequest either rejects the space
// or the '#' silently becomes a fragment — corrupting the request.
func TestGetPathEscapesName(t *testing.T) {
	const name = "Blue Yeti #2"

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path // net/http has already unescaped this
		_ = json.NewEncoder(w).Encode(Path{Name: name, Ready: true})
	}))
	defer server.Close()

	got, err := NewClient(server.URL).GetPath(context.Background(), name)
	if err != nil {
		t.Fatalf("GetPath(%q) error: %v", name, err)
	}
	if want := "/v3/paths/get/" + name; gotPath != want {
		t.Errorf("server received path %q, want %q", gotPath, want)
	}
	if got.Name != name {
		t.Errorf("GetPath returned name %q, want %q", got.Name, name)
	}
}

// TestGetPathBoundsErrorBody verifies that an oversized error response body is
// truncated by maxErrorBodyBytes rather than buffered whole into the error.
func TestGetPathBoundsErrorBody(t *testing.T) {
	huge := strings.Repeat("x", 1<<20) // 1 MiB error body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(huge))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).GetPath(context.Background(), "mic")
	if err == nil {
		t.Fatal("expected error for status 500")
	}
	// The message carries a status prefix plus at most maxErrorBodyBytes of body.
	if len(err.Error()) > maxErrorBodyBytes+128 {
		t.Errorf("error body not bounded: len=%d, want <= %d", len(err.Error()), maxErrorBodyBytes+128)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500: %v", err)
	}
}
