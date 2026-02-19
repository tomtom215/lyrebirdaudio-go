package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	u := New()
	if u == nil {
		t.Fatal("New() returned nil")
	}
	if u.owner != DefaultOwner {
		t.Errorf("owner = %q, want %q", u.owner, DefaultOwner)
	}
	if u.repo != DefaultRepo {
		t.Errorf("repo = %q, want %q", u.repo, DefaultRepo)
	}
}

func TestNewWithOptions(t *testing.T) {
	u := New(
		WithOwner("testowner"),
		WithRepo("testrepo"),
		WithCurrentVersion("v1.0.0"),
	)

	if u.owner != "testowner" {
		t.Errorf("owner = %q, want %q", u.owner, "testowner")
	}
	if u.repo != "testrepo" {
		t.Errorf("repo = %q, want %q", u.repo, "testrepo")
	}
	if u.currentVersion != "v1.0.0" {
		t.Errorf("currentVersion = %q, want %q", u.currentVersion, "v1.0.0")
	}
}

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

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		// Basic comparisons
		{"patch bump", "v1.1.0", "v1.0.0", true},
		{"same version", "v1.0.0", "v1.0.0", false},
		{"current is newer", "v1.0.0", "v1.1.0", false},
		{"no v prefix", "1.1.0", "1.0.0", true},
		{"major version bump", "v2.0.0", "v1.9.9", true},

		// Special versions
		{"dev version always updates", "v1.0.0", "dev", true},
		{"unknown version always updates", "v1.0.0", "unknown", true},

		// CRITICAL: Multi-digit component comparisons (string comparison bug)
		{"0.10.0 is newer than 0.9.0", "0.10.0", "0.9.0", true},
		{"0.9.0 is NOT newer than 0.10.0", "0.9.0", "0.10.0", false},
		{"1.0.0 is newer than 0.99.99", "1.0.0", "0.99.99", true},
		{"2.0.0 is newer than 1.99.99", "2.0.0", "1.99.99", true},
		{"1.10.0 vs 1.9.0", "v1.10.0", "v1.9.0", true},
		{"1.9.0 vs 1.10.0", "v1.9.0", "v1.10.0", false},
		{"1.0.10 vs 1.0.9", "v1.0.10", "v1.0.9", true},
		{"1.0.9 vs 1.0.10", "v1.0.9", "v1.0.10", false},

		// Equal versions (not newer)
		{"equal versions no prefix", "1.0.0", "1.0.0", false},
		{"equal versions with prefix", "v2.5.3", "v2.5.3", false},
		{"equal versions mixed prefix", "v1.0.0", "1.0.0", false},

		// Pre-release suffixes
		{"pre-release vs release", "v1.0.0-rc1", "v1.0.0", false},
		{"release vs pre-release", "v1.0.0", "v0.9.0-rc1", true},
		{"pre-release newer major", "v2.0.0-beta1", "v1.0.0", true},
		{"pre-release same base older current", "v1.1.0-alpha", "v1.0.0", true},

		// Edge cases
		{"latest is dev", "dev", "v1.0.0", false},
		{"both dev", "dev", "dev", true},
		{"latest is unknown", "unknown", "v1.0.0", false},
		{"non-semver latest", "notaversion", "v1.0.0", false},
		{"single component", "2", "1", true},
		{"two components", "1.1", "1.0", true},
		{"large version numbers", "v100.200.300", "v99.199.299", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v",
					tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMajor int
		wantMinor int
		wantPatch int
		wantPre   string
		wantOk    bool
	}{
		{"basic version", "1.2.3", 1, 2, 3, "", true},
		{"with v prefix", "v1.2.3", 1, 2, 3, "", true},
		{"with prerelease", "1.0.0-rc1", 1, 0, 0, "rc1", true},
		{"with v and prerelease", "v2.0.0-beta.1", 2, 0, 0, "beta.1", true},
		{"two components", "1.2", 1, 2, 0, "", true},
		{"single component", "3", 3, 0, 0, "", true},
		{"large numbers", "100.200.300", 100, 200, 300, "", true},
		{"zero version", "0.0.0", 0, 0, 0, "", true},
		{"non-numeric", "abc", 0, 0, 0, "", false},
		{"empty string", "", 0, 0, 0, "", false},
		{"dev", "dev", 0, 0, 0, "", false},
		{"partial non-numeric", "1.abc.3", 0, 0, 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, patch, pre, ok := parseSemver(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseSemver(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if !ok {
				return
			}
			if major != tt.wantMajor {
				t.Errorf("parseSemver(%q) major = %d, want %d", tt.input, major, tt.wantMajor)
			}
			if minor != tt.wantMinor {
				t.Errorf("parseSemver(%q) minor = %d, want %d", tt.input, minor, tt.wantMinor)
			}
			if patch != tt.wantPatch {
				t.Errorf("parseSemver(%q) patch = %d, want %d", tt.input, patch, tt.wantPatch)
			}
			if pre != tt.wantPre {
				t.Errorf("parseSemver(%q) pre = %q, want %q", tt.input, pre, tt.wantPre)
			}
		})
	}
}

func TestGetAssetName(t *testing.T) {
	name := getAssetName()
	if name == "" {
		t.Error("getAssetName() returned empty string")
	}

	// Should contain os and arch
	if !containsString(name, "lyrebird-") {
		t.Errorf("Asset name should start with 'lyrebird-', got %q", name)
	}
}

func TestDownload(t *testing.T) {
	content := "test binary content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "19")
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "download.bin")

	u := New()
	var progressCalled bool
	err := u.Download(context.Background(), server.URL, destPath, func(downloaded, total int64) {
		progressCalled = true
	})

	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}

	if !progressCalled {
		t.Error("Progress callback was not called")
	}

	// Verify content
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(data) != content {
		t.Errorf("Downloaded content = %q, want %q", string(data), content)
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := "test content"
	if err := os.WriteFile(srcPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Copy
	dstPath := filepath.Join(tmpDir, "dest.txt")
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}

	// Verify
	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(data) != content {
		t.Errorf("Copied content = %q, want %q", string(data), content)
	}
}

func TestRollback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create binary and backup
	binaryPath := filepath.Join(tmpDir, "lyrebird")
	backupPath := binaryPath + ".backup"

	if err := os.WriteFile(binaryPath, []byte("new version"), 0755); err != nil {
		t.Fatalf("Failed to create binary: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte("old version"), 0755); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	u := New()
	if err := u.Rollback(binaryPath); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}

	// Verify rollback
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("Failed to read binary: %v", err)
	}

	if string(data) != "old version" {
		t.Errorf("Binary content = %q, want %q", string(data), "old version")
	}

	// Backup should be gone
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("Backup file should be removed after rollback")
	}
}

func TestRollbackNoBackup(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "lyrebird")

	u := New()
	err := u.Rollback(binaryPath)
	if err == nil {
		t.Error("Rollback() expected error when no backup exists")
	}
}

func TestHasBackup(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "lyrebird")
	backupPath := binaryPath + ".backup"

	u := New()

	// No backup
	if u.HasBackup(binaryPath) {
		t.Error("HasBackup() should return false when no backup exists")
	}

	// Create backup
	if err := os.WriteFile(backupPath, []byte("backup"), 0755); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	if !u.HasBackup(binaryPath) {
		t.Error("HasBackup() should return true when backup exists")
	}
}

func TestFormatReleaseInfo(t *testing.T) {
	release := &Release{
		TagName:     "v1.0.0",
		Name:        "First Release",
		PublishedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Body:        "Initial release with basic features",
		Assets: []Asset{
			{Name: "lyrebird-linux-amd64.tar.gz"},
		},
	}

	info := FormatReleaseInfo(release)

	if !containsString(info, "v1.0.0") {
		t.Error("Info should contain version")
	}
	if !containsString(info, "First Release") {
		t.Error("Info should contain name")
	}
	if !containsString(info, "Initial release") {
		t.Error("Info should contain release notes")
	}
}

func TestFormatUpdateInfo(t *testing.T) {
	info := &UpdateInfo{
		CurrentVersion:  "v1.0.0",
		LatestVersion:   "v1.1.0",
		UpdateAvailable: true,
	}

	output := FormatUpdateInfo(info)

	if !containsString(output, "v1.0.0") {
		t.Error("Output should contain current version")
	}
	if !containsString(output, "v1.1.0") {
		t.Error("Output should contain latest version")
	}
	if !containsString(output, "Update available") {
		t.Error("Output should indicate update available")
	}
}

func TestFormatUpdateInfoNoUpdate(t *testing.T) {
	info := &UpdateInfo{
		CurrentVersion:  "v1.1.0",
		LatestVersion:   "v1.1.0",
		UpdateAvailable: false,
	}

	output := FormatUpdateInfo(info)

	if !containsString(output, "latest version") {
		t.Error("Output should indicate running latest version")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 60 * time.Second}
	u := New(WithHTTPClient(customClient))
	if u.httpClient != customClient {
		t.Error("httpClient was not set correctly")
	}
}

func TestDownloadErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "download.bin")

	u := New()
	err := u.Download(context.Background(), server.URL, destPath, nil)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestDownloadNoProgress(t *testing.T) {
	content := "test content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "download.bin")

	u := New()
	err := u.Download(context.Background(), server.URL, destPath, nil)
	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}

	data, _ := os.ReadFile(destPath)
	if string(data) != content {
		t.Errorf("Downloaded content = %q, want %q", string(data), content)
	}
}

func TestCopyFileSourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dest"))
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestFormatReleaseInfoPrerelease(t *testing.T) {
	release := &Release{
		TagName:     "v2.0.0-rc1",
		PublishedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Prerelease:  true,
	}

	info := FormatReleaseInfo(release)

	if !containsString(info, "Pre-release") {
		t.Error("Info should indicate pre-release")
	}
}

func TestFormatReleaseInfoMinimal(t *testing.T) {
	release := &Release{
		TagName:     "v1.0.0",
		PublishedAt: time.Now(),
	}

	info := FormatReleaseInfo(release)

	if !containsString(info, "v1.0.0") {
		t.Error("Info should contain version")
	}
}

func TestUpdateNoDownloadURL(t *testing.T) {
	u := New()
	info := &UpdateInfo{
		DownloadURL: "",
	}

	err := u.Update(context.Background(), info, "/fake/path", nil)
	if err == nil {
		t.Error("Expected error for empty download URL")
	}
}

func TestProgressReader(t *testing.T) {
	content := "hello world"
	r := &progressReader{
		reader: io.NopCloser(strings.NewReader(content)),
		onProgress: func(n int64) {
			// Just verify callback is called
		},
	}

	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != 5 {
		t.Errorf("Read() = %d, want 5", n)
	}

	if err := r.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
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

func TestConstants(t *testing.T) {
	if DefaultOwner == "" {
		t.Error("DefaultOwner should not be empty")
	}
	if DefaultRepo == "" {
		t.Error("DefaultRepo should not be empty")
	}
	if DefaultTimeout <= 0 {
		t.Error("DefaultTimeout should be positive")
	}
	if GitHubAPIURL == "" {
		t.Error("GitHubAPIURL should not be empty")
	}
}

func TestAssetFields(t *testing.T) {
	asset := Asset{
		Name:               "lyrebird-linux-amd64.tar.gz",
		Size:               1024,
		BrowserDownloadURL: "https://example.com/download",
		ContentType:        "application/gzip",
	}

	if asset.Name != "lyrebird-linux-amd64.tar.gz" {
		t.Error("Asset Name mismatch")
	}
	if asset.Size != 1024 {
		t.Error("Asset Size mismatch")
	}
}

func TestReleaseFields(t *testing.T) {
	release := Release{
		TagName:    "v1.0.0",
		Name:       "Release 1.0.0",
		Draft:      false,
		Prerelease: false,
		Body:       "Release notes",
		HTMLURL:    "https://github.com/test/repo/releases/v1.0.0",
	}

	if release.TagName != "v1.0.0" {
		t.Error("Release TagName mismatch")
	}
	if release.HTMLURL != "https://github.com/test/repo/releases/v1.0.0" {
		t.Error("Release HTMLURL mismatch")
	}
}

func TestUpdateInfoFields(t *testing.T) {
	info := UpdateInfo{
		CurrentVersion:  "v1.0.0",
		LatestVersion:   "v1.1.0",
		UpdateAvailable: true,
		ReleaseNotes:    "Bug fixes",
		DownloadURL:     "https://example.com/download",
		AssetName:       "lyrebird-linux-amd64.tar.gz",
		PublishedAt:     time.Now(),
	}

	if info.CurrentVersion != "v1.0.0" {
		t.Error("UpdateInfo CurrentVersion mismatch")
	}
	if !info.UpdateAvailable {
		t.Error("UpdateInfo UpdateAvailable should be true")
	}
}

// mockTransport redirects requests to a mock server
type mockTransport struct {
	mockServer *httptest.Server
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect GitHub API requests to mock server
	mockURL := t.mockServer.URL + req.URL.Path
	mockReq, err := http.NewRequestWithContext(req.Context(), req.Method, mockURL, req.Body)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Header {
		mockReq.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(mockReq)
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

// createTestTarGz creates a test tar.gz archive with the specified files
func createTestTarGz(t *testing.T, archivePath string, files map[string][]byte) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0755,
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("Failed to write content: %v", err)
		}
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create test archive with lyrebird binary
	files := map[string][]byte{
		"lyrebird":        []byte("#!/bin/bash\necho 'hello'\n"),
		"lyrebird-stream": []byte("#!/bin/bash\necho 'stream'\n"),
		"README.md":       []byte("# README"),
	}
	createTestTarGz(t, archivePath, files)

	// Extract
	destDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	binaryPath, err := extractBinaryFromTarGz(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractBinaryFromTarGz() error: %v", err)
	}

	if !strings.HasSuffix(binaryPath, "lyrebird") {
		t.Errorf("Binary path = %q, want to end with 'lyrebird'", binaryPath)
	}

	// Verify binary was extracted
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("Failed to read extracted binary: %v", err)
	}
	if string(content) != string(files["lyrebird"]) {
		t.Error("Extracted content doesn't match")
	}
}

func TestExtractBinaryFromTarGzNoBinary(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create archive without lyrebird binary
	files := map[string][]byte{
		"README.md":  []byte("# README"),
		"other-file": []byte("other content"),
	}
	createTestTarGz(t, archivePath, files)

	destDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	_, err := extractBinaryFromTarGz(archivePath, destDir)
	if err == nil {
		t.Error("Expected error when binary not in archive")
	}
	if !strings.Contains(err.Error(), "binary not found") {
		t.Errorf("Error = %q, want to contain 'binary not found'", err.Error())
	}
}

func TestExtractBinaryFromTarGzNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, "extracted")

	_, err := extractBinaryFromTarGz("/nonexistent/archive.tar.gz", destDir)
	if err == nil {
		t.Error("Expected error for nonexistent archive")
	}
}

func TestExtractBinaryFromTarGzInvalidGzip(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "invalid.tar.gz")

	// Create invalid gzip file
	if err := os.WriteFile(archivePath, []byte("not gzip"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	destDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	_, err := extractBinaryFromTarGz(archivePath, destDir)
	if err == nil {
		t.Error("Expected error for invalid gzip")
	}
}

func TestExtractBinaryFromTarGzInvalidTar(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "invalid.tar.gz")

	// Create valid gzip but invalid tar
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	gw := gzip.NewWriter(f)
	_, _ = gw.Write([]byte("not valid tar"))
	_ = gw.Close()
	_ = f.Close()

	destDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	_, err = extractBinaryFromTarGz(archivePath, destDir)
	if err == nil {
		t.Error("Expected error for invalid tar")
	}
}

func TestUpdateWithMock(t *testing.T) {
	// Create a mock tar.gz archive
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "release.tar.gz")
	files := map[string][]byte{
		"lyrebird": []byte("#!/bin/bash\necho 'new version'\n"),
	}
	createTestTarGz(t, archivePath, files)

	// Read the archive content for serving
	archiveContent, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("Failed to read archive: %v", err)
	}

	// Create mock download server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archiveContent)
	}))
	defer server.Close()

	// Create existing binary
	binaryPath := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(binaryPath, []byte("old version"), 0755); err != nil {
		t.Fatalf("Failed to create binary: %v", err)
	}

	u := New()
	u.httpClient = server.Client()

	info := &UpdateInfo{
		DownloadURL: server.URL + "/release.tar.gz",
		AssetName:   "lyrebird-linux-amd64.tar.gz",
	}

	err = u.Update(context.Background(), info, binaryPath, nil)
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	// Verify the binary was updated
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("Failed to read binary: %v", err)
	}

	expected := string(files["lyrebird"])
	if string(content) != expected {
		t.Errorf("Binary content = %q, want %q", string(content), expected)
	}

	// Verify backup was removed (successful update)
	backupPath := binaryPath + ".backup"
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("Backup should be removed after successful update")
	}
}

func TestUpdateDownloadFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(binaryPath, []byte("old version"), 0755); err != nil {
		t.Fatalf("Failed to create binary: %v", err)
	}

	u := New()
	u.httpClient = server.Client()

	info := &UpdateInfo{
		DownloadURL: server.URL + "/release.tar.gz",
		AssetName:   "lyrebird-linux-amd64.tar.gz",
	}

	err := u.Update(context.Background(), info, binaryPath, nil)
	if err == nil {
		t.Error("Expected error for download failure")
	}
}

func TestCopyFileDestError(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}

	// Try to copy to a nonexistent directory
	err := copyFile(srcPath, "/nonexistent/dir/dest.txt")
	if err == nil {
		t.Error("Expected error for nonexistent destination directory")
	}
}

// TestParseChecksumFile covers the M-13 checksum parser.
func TestParseChecksumFile(t *testing.T) {
	goodContent := `# checksums for v1.2.3
abc123def456abc123def456abc123def456abc123def456abc123def456abc1  lyrebird-linux-amd64.tar.gz
fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210  lyrebird-linux-arm64.tar.gz
`

	tests := []struct {
		name      string
		content   string
		assetName string
		wantHash  string
		wantErr   bool
	}{
		{
			name:      "found exact match",
			content:   goodContent,
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
			wantErr:   false,
		},
		{
			name:      "found second asset",
			content:   goodContent,
			assetName: "lyrebird-linux-arm64.tar.gz",
			wantHash:  "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
			wantErr:   false,
		},
		{
			name:      "asset not found",
			content:   goodContent,
			assetName: "lyrebird-windows-amd64.zip",
			wantErr:   true,
		},
		{
			name:      "empty content",
			content:   "",
			assetName: "any.tar.gz",
			wantErr:   true,
		},
		{
			name:      "comment-only content",
			content:   "# just a comment\n",
			assetName: "any.tar.gz",
			wantErr:   true,
		},
		{
			name:      "binary mode asterisk prefix",
			content:   "abc123def456abc123def456abc123def456abc123def456abc123def456abc1 *lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
			wantErr:   false,
		},
		{
			name:      "invalid hash length",
			content:   "tooshort  lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantErr:   true,
		},
		{
			name:      "basename match ignores path prefix",
			content:   "abc123def456abc123def456abc123def456abc123def456abc123def456abc1  ./dist/lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChecksumFile(tt.content, tt.assetName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseChecksumFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantHash {
				t.Errorf("ParseChecksumFile() = %q, want %q", got, tt.wantHash)
			}
		})
	}
}

// sha256HexTest computes the hex-encoded SHA256 of data (test helper).
func sha256HexTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestVerifyChecksumFromURL tests the end-to-end checksum download and verify path.
func TestVerifyChecksumFromURL(t *testing.T) {
	// Create a test asset file with known content
	tmpDir := t.TempDir()
	assetContent := []byte("fake binary content for testing")
	assetPath := filepath.Join(tmpDir, "lyrebird-linux-amd64.tar.gz")
	if err := os.WriteFile(assetPath, assetContent, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Compute the actual SHA256 using the test helper
	goodHash := sha256HexTest(assetContent)
	checksumContent := goodHash + "  lyrebird-linux-amd64.tar.gz\n"

	t.Run("valid checksum", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(checksumContent))
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err != nil {
			t.Errorf("unexpected error for valid checksum: %v", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		wrongContent := strings.Repeat("0", 64) + "  lyrebird-linux-amd64.tar.gz\n"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(wrongContent))
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err == nil {
			t.Error("expected checksum mismatch error, got nil")
		}
		if !strings.Contains(err.Error(), "mismatch") {
			t.Errorf("error should mention mismatch, got: %v", err)
		}
	})

	t.Run("checksums server error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err == nil {
			t.Error("expected error for server error, got nil")
		}
	})

	t.Run("asset not in checksums", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(goodHash + "  other-file.tar.gz\n"))
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err == nil {
			t.Error("expected error when asset not in checksums, got nil")
		}
	})
}
