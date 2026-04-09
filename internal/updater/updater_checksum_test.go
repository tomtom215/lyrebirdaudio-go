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

// TestParseChecksumFile covers the M-13 checksum parser.
func TestParseChecksumFile(t *testing.T) {
	goodContent := `# checksums for v1.2.3
abc123def456abc123def456abc123def456abc123def456abc123def456abc1  lyrebird-linux-amd64.tar.gz
fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210  lyrebird-linux-arm64.tar.gz
`

	tests := []struct {
		name      string
		content   string
		assetName string
		wantHash  string
		wantErr   bool
	}{
		{
			name:      "found exact match",
			content:   goodContent,
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
			wantErr:   false,
		},
		{
			name:      "found second asset",
			content:   goodContent,
			assetName: "lyrebird-linux-arm64.tar.gz",
			wantHash:  "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
			wantErr:   false,
		},
		{
			name:      "asset not found",
			content:   goodContent,
			assetName: "lyrebird-windows-amd64.zip",
			wantErr:   true,
		},
		{
			name:      "empty content",
			content:   "",
			assetName: "any.tar.gz",
			wantErr:   true,
		},
		{
			name:      "comment-only content",
			content:   "# just a comment\n",
			assetName: "any.tar.gz",
			wantErr:   true,
		},
		{
			name:      "binary mode asterisk prefix",
			content:   "abc123def456abc123def456abc123def456abc123def456abc123def456abc1 *lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
			wantErr:   false,
		},
		{
			name:      "invalid hash length",
			content:   "tooshort  lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantErr:   true,
		},
		{
			name:      "basename match ignores path prefix",
			content:   "abc123def456abc123def456abc123def456abc123def456abc123def456abc1  ./dist/lyrebird-linux-amd64.tar.gz\n",
			assetName: "lyrebird-linux-amd64.tar.gz",
			wantHash:  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChecksumFile(tt.content, tt.assetName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseChecksumFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantHash {
				t.Errorf("ParseChecksumFile() = %q, want %q", got, tt.wantHash)
			}
		})
	}
}

// TestVerifyChecksumFromURL tests the end-to-end checksum download and verify path.
func TestVerifyChecksumFromURL(t *testing.T) {
	// Create a test asset file with known content
	tmpDir := t.TempDir()
	assetContent := []byte("fake binary content for testing")
	assetPath := filepath.Join(tmpDir, "lyrebird-linux-amd64.tar.gz")
	if err := os.WriteFile(assetPath, assetContent, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Compute the actual SHA256 using the test helper
	goodHash := sha256HexTest(assetContent)
	checksumContent := goodHash + "  lyrebird-linux-amd64.tar.gz\n"

	t.Run("valid checksum", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(checksumContent))
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err != nil {
			t.Errorf("unexpected error for valid checksum: %v", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		wrongContent := strings.Repeat("0", 64) + "  lyrebird-linux-amd64.tar.gz\n"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(wrongContent))
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err == nil {
			t.Error("expected checksum mismatch error, got nil")
		}
		if !strings.Contains(err.Error(), "mismatch") {
			t.Errorf("error should mention mismatch, got: %v", err)
		}
	})

	t.Run("checksums server error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err == nil {
			t.Error("expected error for server error, got nil")
		}
	})

	t.Run("asset not in checksums", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(goodHash + "  other-file.tar.gz\n"))
		}))
		defer ts.Close()

		u := New(WithHTTPClient(ts.Client()))
		err := u.verifyChecksumFromURL(context.Background(), ts.URL+"/checksums.txt",
			"lyrebird-linux-amd64.tar.gz", assetPath)
		if err == nil {
			t.Error("expected error when asset not in checksums, got nil")
		}
	})
}
