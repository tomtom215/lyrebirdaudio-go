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

	// The release also ships a checksums file, so this exercises the full
	// verified happy path under the fail-closed default (no opt-out needed).
	const assetName = "lyrebird-linux-amd64.tar.gz"
	checksums := sha256HexTest(archiveContent) + "  " + assetName + "\n"

	// Create mock download server. It serves the checksums file on the
	// checksums path and the archive on every other path.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = w.Write([]byte(checksums))
			return
		}
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
		AssetName:   assetName,
		ChecksumURL: server.URL + "/checksums.txt",
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
