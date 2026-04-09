// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFormatIntSliceForDetect verifies formatting of integer slices.
func TestFormatIntSliceForDetect(t *testing.T) {
	tests := []struct {
		name string
		vals []int
		want string
	}{
		{
			name: "empty slice",
			vals: []int{},
			want: "",
		},
		{
			name: "single value",
			vals: []int{48000},
			want: "48000",
		},
		{
			name: "multiple values",
			vals: []int{8000, 16000, 44100, 48000},
			want: "8000, 16000, 44100, 48000",
		},
		{
			name: "channels",
			vals: []int{1, 2},
			want: "1, 2",
		},
		{
			name: "single zero",
			vals: []int{0},
			want: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatIntSliceForDetect(tt.vals)
			if got != tt.want {
				t.Errorf("formatIntSliceForDetect(%v) = %q, want %q", tt.vals, got, tt.want)
			}
		})
	}
}

// TestCreateTarGzWithSubdirectories verifies tar.gz handles nested directories.
func TestCreateTarGzWithSubdirectories(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// Create nested structure
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested content"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root content"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	outPath := filepath.Join(outDir, "nested.tar.gz")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = createTarGz(outFile, srcDir)
	outFile.Close()
	if err != nil {
		t.Fatalf("createTarGz() unexpected error: %v", err)
	}

	// Verify archive is non-empty
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("archive file is empty")
	}
}
