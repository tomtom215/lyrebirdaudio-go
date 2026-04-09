package updater

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
