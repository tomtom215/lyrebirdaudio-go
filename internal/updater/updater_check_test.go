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

func TestCheckForUpdates(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		wantUpdate     bool
	}{
		{"older version", "v1.0.0", "v1.1.0", true},
		{"same version", "v1.1.0", "v1.1.0", false},
		{"newer version", "v1.2.0", "v1.1.0", false},
		{"dev version", "dev", "v1.0.0", true},
		{"multi-digit minor: 0.9 to 0.10", "v0.9.0", "v0.10.0", true},
		{"multi-digit minor: 0.10 not newer than 0.10", "v0.10.0", "v0.10.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewerVersion(tt.latestVersion, tt.currentVersion)
			if got != tt.wantUpdate {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v",
					tt.latestVersion, tt.currentVersion, got, tt.wantUpdate)
			}
		})
	}
}

func TestCheckForUpdatesWithMock(t *testing.T) {
	assetName := getAssetName()
	release := Release{
		TagName:     "v2.0.0",
		Name:        "Release 2.0.0",
		PublishedAt: time.Now(),
		Body:        "New features",
		Assets: []Asset{
			{Name: assetName + ".tar.gz", BrowserDownloadURL: "https://example.com/download"},
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

	u := New(WithOwner("test"), WithRepo("repo"), WithCurrentVersion("v1.0.0"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	info, err := u.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdates() error: %v", err)
	}

	if !info.UpdateAvailable {
		t.Error("UpdateAvailable should be true")
	}
	if info.LatestVersion != "v2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "v2.0.0")
	}
	if info.DownloadURL == "" {
		t.Error("DownloadURL should not be empty")
	}
}

func TestCheckForUpdatesNoUpdate(t *testing.T) {
	release := Release{
		TagName:     "v1.0.0",
		PublishedAt: time.Now(),
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

	u := New(WithOwner("test"), WithRepo("repo"), WithCurrentVersion("v1.0.0"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	info, err := u.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdates() error: %v", err)
	}

	if info.UpdateAvailable {
		t.Error("UpdateAvailable should be false (same version)")
	}
}

func TestCheckForUpdatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	u := New(WithOwner("test"), WithRepo("repo"))
	u.httpClient = &http.Client{Transport: &mockTransport{mockServer: server}}

	_, err := u.CheckForUpdates(context.Background())
	if err == nil {
		t.Error("Expected error when release not found")
	}
}
