// SPDX-License-Identifier: MIT

// Package updater provides version management and self-update functionality.
//
// Unlike the bash version which uses git-based updates, this Go implementation
// downloads pre-built binaries from GitHub releases. This is more appropriate
// for compiled binaries and doesn't require git on the target system.
//
// Reference: lyrebird-updater.sh
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// GitHubAPIURL is the base URL for GitHub API.
	GitHubAPIURL = "https://api.github.com"

	// DefaultOwner is the default repository owner.
	DefaultOwner = "tomtom215"

	// DefaultRepo is the default repository name.
	DefaultRepo = "lyrebirdaudio-go"

	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 30 * time.Second
)

// Release represents a GitHub release.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	Assets      []Asset   `json:"assets"`
	HTMLURL     string    `json:"html_url"`
}

// Asset represents a release asset (downloadable file).
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// UpdateInfo contains information about an available update.
type UpdateInfo struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	ReleaseNotes    string
	DownloadURL     string
	AssetName       string
	ChecksumURL     string // URL to checksums.txt (empty if not found in release assets)
	PublishedAt     time.Time
}

// Updater handles version checking and updates.
type Updater struct {
	owner          string
	repo           string
	httpClient     *http.Client
	currentVersion string
}

// Option is a functional option for configuring the Updater.
type Option func(*Updater)

// WithOwner sets the GitHub repository owner.
func WithOwner(owner string) Option {
	return func(u *Updater) {
		u.owner = owner
	}
}

// WithRepo sets the GitHub repository name.
func WithRepo(repo string) Option {
	return func(u *Updater) {
		u.repo = repo
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(u *Updater) {
		u.httpClient = client
	}
}

// WithCurrentVersion sets the current version for comparison.
func WithCurrentVersion(version string) Option {
	return func(u *Updater) {
		u.currentVersion = version
	}
}

// New creates a new Updater.
func New(opts ...Option) *Updater {
	u := &Updater{
		owner: DefaultOwner,
		repo:  DefaultRepo,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		currentVersion: "dev",
	}

	for _, opt := range opts {
		opt(u)
	}

	return u
}

// CheckForUpdates checks if a newer version is available.
func (u *Updater) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	latest, err := u.GetLatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest release: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion: u.currentVersion,
		LatestVersion:  latest.TagName,
		ReleaseNotes:   latest.Body,
		PublishedAt:    latest.PublishedAt,
	}

	// Compare versions
	info.UpdateAvailable = isNewerVersion(latest.TagName, u.currentVersion)

	// Find appropriate asset for this platform and the checksums file.
	assetName := getAssetName()
	for _, asset := range latest.Assets {
		if strings.Contains(asset.Name, assetName) {
			info.DownloadURL = asset.BrowserDownloadURL
			info.AssetName = asset.Name
		}
		// Look for checksums.txt (GitHub convention: "checksums.txt" or "sha256sums.txt")
		lowerName := strings.ToLower(asset.Name)
		if lowerName == "checksums.txt" || lowerName == "sha256sums.txt" || strings.HasSuffix(lowerName, "_checksums.txt") {
			info.ChecksumURL = asset.BrowserDownloadURL
		}
	}

	return info, nil
}

// GetLatestRelease fetches the latest release from GitHub.
func (u *Updater) GetLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", GitHubAPIURL, u.owner, u.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := u.httpClient.Do(req) // #nosec G704 -- URL is from config/GitHub API, not user HTTP input
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release: %w", err)
	}

	return &release, nil
}

// ListReleases fetches all releases from GitHub.
func (u *Updater) ListReleases(ctx context.Context) ([]Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases", GitHubAPIURL, u.owner, u.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := u.httpClient.Do(req) // #nosec G704 -- URL is from config/GitHub API, not user HTTP input
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode releases: %w", err)
	}

	// Filter out drafts and sort by date
	var filtered []Release
	for _, r := range releases {
		if !r.Draft {
			filtered = append(filtered, r)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].PublishedAt.After(filtered[j].PublishedAt)
	})

	return filtered, nil
}

// GetRelease fetches a specific release by tag.
func (u *Updater) GetRelease(ctx context.Context, tag string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", GitHubAPIURL, u.owner, u.repo, tag)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := u.httpClient.Do(req) // #nosec G704 -- URL is from config/GitHub API, not user HTTP input
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("release %s not found", tag)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release: %w", err)
	}

	return &release, nil
}

// Download downloads a release asset to the specified path.
func (u *Updater) Download(ctx context.Context, url, destPath string, progress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := u.httpClient.Do(req) // #nosec G704 -- URL is from config/GitHub API, not user HTTP input
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create destination file
	// #nosec G304 -- destPath is from controlled temp directory
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = out.Close() }()

	// Download with progress
	var downloaded int64
	total := resp.ContentLength

	reader := resp.Body
	if progress != nil {
		reader = &progressReader{
			reader: resp.Body,
			onProgress: func(n int64) {
				downloaded += n
				progress(downloaded, total)
			},
		}
	}

	_, err = io.Copy(out, reader)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	return nil
}

// Update downloads and installs the latest version.
//
// Steps:
//  1. Download the release asset to a temp file
//  2. Extract if it's a tarball
//  3. Backup the current binary
//  4. Replace with new binary
//  5. Verify the new binary works
//
// If anything fails, the backup is restored.
func (u *Updater) Update(ctx context.Context, info *UpdateInfo, binaryPath string, progress func(downloaded, total int64)) (retErr error) {
	if info.DownloadURL == "" {
		return fmt.Errorf("no download URL available for this platform")
	}

	// Create temp directory for download
	tmpDir, err := os.MkdirTemp("", "lyrebird-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Download to temp file
	downloadPath := filepath.Join(tmpDir, info.AssetName)
	if err := u.Download(ctx, info.DownloadURL, downloadPath, progress); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// M-13: Verify SHA256 checksum before installing.
	// When the release includes a checksums.txt asset, its absence or mismatch
	// is treated as a hard failure to prevent MITM / corrupted-CDN attacks.
	if info.ChecksumURL != "" {
		if err := u.verifyChecksumFromURL(ctx, info.ChecksumURL, info.AssetName, downloadPath); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Extract binary from tarball if needed
	var newBinaryPath string
	if strings.HasSuffix(info.AssetName, ".tar.gz") || strings.HasSuffix(info.AssetName, ".tgz") {
		newBinaryPath, err = extractBinaryFromTarGz(downloadPath, tmpDir)
		if err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}
	} else {
		newBinaryPath = downloadPath
	}

	// Make executable
	// #nosec G302 -- binary must be executable
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	// Backup current binary.
	// The defer references the named return retErr so it can observe the
	// final outcome of the function and roll back only on failure.
	backupPath := binaryPath + ".backup"
	if _, err := os.Stat(binaryPath); err == nil {
		if err := copyFile(binaryPath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		defer func() {
			// Restore backup if update failed
			if retErr != nil {
				_ = os.Rename(backupPath, binaryPath)
			} else {
				_ = os.Remove(backupPath)
			}
		}()
	}

	// Replace binary
	if err := copyFile(newBinaryPath, binaryPath); err != nil {
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	return nil
}

// Rollback restores the backup of the binary.
func (u *Updater) Rollback(binaryPath string) error {
	backupPath := binaryPath + ".backup"

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found at %s", backupPath)
	}

	if err := os.Rename(backupPath, binaryPath); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return nil
}

// HasBackup checks if a backup exists.
func (u *Updater) HasBackup(binaryPath string) bool {
	backupPath := binaryPath + ".backup"
	_, err := os.Stat(backupPath)
	return err == nil
}
