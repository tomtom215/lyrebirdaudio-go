// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckLogFilesLargeFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a log file exceeding LogSizeWarningBytes (100 MiB)
	// Use a sparse file to avoid actually writing 100+ MiB
	largePath := filepath.Join(tmpDir, "large.log")
	f, err := os.Create(largePath)
	if err != nil {
		t.Fatalf("failed to create large log file: %v", err)
	}
	// Truncate to 101 MiB to exceed threshold
	if err := f.Truncate(101 * 1024 * 1024); err != nil {
		_ = f.Close()
		t.Fatalf("failed to truncate file: %v", err)
	}
	_ = f.Close()

	opts := DefaultOptions()
	opts.LogDir = tmpDir
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for large log files, got %s: %s", result.Status, result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions for large log files")
	}
}

func TestCheckLogFilesWithSmallFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create small log files
	for i := 0; i < 5; i++ {
		name := filepath.Join(tmpDir, fmt.Sprintf("app_%d.log", i))
		if err := os.WriteFile(name, []byte("some log content\n"), 0644); err != nil {
			t.Fatalf("failed to create log file: %v", err)
		}
	}

	opts := DefaultOptions()
	opts.LogDir = tmpDir
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for small log files, got %s", result.Status)
	}
}

func TestCheckLogFilesSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory with log files
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.log"), []byte("nested log"), 0644); err != nil {
		t.Fatalf("failed to create nested log file: %v", err)
	}

	opts := DefaultOptions()
	opts.LogDir = tmpDir
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
}
