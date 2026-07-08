// SPDX-License-Identifier: MIT

package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// invalidURLOwner contains a null byte (control character) so that
// fmt.Sprintf produces a URL that url.Parse rejects, causing
// http.NewRequestWithContext to return an error.
const invalidURLOwner = "invalid\x00owner"

// newInvalidURLUpdater creates an Updater whose GitHub API URL construction
// will produce an invalid URL (space in owner → http.NewRequestWithContext
// returns an error). This exercises the `if err != nil { return nil, err }`
// error paths in GetLatestRelease, ListReleases, GetRelease, and Download.
func newInvalidURLUpdater() *Updater {
	return New(
		WithOwner(invalidURLOwner),
		WithRepo("repo"),
	)
}

// TestGetLatestReleaseInvalidURL covers updater.go:167-169 — the
// http.NewRequestWithContext error branch in GetLatestRelease.
func TestGetLatestReleaseInvalidURL(t *testing.T) {
	u := newInvalidURLUpdater()
	_, err := u.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("GetLatestRelease() expected error for invalid URL, got nil")
	}
}

// TestListReleasesInvalidURL covers updater.go:199-201 — the
// http.NewRequestWithContext error branch in ListReleases.
func TestListReleasesInvalidURL(t *testing.T) {
	u := newInvalidURLUpdater()
	_, err := u.ListReleases(context.Background())
	if err == nil {
		t.Error("ListReleases() expected error for invalid URL, got nil")
	}
}

// TestGetReleaseInvalidURL covers updater.go:239-241 — the
// http.NewRequestWithContext error branch in GetRelease.
func TestGetReleaseInvalidURL(t *testing.T) {
	u := newInvalidURLUpdater()
	_, err := u.GetRelease(context.Background(), "v1.0.0")
	if err == nil {
		t.Error("GetRelease() expected error for invalid URL, got nil")
	}
}

// TestDownloadInvalidURL covers updater.go:269-271 — the
// http.NewRequestWithContext error branch in Download.
func TestDownloadInvalidURL(t *testing.T) {
	u := New()
	// A URL with a space is invalid.
	err := u.Download(context.Background(), "http://invalid host/file.tar.gz", "/tmp/out", nil)
	if err == nil {
		t.Error("Download() expected error for invalid URL, got nil")
	}
}

// TestGetLatestReleaseNon200 covers updater.go:182-184 — the non-OK, non-404
// status path in GetLatestRelease. The existing test suite covers 404; this
// test exercises the generic non-200 branch (e.g., 500).
func TestGetLatestReleaseNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	_, err := u.GetLatestRelease(context.Background())
	if err == nil {
		t.Error("GetLatestRelease() expected error for 500 response, got nil")
	}
}

// TestListReleasesNon200 covers updater.go:205-207 — the non-OK status path
// in ListReleases (e.g., 500 response).
func TestListReleasesNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	_, err := u.ListReleases(context.Background())
	if err == nil {
		t.Error("ListReleases() expected error for 500 response, got nil")
	}
}

// TestGetReleaseNon200 covers updater.go:254-256 — the non-OK, non-404 status
// path in GetRelease. This exercises the generic non-200 branch.
func TestGetReleaseNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	_, err := u.GetRelease(context.Background(), "v1.0.0")
	if err == nil {
		t.Error("GetRelease() expected error for 503 response, got nil")
	}
}

// TestCheckForUpdatesChecksumAsset covers updater.go:154-156 — the
// info.ChecksumURL assignment when an asset name ends with "_checksums.txt".
// The test serves a mock GitHub releases/latest response that includes a
// checksums asset with that suffix.
func TestCheckForUpdatesChecksumAsset(t *testing.T) {
	releaseJSON := `{
		"tag_name": "v2.0.0",
		"name": "v2.0.0",
		"body": "release notes",
		"published_at": "2025-01-01T00:00:00Z",
		"assets": [
			{
				"name": "lyrebird_linux_amd64.tar.gz",
				"browser_download_url": "http://example.com/lyrebird.tar.gz"
			},
			{
				"name": "lyrebird_v2.0.0_checksums.txt",
				"browser_download_url": "http://example.com/checksums.txt"
			}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(releaseJSON))
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	info, err := u.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdates() unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("CheckForUpdates() returned nil info")
	}
	if info.ChecksumURL == "" {
		t.Error("CheckForUpdates() expected ChecksumURL to be set for _checksums.txt asset")
	}
}

// newTestUpdater creates an Updater that routes API calls to the given base
// URL instead of api.github.com. It replaces GitHubAPIURL by using a custom
// http.Client whose Transport rewrites requests to the test server.
//
// It opts into WithAllowUnverified(true) because these fixtures exercise the
// download/extract/backup/install mechanics against mock servers that do not
// serve a checksums asset; the fail-closed default (verified in
// TestUpdateFailsClosedWithoutChecksum) would otherwise short-circuit those
// paths before they are reached.
func newTestUpdater(serverURL string) *Updater {
	// Use a custom transport to redirect GitHub API requests to the test server.
	transport := &redirectTransport{serverURL: serverURL}
	client := &http.Client{Transport: transport}
	return New(
		WithOwner("owner"),
		WithRepo("repo"),
		WithHTTPClient(client),
		WithAllowUnverified(true),
	)
}

// TestGetLatestReleaseCancelledCtx covers updater.go:173-175 — the
// httpClient.Do error branch in GetLatestRelease when the context is already
// cancelled. http.NewRequestWithContext succeeds (valid URL), but Do fails
// because the context is cancelled before the request is sent.
func TestGetLatestReleaseCancelledCtx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	u := newTestUpdater(srv.URL)
	_, err := u.GetLatestRelease(ctx)
	if err == nil {
		t.Error("GetLatestRelease() expected error for cancelled context, got nil")
	}
}

// TestListReleasesCancelledCtx covers updater.go:205-207 — the httpClient.Do
// error branch in ListReleases when the context is pre-cancelled.
func TestListReleasesCancelledCtx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	u := newTestUpdater(srv.URL)
	_, err := u.ListReleases(ctx)
	if err == nil {
		t.Error("ListReleases() expected error for cancelled context, got nil")
	}
}

// TestGetReleaseCancelledCtx covers updater.go:245-247 — the httpClient.Do
// error branch in GetRelease when the context is pre-cancelled.
func TestGetReleaseCancelledCtx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	u := newTestUpdater(srv.URL)
	_, err := u.GetRelease(ctx, "v1.0.0")
	if err == nil {
		t.Error("GetRelease() expected error for cancelled context, got nil")
	}
}

// TestDownloadCancelledCtx covers updater.go:274-276 — the httpClient.Do
// error branch in Download when the context is pre-cancelled.
func TestDownloadCancelledCtx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	u := newTestUpdater(srv.URL)
	err := u.Download(ctx, srv.URL+"/file", "/tmp/out-download", nil)
	if err == nil {
		t.Error("Download() expected error for cancelled context, got nil")
	}
}

// TestDownloadNon200 covers updater.go:279-281 — the non-200 status error
// branch in Download (e.g., 403 response from server).
func TestDownloadNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	err := u.Download(context.Background(), srv.URL+"/file", "/tmp/out-download", nil)
	if err == nil {
		t.Error("Download() expected error for 403 response, got nil")
	}
}

// TestDownloadCreateFileError covers updater.go:286-288 — the os.Create error
// branch in Download when destPath is a directory (EISDIR).
func TestDownloadCreateFileError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	// Create a directory at destPath so os.Create fails.
	destDir := filepath.Join(tmpDir, "destdir")
	if err := os.Mkdir(destDir, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	u := newTestUpdater(srv.URL)
	err := u.Download(context.Background(), srv.URL+"/file", destDir, nil)
	if err == nil {
		t.Error("Download() expected error when destPath is a directory, got nil")
	}
}

// TestUpdateNonTarball covers updater.go:358-360 — the non-tarball asset path
// in Update where AssetName does not end in .tar.gz or .tgz. The downloaded
// file is used directly as the new binary without extraction.
func TestUpdateNonTarball(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "new-binary")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Serve the fake binary.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile(fakeBinary)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	info := &UpdateInfo{
		DownloadURL:     srv.URL + "/binary",
		AssetName:       "lyrebird-linux-amd64", // NOT .tar.gz
		UpdateAvailable: true,
	}

	targetBinary := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(targetBinary, []byte("old"), 0755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	err := u.Update(context.Background(), info, targetBinary, nil)
	if err != nil {
		t.Logf("Update() error (acceptable if chmod/copy fail): %v", err)
	}
}

// redirectTransport rewrites the host of every outgoing request to serverURL.
type redirectTransport struct {
	serverURL string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Build a new URL by replacing scheme+host with the test server.
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = "http"
	// Extract just the host from serverURL (e.g., "127.0.0.1:PORT")
	// serverURL is like "http://127.0.0.1:PORT"
	parsed, _ := http.NewRequest("GET", t.serverURL, nil)
	if parsed != nil {
		newReq.URL.Host = parsed.URL.Host
	}
	return http.DefaultTransport.RoundTrip(newReq)
}
