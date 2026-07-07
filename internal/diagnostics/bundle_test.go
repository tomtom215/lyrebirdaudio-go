// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportBundle_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "test.tar.gz")

	opts := BundleOptions{
		Options: DefaultOptions(),
	}

	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat bundle: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty bundle file")
	}
}

func TestExportBundle_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "bundle.tar.gz")

	opts := BundleOptions{Options: DefaultOptions()}
	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("expected mode 0600, got %04o", mode)
	}
}

func TestExportBundle_ContainsExpectedEntries(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "bundle.tar.gz")

	opts := BundleOptions{Options: DefaultOptions()}
	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	entries := readTarEntries(t, dst)

	var hasDiag, hasSysInfo bool
	for _, e := range entries {
		base := filepath.Base(e)
		if base == "diagnostics.json" {
			hasDiag = true
		}
		if base == "system-info.txt" {
			hasSysInfo = true
		}
	}

	if !hasDiag {
		t.Errorf("expected diagnostics.json in bundle, got: %v", entries)
	}
	if !hasSysInfo {
		t.Errorf("expected system-info.txt in bundle, got: %v", entries)
	}
}

func TestExportBundle_DiagnosticsJSONIsValid(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "bundle.tar.gz")

	opts := BundleOptions{Options: DefaultOptions()}
	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	data := readTarFile(t, dst, "diagnostics.json")
	if len(data) == 0 {
		t.Fatal("diagnostics.json is empty")
	}

	var report DiagnosticReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON in diagnostics.json: %v", err)
	}
	if report.Summary == nil {
		t.Error("report.Summary is nil")
	}
	if report.Summary.Total == 0 {
		t.Error("report.Summary.Total is 0 — no checks ran")
	}
}

func TestExportBundle_SystemInfoContainsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "bundle.tar.gz")

	opts := BundleOptions{Options: DefaultOptions()}
	opts.ConfigPath = "/etc/lyrebird/custom.yaml"
	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	data := readTarFile(t, dst, "system-info.txt")
	if !strings.Contains(string(data), "custom.yaml") {
		t.Errorf("system-info.txt should contain ConfigPath, got:\n%s", string(data))
	}
}

func TestExportBundle_IncludesLogSnippets(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	if err := os.MkdirAll(logDir, 0750); err != nil {
		t.Fatal(err)
	}
	// Write a log file with known content.
	logContent := strings.Repeat("log line\n", 10)
	if err := os.WriteFile(filepath.Join(logDir, "stream.log"), []byte(logContent), 0640); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(tmpDir, "bundle.tar.gz")
	opts := BundleOptions{Options: DefaultOptions(), MaxLogLines: 5}
	opts.LogDir = logDir
	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	entries := readTarEntries(t, dst)
	var hasLog bool
	for _, e := range entries {
		if filepath.Base(e) == "stream.log" {
			hasLog = true
		}
	}
	if !hasLog {
		t.Errorf("expected stream.log in bundle, got: %v", entries)
	}
}

func TestExportBundle_MaxLogLinesDefault(t *testing.T) {
	// MaxLogLines == 0 should default to 200.
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "bundle.tar.gz")

	opts := BundleOptions{Options: DefaultOptions(), MaxLogLines: 0}
	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	// Just verify it succeeds with default lines.
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("bundle not created: %v", err)
	}
}

func TestExportBundle_ContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "bundle.tar.gz")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	opts := BundleOptions{Options: DefaultOptions()}
	// Context cancelled before checks run — Run() should return ctx.Err(),
	// but ExportBundle wraps it and returns an error.
	// We just verify it doesn't panic.
	_ = ExportBundle(ctx, opts, dst)
}

func TestExportBundle_InvalidDstPath(t *testing.T) {
	opts := BundleOptions{Options: DefaultOptions()}
	err := ExportBundle(context.Background(), opts, "/nonexistent-directory/bundle.tar.gz")
	if err == nil {
		t.Error("expected error for invalid destination path")
	}
}

func TestBundleSize(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "bundle.tar.gz")

	opts := BundleOptions{Options: DefaultOptions()}
	if err := ExportBundle(context.Background(), opts, dst); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	sz, err := BundleSize(dst)
	if err != nil {
		t.Fatalf("BundleSize: %v", err)
	}
	if sz <= 0 {
		t.Errorf("expected positive bundle size, got %d", sz)
	}
}

func TestBundleSize_NonExistent(t *testing.T) {
	_, err := BundleSize("/nonexistent/file.tar.gz")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestCollectLogSnippets_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	snippets := collectLogSnippets(tmpDir, 50)
	if len(snippets) != 0 {
		t.Errorf("expected no snippets from empty dir, got %d", len(snippets))
	}
}

func TestCollectLogSnippets_NonexistentDir(t *testing.T) {
	snippets := collectLogSnippets("/nonexistent/log/dir", 50)
	if len(snippets) != 0 {
		t.Errorf("expected no snippets from nonexistent dir, got %d", len(snippets))
	}
}

func TestCollectLogSnippets_EmptyLogDir(t *testing.T) {
	snippets := collectLogSnippets("", 50)
	if len(snippets) != 0 {
		t.Errorf("expected no snippets for empty logDir, got %d", len(snippets))
	}
}

func TestCollectLogSnippets_SkipsNonLogFiles(t *testing.T) {
	tmpDir := t.TempDir()
	// Write a .txt file and a .log file.
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "app.log"), []byte("log\n"), 0644); err != nil {
		t.Fatal(err)
	}

	snippets := collectLogSnippets(tmpDir, 50)
	if _, ok := snippets["notes.txt"]; ok {
		t.Error("should not include non-.log files")
	}
	if _, ok := snippets["app.log"]; !ok {
		t.Error("should include app.log")
	}
}

func TestCollectLogSnippets_TailLines(t *testing.T) {
	tmpDir := t.TempDir()
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line")
	}
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "app.log"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	snippets := collectLogSnippets(tmpDir, 5)
	if _, ok := snippets["app.log"]; !ok {
		t.Fatal("expected app.log in snippets")
	}
}

func TestTailFile_ShortFile(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "short.log")
	if err := os.WriteFile(f, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result := tailFile(f, 100)
	if !strings.Contains(result, "line1") {
		t.Errorf("expected 'line1' in tail output, got %q", result)
	}
}

func TestTailFile_Nonexistent(t *testing.T) {
	result := tailFile("/nonexistent/file.log", 10)
	if result != "" {
		t.Errorf("expected empty string for nonexistent file, got %q", result)
	}
}

func TestBuildSystemInfoText(t *testing.T) {
	opts := DefaultOptions()
	result := buildSystemInfoText(opts)
	if !strings.Contains(result, "LyreBirdAudio") {
		t.Error("expected 'LyreBirdAudio' in system info text")
	}
	if !strings.Contains(result, opts.ConfigPath) {
		t.Errorf("expected ConfigPath %q in system info", opts.ConfigPath)
	}
}

// ---- helpers ----

// readTarEntries returns all entry names in a .tar.gz file.
func readTarEntries(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path) //#nosec G304 -- test helper, path from t.TempDir()
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

// readTarFile returns the content of a specific file inside a .tar.gz archive.
// The search is by base filename (not full path within archive).
func readTarFile(t *testing.T, archivePath, baseName string) []byte {
	t.Helper()
	f, err := os.Open(archivePath) //#nosec G304 -- test helper, path from t.TempDir()
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		if filepath.Base(hdr.Name) == baseName {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read %s: %v", baseName, err)
			}
			return data
		}
	}
	t.Fatalf("file %q not found in archive", baseName)
	return nil
}
