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
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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
func (u *Updater) Update(ctx context.Context, info *UpdateInfo, binaryPath string, progress func(downloaded, total int64)) error {
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

	// Backup current binary
	backupPath := binaryPath + ".backup"
	if _, err := os.Stat(binaryPath); err == nil {
		if err := copyFile(binaryPath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		defer func() {
			// Restore backup if update failed
			if err != nil {
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

// verifyChecksumFromURL downloads checksums.txt and verifies the downloaded
// asset matches the expected SHA256 hash.
func (u *Updater) verifyChecksumFromURL(ctx context.Context, checksumURL, assetName, downloadedPath string) error {
	// Download checksums.txt to a temp file
	tmpFile, err := os.CreateTemp("", "lyrebird-checksums-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file for checksums: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := u.Download(ctx, checksumURL, tmpPath, nil); err != nil {
		return fmt.Errorf("failed to download checksums.txt: %w", err)
	}

	// Read checksums file
	// #nosec G304 -- path is a controlled temp file created above
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to read checksums file: %w", err)
	}

	// Parse expected checksum for this asset
	expectedHash, err := ParseChecksumFile(string(data), assetName)
	if err != nil {
		return fmt.Errorf("checksum not found for %s: %w", assetName, err)
	}

	// Compute actual SHA256 of the downloaded file
	// #nosec G304 -- downloadedPath is a controlled temp path
	fileData, err := os.ReadFile(downloadedPath)
	if err != nil {
		return fmt.Errorf("failed to read downloaded file: %w", err)
	}
	sum := sha256.Sum256(fileData)
	actualHash := hex.EncodeToString(sum[:])

	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, expectedHash, actualHash)
	}

	return nil
}

// ParseChecksumFile parses a sha256sums-style checksum file and returns the
// hex-encoded SHA256 hash for the named asset.
//
// The expected format is one entry per line:
//
//	<sha256hex>  <filename>
//
// Both one-space and two-space separators (GNU coreutils format) are accepted.
// Returns an error if the asset is not found.
func ParseChecksumFile(content, assetName string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Fields: <hash>  <filename>  (two spaces) or <hash> <filename>
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		// The filename may have a leading '*' (binary mode marker from sha256sum -b)
		name := strings.TrimPrefix(parts[1], "*")
		// Match by basename to be robust against path prefixes
		if filepath.Base(name) == filepath.Base(assetName) {
			// Validate that it looks like a hex SHA256 (64 chars)
			if len(hash) != 64 {
				return "", fmt.Errorf("invalid SHA256 hash length %d for %s", len(hash), assetName)
			}
			return strings.ToLower(hash), nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %q in checksums file", assetName)
}

// progressReader wraps a reader to report progress.
type progressReader struct {
	reader     io.ReadCloser
	onProgress func(int64)
}

func (r *progressReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 && r.onProgress != nil {
		r.onProgress(int64(n))
	}
	return
}

func (r *progressReader) Close() error {
	return r.reader.Close()
}

// parseSemver parses a version string into major, minor, patch components
// and an optional pre-release suffix. Returns ok=false if the string cannot
// be parsed as a valid semver-like version.
//
// Accepted formats: "1.2.3", "v1.2.3", "1.2.3-rc1", "1.2", "1"
func parseSemver(version string) (major, minor, patch int, prerelease string, ok bool) {
	// Strip 'v' prefix
	version = strings.TrimPrefix(version, "v")

	if version == "" {
		return 0, 0, 0, "", false
	}

	// Split off pre-release suffix (e.g., "1.0.0-rc1" -> "1.0.0", "rc1")
	versionCore := version
	if idx := strings.IndexByte(version, '-'); idx >= 0 {
		prerelease = version[idx+1:]
		versionCore = version[:idx]
	}

	parts := strings.Split(versionCore, ".")

	// Parse major (required)
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, "", false
	}
	major = maj

	// Parse minor (optional, defaults to 0)
	if len(parts) >= 2 {
		min, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, "", false
		}
		minor = min
	}

	// Parse patch (optional, defaults to 0)
	if len(parts) >= 3 {
		pat, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, "", false
		}
		patch = pat
	}

	return major, minor, patch, prerelease, true
}

// isNewerVersion compares two version strings.
// Returns true if latest is newer than current.
// Properly handles multi-digit version components (e.g., 0.10.0 > 0.9.0)
// and pre-release suffixes (e.g., 1.0.0 > 1.0.0-rc1).
func isNewerVersion(latest, current string) bool {
	// Handle "dev" / "unknown" current version -- always offer update
	if current == "dev" || current == "unknown" {
		return true
	}

	// Parse both versions
	lMajor, lMinor, lPatch, lPre, lOk := parseSemver(latest)
	cMajor, cMinor, cPatch, cPre, cOk := parseSemver(current)

	// If latest cannot be parsed as semver, it is not a valid newer version
	if !lOk {
		return false
	}

	// If current cannot be parsed, treat as unknown and offer update
	if !cOk {
		return true
	}

	// Compare major.minor.patch numerically
	if lMajor != cMajor {
		return lMajor > cMajor
	}
	if lMinor != cMinor {
		return lMinor > cMinor
	}
	if lPatch != cPatch {
		return lPatch > cPatch
	}

	// Same major.minor.patch: handle pre-release comparison.
	// A release version (no pre-release) is considered newer than
	// a pre-release version with the same numeric components.
	// e.g., 1.0.0 > 1.0.0-rc1
	if lPre == "" && cPre != "" {
		return true // release is newer than pre-release
	}
	if lPre != "" && cPre == "" {
		return false // pre-release is not newer than release
	}

	// Both have pre-release or both don't: same version, not newer
	return false
}

// getAssetName returns the expected asset name for this platform.
func getAssetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Map Go arch names to common conventions
	archMap := map[string]string{
		"amd64": "amd64",
		"386":   "386",
		"arm64": "arm64",
		"arm":   "arm",
	}

	if mapped, ok := archMap[arch]; ok {
		arch = mapped
	}

	return fmt.Sprintf("lyrebird-%s-%s", os, arch)
}

// extractBinaryFromTarGz extracts the binary from a tar.gz archive.
func extractBinaryFromTarGz(archivePath, destDir string) (string, error) {
	// #nosec G304 -- archivePath is from controlled temp directory
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)

	// Maximum binary size: 100MB (protection against decompression bombs)
	const maxBinarySize = 100 * 1024 * 1024

	var binaryPath string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Look for the main binary
		name := filepath.Base(header.Name)
		if name == "lyrebird" || name == "lyrebird-stream" {
			destPath := filepath.Join(destDir, name)
			outFile, err := os.Create(destPath) // #nosec G304 G703 -- destPath is constructed from controlled destDir
			if err != nil {
				return "", err
			}

			// Limit copy size to prevent decompression bombs
			limitReader := io.LimitReader(tarReader, maxBinarySize)
			if _, err := io.Copy(outFile, limitReader); err != nil {
				_ = outFile.Close()
				return "", err
			}
			if err := outFile.Close(); err != nil {
				return "", err
			}

			if name == "lyrebird" {
				binaryPath = destPath
			}
		}
	}

	if binaryPath == "" {
		return "", fmt.Errorf("binary not found in archive")
	}

	return binaryPath, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	// #nosec G304 -- src is from controlled paths (binary backup/restore)
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	// Get source file info for permissions
	info, err := source.Stat()
	if err != nil {
		return err
	}

	dest, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode()) // #nosec G304 G703 -- dst is from controlled paths (binary backup/restore)
	if err != nil {
		return err
	}
	defer func() { _ = dest.Close() }()

	_, err = io.Copy(dest, source)
	return err
}

// FormatReleaseInfo formats release information for display.
func FormatReleaseInfo(release *Release) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Version: %s\n", release.TagName))
	sb.WriteString(fmt.Sprintf("Published: %s\n", release.PublishedAt.Format("2006-01-02 15:04")))

	if release.Name != "" && release.Name != release.TagName {
		sb.WriteString(fmt.Sprintf("Name: %s\n", release.Name))
	}

	if release.Prerelease {
		sb.WriteString("Type: Pre-release\n")
	}

	if len(release.Assets) > 0 {
		sb.WriteString(fmt.Sprintf("Assets: %d available\n", len(release.Assets)))
	}

	if release.Body != "" {
		sb.WriteString("\nRelease Notes:\n")
		sb.WriteString(release.Body)
	}

	return sb.String()
}

// FormatUpdateInfo formats update information for display.
func FormatUpdateInfo(info *UpdateInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Current version: %s\n", info.CurrentVersion))
	sb.WriteString(fmt.Sprintf("Latest version:  %s\n", info.LatestVersion))

	if info.UpdateAvailable {
		sb.WriteString("\n✓ Update available!\n")
		if info.PublishedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("Published: %s\n", info.PublishedAt.Format("2006-01-02")))
		}
	} else {
		sb.WriteString("\n✓ You are running the latest version.\n")
	}

	return sb.String()
}
