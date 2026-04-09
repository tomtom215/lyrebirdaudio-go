package updater

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownload(t *testing.T) {
	content := "test binary content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "19")
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "download.bin")

	u := New()
	var progressCalled bool
	err := u.Download(context.Background(), server.URL, destPath, func(downloaded, total int64) {
		progressCalled = true
	})

	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}

	if !progressCalled {
		t.Error("Progress callback was not called")
	}

	// Verify content
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(data) != content {
		t.Errorf("Downloaded content = %q, want %q", string(data), content)
	}
}

func TestDownloadErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "download.bin")

	u := New()
	err := u.Download(context.Background(), server.URL, destPath, nil)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestDownloadNoProgress(t *testing.T) {
	content := "test content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "download.bin")

	u := New()
	err := u.Download(context.Background(), server.URL, destPath, nil)
	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}

	data, _ := os.ReadFile(destPath)
	if string(data) != content {
		t.Errorf("Downloaded content = %q, want %q", string(data), content)
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := "test content"
	if err := os.WriteFile(srcPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Copy
	dstPath := filepath.Join(tmpDir, "dest.txt")
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}

	// Verify
	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(data) != content {
		t.Errorf("Copied content = %q, want %q", string(data), content)
	}
}

func TestCopyFileSourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dest"))
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestCopyFileDestError(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}

	// Try to copy to a nonexistent directory
	err := copyFile(srcPath, "/nonexistent/dir/dest.txt")
	if err == nil {
		t.Error("Expected error for nonexistent destination directory")
	}
}

func TestProgressReader(t *testing.T) {
	content := "hello world"
	r := &progressReader{
		reader: io.NopCloser(strings.NewReader(content)),
		onProgress: func(n int64) {
			// Just verify callback is called
		},
	}

	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != 5 {
		t.Errorf("Read() = %d, want 5", n)
	}

	if err := r.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}
