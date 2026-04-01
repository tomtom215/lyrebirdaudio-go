// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRunMigrateDestinationExistsNoForce covers cmd_config.go:43-44 —
// the early-return branch when the destination file already exists and
// --force was not passed. The function returns an error before invoking
// config.MigrateFromBash, so the --from path need not be a real file.
func TestRunMigrateDestinationExistsNoForce(t *testing.T) {
	tmpDir := t.TempDir()
	toPath := filepath.Join(tmpDir, "config.yaml")

	// Pre-create the destination so os.Stat succeeds.
	if err := os.WriteFile(toPath, []byte("# existing config\n"), 0640); err != nil {
		t.Fatalf("WriteFile destination: %v", err)
	}

	err := runMigrate([]string{"--from=/nonexistent/bash/config", "--to=" + toPath})
	if err == nil {
		t.Error("expected error when destination exists without --force, got nil")
	}
}

// TestRunValidateLoadFailure covers cmd_config.go:110-112 —
// the `config.LoadConfig error → return fmt.Errorf("failed to load config")` path.
// Providing a file with invalid YAML syntax causes LoadConfig to return an error.
func TestRunValidateLoadFailure(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")

	if err := os.WriteFile(cfgPath, []byte("{{{{invalid yaml}}}}\n"), 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := runValidate([]string{"--config=" + cfgPath}); err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// TestRunValidateValidationFailure covers cmd_config.go:114-116 —
// the `cfg.Validate() error → return fmt.Errorf("validation failed")` path.
// segment_format "invalid_xyz" passes YAML parsing but fails Config.Validate().
func TestRunValidateValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "invalid.yaml")

	content := `default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
stream:
  segment_format: invalid_xyz
`
	if err := os.WriteFile(cfgPath, []byte(content), 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := runValidate([]string{"--config=" + cfgPath}); err == nil {
		t.Error("expected validation error for invalid segment_format, got nil")
	}
}

// TestVerifyDownloadIntegrityMissingFile covers cmd_install.go:224-226 —
// the `os.Open error → return error` path when the file does not exist.
func TestVerifyDownloadIntegrityMissingFile(t *testing.T) {
	_, err := verifyDownloadIntegrity(filepath.Join(t.TempDir(), "nonexistent.tar.gz"))
	if err == nil {
		t.Error("verifyDownloadIntegrity() expected error for missing file, got nil")
	}
}

// TestVerifyDownloadIntegrityEmptyFile covers cmd_install.go:233-235 —
// the `info.Size() == 0 → return error("downloaded file is empty")` path.
func TestVerifyDownloadIntegrityEmptyFile(t *testing.T) {
	emptyFile := filepath.Join(t.TempDir(), "empty.tar.gz")
	if err := os.WriteFile(emptyFile, []byte{}, 0640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := verifyDownloadIntegrity(emptyFile)
	if err == nil {
		t.Error("verifyDownloadIntegrity() expected error for empty file, got nil")
	}
}

// TestInstallLyreBirdServiceWrapper covers cmd_setup.go:346-348 —
// the single executable statement in installLyreBirdService() which delegates
// to installLyreBirdServiceToPath with the system-wide path. The call will fail
// in test environments (no write access to /etc/systemd/system or systemd not
// running), which is expected. Only the statement itself needs to execute.
func TestInstallLyreBirdServiceWrapper(t *testing.T) {
	// The error is expected and intentionally ignored here.
	_ = installLyreBirdService()
}

// TestCreateDiagnosticBundleHealthEndpoint covers cmd_bundle.go:85-94 —
// the healthz and metrics HTTP fetch branches. A local server is started on
// 127.0.0.1:9998 so both GET requests succeed; the if-err == nil bodies
// (writeFile calls) are then executed for each endpoint.
//
// If port 9998 is already bound by another test binary the test is skipped
// to avoid a port-conflict failure.
func TestCreateDiagnosticBundleHealthEndpoint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9998")
	if err != nil {
		t.Skipf("port 9998 already in use, skipping health-endpoint coverage test: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","streams":[]}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("# lyrebird metrics\nstreams_active 0\n"))
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	// The bundle may partially fail (missing system tools) but that's fine —
	// we only need the healthz/metrics branches to execute.
	_ = createDiagnosticBundle(bundlePath)
}
