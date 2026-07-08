// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateTarGz verifies tar.gz archive creation.
func TestCreateTarGz(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // relative path -> content
	}{
		{
			name: "single file",
			files: map[string]string{
				"hello.txt": "Hello, World!",
			},
		},
		{
			name: "multiple files",
			files: map[string]string{
				"file1.txt":  "content one",
				"file2.txt":  "content two",
				"readme.txt": "some readme content",
			},
		},
		{
			name:  "empty directory",
			files: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir := t.TempDir()
			outDir := t.TempDir()

			// Create source files
			for name, content := range tt.files {
				filePath := filepath.Join(srcDir, name)
				if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
					t.Fatalf("failed to create source file: %v", err)
				}
			}

			// Create tar.gz
			outPath := filepath.Join(outDir, "test.tar.gz")
			outFile, err := os.Create(outPath)
			if err != nil {
				t.Fatalf("failed to create output file: %v", err)
			}

			err = createTarGz(outFile, srcDir)
			outFile.Close()
			if err != nil {
				t.Fatalf("createTarGz() unexpected error: %v", err)
			}

			// Verify the archive is valid and contains expected files
			f, err := os.Open(outPath)
			if err != nil {
				t.Fatalf("failed to open archive: %v", err)
			}
			defer f.Close()

			gzr, err := gzip.NewReader(f)
			if err != nil {
				t.Fatalf("failed to create gzip reader: %v", err)
			}
			defer gzr.Close()

			tr := tar.NewReader(gzr)
			foundFiles := make(map[string]string)

			for {
				hdr, err := tr.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("tar read error: %v", err)
				}

				content, err := io.ReadAll(tr)
				if err != nil {
					t.Fatalf("failed to read tar entry: %v", err)
				}
				foundFiles[hdr.Name] = string(content)
			}

			// Verify all expected files are present
			for name, expectedContent := range tt.files {
				gotContent, ok := foundFiles[name]
				if !ok {
					t.Errorf("expected file %q not found in archive", name)
					continue
				}
				if gotContent != expectedContent {
					t.Errorf("file %q content = %q, want %q", name, gotContent, expectedContent)
				}
			}

			// Verify no unexpected files
			if len(foundFiles) != len(tt.files) {
				t.Errorf("archive contains %d files, want %d", len(foundFiles), len(tt.files))
			}
		})
	}
}

// TestCreateTarGzInvalidSrcDir verifies error handling for nonexistent source.
func TestCreateTarGzInvalidSrcDir(t *testing.T) {
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "test.tar.gz")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	err = createTarGz(outFile, "/nonexistent/directory")
	if err == nil {
		t.Error("createTarGz() expected error for nonexistent directory, got nil")
	}
}

// TestCreateTarGzFilePermissions verifies tar headers have correct mode.
func TestCreateTarGzFilePermissions(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// Create a source file
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("data"), 0600); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	outPath := filepath.Join(outDir, "perm-test.tar.gz")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}

	err = createTarGz(outFile, srcDir)
	outFile.Close()
	if err != nil {
		t.Fatalf("createTarGz() unexpected error: %v", err)
	}

	// Read back and check permissions
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("failed to open archive: %v", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader error: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar next error: %v", err)
	}

	if hdr.Mode != 0600 {
		t.Errorf("tar header mode = %o, want 0600", hdr.Mode)
	}
}
