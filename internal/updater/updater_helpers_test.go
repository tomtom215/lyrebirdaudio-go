package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// mockTransport redirects requests to a mock server
type mockTransport struct {
	mockServer *httptest.Server
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect GitHub API requests to mock server
	mockURL := t.mockServer.URL + req.URL.Path
	mockReq, err := http.NewRequestWithContext(req.Context(), req.Method, mockURL, req.Body)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Header {
		mockReq.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(mockReq)
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// createTestTarGz creates a test tar.gz archive with the specified files
func createTestTarGz(t *testing.T, archivePath string, files map[string][]byte) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0755,
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("Failed to write content: %v", err)
		}
	}
}

// sha256HexTest computes the hex-encoded SHA256 of data (test helper).
func sha256HexTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
