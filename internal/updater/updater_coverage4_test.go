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

// TestUpdateDownloadServerError covers updater.go:338-340 — the Download
// error branch in Update. The server returns 500 for the asset download,
// causing Download to fail and Update to return "download failed".
func TestUpdateDownloadServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	targetBinary := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(targetBinary, []byte("old"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	u := newTestUpdater(srv.URL)
	info := &UpdateInfo{
		DownloadURL:     srv.URL + "/binary",
		AssetName:       "lyrebird-linux-amd64",
		UpdateAvailable: true,
	}

	err := u.Update(context.Background(), info, targetBinary, nil)
	if err == nil {
		t.Error("Update() expected error for download server error, got nil")
	}
}

// TestUpdateEmptyDownloadURL covers updater.go:325-327 — the first guard in
// Update that rejects an empty DownloadURL.
func TestUpdateEmptyDownloadURL(t *testing.T) {
	u := New()
	info := &UpdateInfo{
		DownloadURL:     "",
		AssetName:       "lyrebird-linux-amd64",
		UpdateAvailable: true,
	}

	err := u.Update(context.Background(), info, "/tmp/lyrebird", nil)
	if err == nil {
		t.Error("Update() expected error for empty DownloadURL, got nil")
	}
}

// TestUpdateBackupCopyFileError covers updater.go:373-375 — the
// "failed to create backup" error branch. The target binary path is a
// directory: os.Stat succeeds (directory exists), but copyFile opens the
// directory and io.Copy fails with EISDIR, causing the backup to fail.
func TestUpdateBackupCopyFileError(t *testing.T) {
	content := []byte("fake binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()

	// Create a directory at the target binary path.
	// os.Stat(targetBinary) succeeds (directory exists), but
	// copyFile(targetBinary, backupPath) fails because reading a directory
	// returns EISDIR.
	targetBinary := filepath.Join(tmpDir, "lyrebird")
	if err := os.Mkdir(targetBinary, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	u := newTestUpdater(srv.URL)
	info := &UpdateInfo{
		DownloadURL:     srv.URL + "/binary",
		AssetName:       "lyrebird-linux-amd64", // non-tarball
		UpdateAvailable: true,
	}

	err := u.Update(context.Background(), info, targetBinary, nil)
	if err == nil {
		t.Error("Update() expected error when target binary path is a directory, got nil")
	}
}
