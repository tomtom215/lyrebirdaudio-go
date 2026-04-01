// SPDX-License-Identifier: MIT

package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestIsNewerVersionReleaseNewerThanPreRelease covers version.go:102-104 —
// the `lPre == "" && cPre != ""` branch where the latest version is a full
// release and the current version is a pre-release of the same numbers.
// Example: latest=v1.0.0 (no pre-release) vs current=v1.0.0-rc1 (pre-release).
func TestIsNewerVersionReleaseNewerThanPreRelease(t *testing.T) {
	// v1.0.0 (latest, no pre-release) should be considered newer than
	// v1.0.0-rc1 (current, pre-release).
	if !isNewerVersion("v1.0.0", "v1.0.0-rc1") {
		t.Error("isNewerVersion(v1.0.0, v1.0.0-rc1) = false, want true")
	}
}

// TestUpdateChecksumURLInvalidDownload covers updater.go:345-348 — the
// verifyChecksumFromURL error branch in Update. We set info.ChecksumURL to
// an invalid URL (null byte), so the Download call inside verifyChecksumFromURL
// fails, propagating the error back through Update.
func TestUpdateChecksumURLInvalidDownload(t *testing.T) {
	// Serve a valid binary download (for the main asset).
	content := []byte("fake binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	targetBinary := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(targetBinary, []byte("old"), 0755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	u := newTestUpdater(srv.URL)
	info := &UpdateInfo{
		DownloadURL: srv.URL + "/binary",
		AssetName:   "lyrebird-linux-amd64",
		// ChecksumURL with null byte causes http.NewRequestWithContext to fail.
		ChecksumURL:     "http://bad\x00host/checksums.txt",
		UpdateAvailable: true,
	}

	err := u.Update(context.Background(), info, targetBinary, nil)
	if err == nil {
		t.Error("Update() expected error from checksum verification failure, got nil")
	}
}

// TestUpdateExtractionError covers updater.go:355-357 — the
// extractBinaryFromTarGz error branch in Update. We serve a tar.gz archive
// that contains no file named "lyrebird", so extraction fails with
// "binary not found in archive".
func TestUpdateExtractionError(t *testing.T) {
	// Build a tar.gz archive without a "lyrebird" entry.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name:     "other-binary",
		Mode:     0755,
		Size:     4,
		Typeflag: tar.TypeReg,
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("data"))
	_ = tw.Close()
	_ = gz.Close()
	archiveData := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archiveData)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	targetBinary := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(targetBinary, []byte("old"), 0755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	u := newTestUpdater(srv.URL)
	info := &UpdateInfo{
		DownloadURL:     srv.URL + "/release.tar.gz",
		AssetName:       "lyrebird-linux-amd64.tar.gz",
		UpdateAvailable: true,
	}

	err := u.Update(context.Background(), info, targetBinary, nil)
	if err == nil {
		t.Error("Update() expected extraction error for archive without lyrebird, got nil")
	}
}

// TestRollbackRenameError covers updater.go:402-404 — the os.Rename error
// path in Rollback. We create the backup file but make the parent directory
// of the target path unwritable (as a non-root user) so os.Rename fails.
func TestRollbackRenameError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission enforcement not applicable as root")
	}

	tmpDir := t.TempDir()

	// Create the backup file.
	backupPath := filepath.Join(tmpDir, "lyrebird.backup")
	if err := os.WriteFile(backupPath, []byte("backup"), 0644); err != nil {
		t.Fatalf("WriteFile backup: %v", err)
	}

	// Make the target directory read-only so os.Rename fails.
	roDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(roDir, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(roDir, 0500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0750) })

	u := New()
	// The target path (binaryPath) is inside the read-only directory; the
	// backup exists at binaryPath+".backup". os.Rename will fail because
	// the parent directory is read-only.
	binaryPath := filepath.Join(roDir, "lyrebird")
	// Place the backup at binaryPath+".backup".
	roBackup := binaryPath + ".backup"
	// We need backup in the same parent dir. Copy backupPath there (will fail if dir is ro).
	// Instead, write the backup before making dir read-only.
	if err := os.Chmod(roDir, 0750); err != nil {
		t.Fatalf("re-chmod: %v", err)
	}
	if err := os.WriteFile(roBackup, []byte("backup"), 0644); err != nil {
		t.Fatalf("WriteFile roBackup: %v", err)
	}
	if err := os.Chmod(roDir, 0500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	err := u.Rollback(binaryPath)
	if err == nil {
		t.Error("Rollback() expected rename error for read-only directory, got nil")
	}
}

// TestVerifyChecksumFromURLDownloadFails covers archive.go:117-119 — the
// Download error path in verifyChecksumFromURL. By passing an invalid
// checksumURL (null byte), the internal Download call fails immediately.
func TestVerifyChecksumFromURLDownloadFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	tmpDir := t.TempDir()
	downloadedPath := filepath.Join(tmpDir, "asset.tar.gz")
	if err := os.WriteFile(downloadedPath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Invalid checksumURL (null byte) causes http.NewRequestWithContext to fail.
	err := u.verifyChecksumFromURL(context.Background(),
		"http://invalid\x00host/checksums.txt",
		"lyrebird-linux-amd64.tar.gz",
		downloadedPath)
	if err == nil {
		t.Error("verifyChecksumFromURL() expected error for invalid checksumURL, got nil")
	}
}

// TestVerifyChecksumFromURLChecksumNotFound covers archive.go:130-132 —
// the ParseChecksumFile error path when the downloaded checksums file does
// not contain an entry for the requested asset name.
func TestVerifyChecksumFromURLChecksumNotFound(t *testing.T) {
	// Serve a checksums file that does NOT contain our asset name.
	checksumContent := "abc123  other-asset.tar.gz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(checksumContent))
	}))
	defer srv.Close()

	u := newTestUpdater(srv.URL)
	tmpDir := t.TempDir()
	downloadedPath := filepath.Join(tmpDir, "asset.tar.gz")
	if err := os.WriteFile(downloadedPath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := u.verifyChecksumFromURL(context.Background(),
		srv.URL+"/checksums.txt",
		"lyrebird-linux-amd64.tar.gz",
		downloadedPath)
	if err == nil {
		t.Error("verifyChecksumFromURL() expected error when asset not in checksums, got nil")
	}
}
