// SPDX-License-Identifier: MIT

package updater

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// makeTestArchive creates a tar.gz archive with the given files in a temp dir.
func makeTestArchive(t *testing.T, dir string, files map[string][]byte) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test_archive.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gzw := gzip.NewWriter(f)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}

	return archivePath
}

func TestExtractBinaryFromTarGz_NoBinaryFound(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := makeTestArchive(t, tmpDir, map[string][]byte{
		"README.md":   []byte("# Docs\n"),
		"LICENSE":     []byte("MIT\n"),
		"config.yaml": []byte("key: value\n"),
	})

	destDir := t.TempDir()
	_, err := extractBinaryFromTarGz(archivePath, destDir)
	if err == nil {
		t.Error("expected error for archive without lyrebird binary")
	}
	if err.Error() != "binary not found in archive" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractBinaryFromTarGz_CorruptedData(t *testing.T) {
	tmpDir := t.TempDir()
	corruptPath := filepath.Join(tmpDir, "corrupt.tar.gz")
	if err := os.WriteFile(corruptPath, []byte("this is not a tar.gz"), 0644); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	_, err := extractBinaryFromTarGz(corruptPath, destDir)
	if err == nil {
		t.Error("expected error for corrupted archive")
	}
}

func TestExtractBinaryFromTarGz_EmptyTar(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "empty.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)
	tw.Close()
	gzw.Close()
	f.Close()

	destDir := t.TempDir()
	_, err = extractBinaryFromTarGz(archivePath, destDir)
	if err == nil {
		t.Error("expected error for empty archive")
	}
}

func TestExtractBinaryFromTarGz_NonExistentPath(t *testing.T) {
	destDir := t.TempDir()
	_, err := extractBinaryFromTarGz("/nonexistent/file.tar.gz", destDir)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestExtractBinaryFromTarGz_NestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := makeTestArchive(t, tmpDir, map[string][]byte{
		"bin/lyrebird": []byte("#!/bin/sh\necho nested\n"),
	})

	destDir := t.TempDir()
	binaryPath, err := extractBinaryFromTarGz(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractBinaryFromTarGz() error = %v", err)
	}

	if filepath.Base(binaryPath) != "lyrebird" {
		t.Errorf("expected lyrebird, got %q", filepath.Base(binaryPath))
	}
}

func TestExtractBinaryFromTarGz_OnlyStreamNoPrimary(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := makeTestArchive(t, tmpDir, map[string][]byte{
		"lyrebird-stream": []byte("#!/bin/sh\necho stream\n"),
	})

	destDir := t.TempDir()
	_, err := extractBinaryFromTarGz(archivePath, destDir)
	// binaryPath is only set for "lyrebird", not "lyrebird-stream"
	if err == nil {
		t.Error("expected error when only lyrebird-stream is present")
	}
}

func TestParseChecksumFile_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		assetName string
		wantHash  string
		wantErr   bool
	}{
		{
			name:      "binary mode marker",
			content:   "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2 *lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:      "with path prefix",
			content:   "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2  dist/lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:      "with comments and empty lines",
			content:   "# SHA256 checksums\n\na1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2  lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:      "uppercase hash normalized",
			content:   "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4E5F6A1B2  lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:      "asset not in file",
			content:   "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2  other-file.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantErr:   true,
		},
		{
			name:      "empty content",
			content:   "",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantErr:   true,
		},
		{
			name:      "invalid hash length",
			content:   "shorthash  lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantErr:   true,
		},
		{
			name:      "single field lines skipped",
			content:   "nothinghere\na1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2  lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:      "multiple entries selects correct one",
			content:   "0000000000000000000000000000000000000000000000000000000000000000  other.tar.gz\na1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2  lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := ParseChecksumFile(tt.content, tt.assetName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got hash %q", hash)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}
		})
	}
}

func TestCopyFilePreservesContent(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src_file")
	dst := filepath.Join(tmpDir, "dst_file")

	// Create a file with specific content and permissions
	content := []byte("test content for copy verification")
	if err := os.WriteFile(src, content, 0755); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}
