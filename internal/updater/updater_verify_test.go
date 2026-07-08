// SPDX-License-Identifier: MIT

package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveArchive builds a tar.gz containing a "lyrebird" binary with the given
// content and returns a server that serves it on every path, plus the archive
// bytes. The caller closes the server.
func serveArchive(t *testing.T, binaryContent []byte) (*httptest.Server, []byte) {
	t.Helper()
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "release.tar.gz")
	createTestTarGz(t, archivePath, map[string][]byte{"lyrebird": binaryContent})
	archiveContent, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archiveContent)
	}))
	return srv, archiveContent
}

// TestUpdateFailsClosedWithoutChecksum verifies the fail-closed default: when a
// release provides no checksums asset (info.ChecksumURL == ""), Update refuses
// to install and leaves the existing binary untouched.
func TestUpdateFailsClosedWithoutChecksum(t *testing.T) {
	server, _ := serveArchive(t, []byte("new binary content"))
	defer server.Close()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(binaryPath, []byte("old version"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	u := New() // default: verification mandatory (fail-closed)
	u.httpClient = server.Client()

	info := &UpdateInfo{
		DownloadURL: server.URL + "/release.tar.gz",
		AssetName:   "lyrebird-linux-amd64.tar.gz",
		// ChecksumURL deliberately empty: the release shipped no checksums asset.
	}

	err := u.Update(context.Background(), info, binaryPath, nil)
	if err == nil {
		t.Fatal("Update() must fail closed when no checksum is available, got nil")
	}
	if !strings.Contains(err.Error(), "no checksum") {
		t.Errorf("error = %q, want it to mention 'no checksum'", err.Error())
	}

	// The existing binary must be left intact.
	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(got) != "old version" {
		t.Errorf("binary was modified to %q; it must be left intact when failing closed", got)
	}
	// Failure happens before the backup step, so no stray backup is left behind.
	if _, err := os.Stat(binaryPath + ".backup"); !os.IsNotExist(err) {
		t.Error("no backup should exist after failing closed before install")
	}
}

// TestUpdateAllowUnverifiedProceeds verifies the explicit opt-out: with
// WithAllowUnverified(true), Update installs even though the release provides no
// checksums asset.
func TestUpdateAllowUnverifiedProceeds(t *testing.T) {
	newContent := []byte("new binary content")
	server, _ := serveArchive(t, newContent)
	defer server.Close()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "lyrebird")
	if err := os.WriteFile(binaryPath, []byte("old version"), 0755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	u := New(WithAllowUnverified(true))
	u.httpClient = server.Client()

	info := &UpdateInfo{
		DownloadURL: server.URL + "/release.tar.gz",
		AssetName:   "lyrebird-linux-amd64.tar.gz",
	}

	if err := u.Update(context.Background(), info, binaryPath, nil); err != nil {
		t.Fatalf("Update() with WithAllowUnverified(true) must proceed, got: %v", err)
	}

	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(got) != string(newContent) {
		t.Errorf("binary = %q, want %q", got, newContent)
	}
}

// TestWithAllowUnverifiedOption verifies the option wiring and the fail-closed
// default value.
func TestWithAllowUnverifiedOption(t *testing.T) {
	if New().allowUnverified {
		t.Error("allowUnverified must default to false (fail-closed)")
	}
	if !New(WithAllowUnverified(true)).allowUnverified {
		t.Error("WithAllowUnverified(true) must set allowUnverified")
	}
}
