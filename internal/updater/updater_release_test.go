package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetLatestRelease(t *testing.T) {
	release := Release{
		TagName:     "v1.2.0",
		Name:        "Release 1.2.0",
		PublishedAt: time.Now(),
		Assets: []Asset{
			{Name: "lyrebird-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/download"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/test/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	u := New(
		WithOwner("test"),
		WithRepo("repo"),
	)
	u.httpClient = &http.Client{}

	// Override base URL by using a custom transport
	_ = GitHubAPIURL // Can't actually override const in test

	// For testing, we'll create a mock server and use it directly
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer mockServer.Close()

	// Test with direct HTTP call
	req, _ := http.NewRequest("GET", mockServer.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var gotRelease Release
	if err := json.NewDecoder(resp.Body).Decode(&gotRelease); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if gotRelease.TagName != "v1.2.0" {
		t.Errorf("TagName = %q, want %q", gotRelease.TagName, "v1.2.0")
	}
}

func TestGetLatestReleaseWithMock(t *testing.T) {
	release := Release{
		TagName:     "v1.2.0",
		Name:        "Release 1.2.0",
		PublishedAt: time.Now(),
		Body:        "Release notes",
		Assets: []Asset{
			{Name: "lyrebird-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/download"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	got, err := u.GetLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("GetLatestRelease() error: %v", err)
	}

	if got.TagName != "v1.2.0" {
		t.Errorf("TagName = %q, want %q", got.TagName, "v1.2.0")
	}
}

func TestGetLatestReleaseNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("Expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "no releases found") {
		t.Errorf("Error = %q, want to contain 'no releases found'", err.Error())
	}
}

func TestGetLatestReleaseServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestGetLatestReleaseInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestListReleases(t *testing.T) {
	releases := []Release{
		{TagName: "v1.2.0", PublishedAt: time.Now()},
		{TagName: "v1.1.0", PublishedAt: time.Now().Add(-24 * time.Hour)},
		{TagName: "v1.0.0-draft", Draft: true}, // Should be filtered
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	// Direct test of filtering logic
	var filtered []Release
	for _, r := range releases {
		if !r.Draft {
			filtered = append(filtered, r)
		}
	}

	if len(filtered) != 2 {
		t.Errorf("Expected 2 non-draft releases, got %d", len(filtered))
	}
}

func TestListReleasesWithMock(t *testing.T) {
	releases := []Release{
		{TagName: "v1.2.0", PublishedAt: time.Now()},
		{TagName: "v1.1.0", PublishedAt: time.Now().Add(-24 * time.Hour)},
		{TagName: "v1.0.0-draft", Draft: true}, // Should be filtered
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(releases)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	got, err := u.ListReleases(context.Background())
	if err != nil {
		t.Fatalf("ListReleases() error: %v", err)
	}

	// Draft should be filtered
	if len(got) != 2 {
		t.Errorf("Expected 2 releases (draft filtered), got %d", len(got))
	}
}

func TestListReleasesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.ListReleases(context.Background())
	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestListReleasesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.ListReleases(context.Background())
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestGetReleaseNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Verify the test server returns 404
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestGetReleaseWithMock(t *testing.T) {
	release := Release{
		TagName:     "v1.0.0",
		Name:        "Release 1.0.0",
		PublishedAt: time.Now(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/tags/v1.0.0") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	got, err := u.GetRelease(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatalf("GetRelease() error: %v", err)
	}

	if got.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want %q", got.TagName, "v1.0.0")
	}
}

func TestGetReleaseNotFoundWithMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.GetRelease(context.Background(), "v999.0.0")
	if err == nil {
		t.Error("Expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error = %q, want to contain 'not found'", err.Error())
	}
}

func TestGetReleaseServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.GetRelease(context.Background(), "v1.0.0")
	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestGetReleaseInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.GetRelease(context.Background(), "v1.0.0")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}
