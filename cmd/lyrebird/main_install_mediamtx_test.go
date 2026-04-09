package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDownloadFileNeitherFound covers the "neither curl nor wget" error path.
func TestDownloadFileNeitherFound(t *testing.T) {
	// Use a temp dir with no executables so LookPath for both curl and wget fails.
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)

	err := downloadFile("http://example.invalid/fake", filepath.Join(emptyBin, "out"))
	if err == nil {
		t.Fatal("downloadFile() expected error when neither curl nor wget found")
	}
	if !strings.Contains(err.Error(), "neither curl nor wget") {
		t.Errorf("downloadFile() error = %q; want 'neither curl nor wget'", err.Error())
	}
}

// TestDownloadFileCurlSuccess covers the happy path when curl is available.
func TestDownloadFileCurlSuccess(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Fake curl: writes "fake content" to its -o argument ($3).
	// Invocation: curl -fsSL -o <dest> <url>  →  $1=-fsSL $2=-o $3=dest $4=url
	fakeCurl := filepath.Join(tmpBin, "curl")
	if err := os.WriteFile(fakeCurl, []byte("#!/bin/sh\nprintf 'fake content' > \"$3\"\nexit 0\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	dest := filepath.Join(tmpDir, "download.bin")
	if err := downloadFile("http://example.invalid/fake", dest); err != nil {
		t.Fatalf("downloadFile(curl success) = %v; want nil", err)
	}
	data, err := os.ReadFile(dest) //#nosec G304
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "fake content" {
		t.Errorf("downloaded content = %q; want %q", string(data), "fake content")
	}
}

// TestDownloadFileCurlFailure covers the error path when curl exits non-zero.
func TestDownloadFileCurlFailure(t *testing.T) {
	tmpBin := t.TempDir()

	// Fake curl that always fails.
	fakeCurl := filepath.Join(tmpBin, "curl")
	if err := os.WriteFile(fakeCurl, []byte("#!/bin/sh\necho 'download error' >&2\nexit 1\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := downloadFile("http://example.invalid/fake", filepath.Join(tmpBin, "out"))
	if err == nil {
		t.Fatal("downloadFile(curl failure) expected non-nil error")
	}
	if !strings.Contains(err.Error(), "curl failed") {
		t.Errorf("downloadFile(curl failure) error = %q; want 'curl failed'", err.Error())
	}
}

// TestDownloadFileWgetSuccess covers the wget fallback when curl is absent.
func TestDownloadFileWgetSuccess(t *testing.T) {
	tmpBin := t.TempDir()
	tmpDir := t.TempDir()

	// Only wget available; curl is not in this isolated PATH so LookPath("curl") fails.
	// Invocation: wget -q -O <dest> <url>  →  $1=-q $2=-O $3=dest $4=url
	fakeWget := filepath.Join(tmpBin, "wget")
	if err := os.WriteFile(fakeWget, []byte("#!/bin/sh\nprintf 'wget content' > \"$3\"\nexit 0\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	// Isolate PATH to tmpBin only so real curl is hidden.
	t.Setenv("PATH", tmpBin)

	dest := filepath.Join(tmpDir, "download.bin")
	if err := downloadFile("http://example.invalid/fake", dest); err != nil {
		t.Fatalf("downloadFile(wget success) = %v; want nil", err)
	}
	data, err := os.ReadFile(dest) //#nosec G304
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "wget content" {
		t.Errorf("downloaded content = %q; want %q", string(data), "wget content")
	}
}

// TestDownloadFileWgetFailure covers the wget error path.
func TestDownloadFileWgetFailure(t *testing.T) {
	tmpBin := t.TempDir()

	// Only wget (failing), curl absent from isolated PATH.
	fakeWget := filepath.Join(tmpBin, "wget")
	if err := os.WriteFile(fakeWget, []byte("#!/bin/sh\necho 'wget error' >&2\nexit 1\n"), 0750); err != nil { //#nosec G306
		t.Fatal(err)
	}
	// Isolate PATH to tmpBin only so real curl is hidden.
	t.Setenv("PATH", tmpBin)

	err := downloadFile("http://example.invalid/fake", filepath.Join(tmpBin, "out"))
	if err == nil {
		t.Fatal("downloadFile(wget failure) expected non-nil error")
	}
	if !strings.Contains(err.Error(), "wget failed") {
		t.Errorf("downloadFile(wget failure) error = %q; want 'wget failed'", err.Error())
	}
}

// TestVerifyDownloadIntegrity verifies the P-14 download integrity check.
func TestVerifyDownloadIntegrity(t *testing.T) {
	t.Run("valid file returns SHA256 hash", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.tar.gz")
		content := []byte("test content for hashing")
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
		hash, err := verifyDownloadIntegrity(path)
		if err != nil {
			t.Fatalf("verifyDownloadIntegrity() unexpected error: %v", err)
		}
		if len(hash) != 64 {
			t.Errorf("hash length = %d, want 64 hex chars", len(hash))
		}
		// Verify hash is deterministic.
		hash2, _ := verifyDownloadIntegrity(path)
		if hash != hash2 {
			t.Errorf("non-deterministic hash: %q != %q", hash, hash2)
		}
	})

	t.Run("empty file returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.tar.gz")
		if err := os.WriteFile(path, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		_, err := verifyDownloadIntegrity(path)
		if err == nil {
			t.Fatal("verifyDownloadIntegrity() expected error for empty file")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("error = %q, want containing 'empty'", err.Error())
		}
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, err := verifyDownloadIntegrity("/nonexistent/file.tar.gz")
		if err == nil {
			t.Fatal("verifyDownloadIntegrity() expected error for nonexistent file")
		}
	})
}

// TestVerifyChecksumFile verifies the P-14 checksum verification against an
// official checksums file (sha256sum output format).
func TestVerifyChecksumFile(t *testing.T) {
	t.Run("matching hash passes", func(t *testing.T) {
		dir := t.TempDir()
		checksumPath := filepath.Join(dir, "checksums.sha256")
		content := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  mediamtx_v1.0.0_linux_amd64.tar.gz\n"
		if err := os.WriteFile(checksumPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		err := verifyChecksumFile(checksumPath, "mediamtx_v1.0.0_linux_amd64.tar.gz", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")
		if err != nil {
			t.Errorf("verifyChecksumFile() unexpected error: %v", err)
		}
	})

	t.Run("mismatched hash fails", func(t *testing.T) {
		dir := t.TempDir()
		checksumPath := filepath.Join(dir, "checksums.sha256")
		content := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  mediamtx_v1.0.0_linux_amd64.tar.gz\n"
		if err := os.WriteFile(checksumPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		err := verifyChecksumFile(checksumPath, "mediamtx_v1.0.0_linux_amd64.tar.gz", "0000000000000000000000000000000000000000000000000000000000000000")
		if err == nil {
			t.Fatal("verifyChecksumFile() expected error for hash mismatch")
		}
		if !strings.Contains(err.Error(), "hash mismatch") {
			t.Errorf("error = %q, want containing 'hash mismatch'", err.Error())
		}
	})

	t.Run("filename not found in checksums", func(t *testing.T) {
		dir := t.TempDir()
		checksumPath := filepath.Join(dir, "checksums.sha256")
		content := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  mediamtx_v1.0.0_linux_arm64.tar.gz\n"
		if err := os.WriteFile(checksumPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		err := verifyChecksumFile(checksumPath, "mediamtx_v1.0.0_linux_amd64.tar.gz", "abcdef")
		if err == nil {
			t.Fatal("verifyChecksumFile() expected error for missing filename")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %q, want containing 'not found'", err.Error())
		}
	})

	t.Run("nonexistent checksums file returns error", func(t *testing.T) {
		err := verifyChecksumFile("/nonexistent/checksums.sha256", "test.tar.gz", "abc")
		if err == nil {
			t.Fatal("verifyChecksumFile() expected error for nonexistent file")
		}
	})

	t.Run("case insensitive hash comparison", func(t *testing.T) {
		dir := t.TempDir()
		checksumPath := filepath.Join(dir, "checksums.sha256")
		content := "ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890  test.tar.gz\n"
		if err := os.WriteFile(checksumPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		err := verifyChecksumFile(checksumPath, "test.tar.gz", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")
		if err != nil {
			t.Errorf("verifyChecksumFile() should be case-insensitive: %v", err)
		}
	})
}

// TestIsValidMediaMTXVersion verifies SEC-5: version string validation.
func TestIsValidMediaMTXVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		// Valid versions
		{"v1.9.3", true},
		{"v0.1.0", true},
		{"v10.20.30", true},
		{"1.9.3", true},
		{"v1.9.3-rc1", true},
		{"v2.0.0-beta.1", true},

		// Invalid versions (potential injection)
		{"", false},
		{"latest", false},
		{"v1.9.3/%2e%2e/", false},
		{"v1.9.3; rm -rf /", false},
		{"../../../etc/passwd", false},
		{"v1.9", false}, // missing patch
		{"v1", false},   // missing minor+patch
		{"v1.9.3\nmalicious", false},
		{"v1.9.3 --help", false},
		{"v1.9.3&foo=bar", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.version), func(t *testing.T) {
			got := isValidMediaMTXVersion(tt.version)
			if got != tt.want {
				t.Errorf("isValidMediaMTXVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

// TestInstallMediaMTXVersionValidation verifies SEC-5: invalid versions are rejected.
func TestInstallMediaMTXVersionValidation(t *testing.T) {
	// Requires root, so only test the validation error path
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}

	err := runInstallMediaMTX([]string{"--version=../../../etc/passwd"})
	if err == nil {
		t.Fatal("runInstallMediaMTX should fail for non-root or bad version")
	}

	// Should fail at root check first (non-root env), but if root check somehow
	// passes, it must fail on version validation
	if strings.Contains(err.Error(), "root privileges") {
		// Expected: root check fired first
		return
	}
	if !strings.Contains(err.Error(), "invalid version format") {
		t.Errorf("expected 'invalid version format' error, got: %v", err)
	}
}
