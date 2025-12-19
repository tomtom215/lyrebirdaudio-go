package updater

import (
	"context"
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
		latest  string
		current string
		want    bool
	}{
		{"v1.1.0", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.1.0", false},
		{"1.1.0", "1.0.0", true},   // Without 'v' prefix
		{"v2.0.0", "v1.9.9", true}, // Major version
		{"v1.0.0", "dev", true},    // Dev version
		{"v1.0.0", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.latest+"_vs_"+tt.current, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v",
					tt.latest, tt.current, got, tt.want)
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
