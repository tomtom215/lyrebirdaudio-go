// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/diagnostics"
)

// createDiagnosticBundle collects diagnostic information into a tar.gz archive
// suitable for remote support engineers (GAP-9 / B-5).
func createDiagnosticBundle(outputPath string) error {
	// Sanitize output path to prevent path traversal.
	outputPath = filepath.Clean(outputPath)
	fmt.Printf("\nCreating diagnostic bundle: %s\n", outputPath)

	// Collect data into a temporary directory.
	tmpDir, err := os.MkdirTemp("", "lyrebird-bundle-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	writeFile := func(name, content string) {
		// #nosec G703 -- name is always a hardcoded filename literal; tmpDir is from os.MkdirTemp
		if err := os.WriteFile(filepath.Join(tmpDir, filepath.Clean(name)), []byte(content), 0600); err != nil {
			fmt.Printf("  warning: failed to write %s: %v\n", name, err)
		}
	}

	runCmd := func(name string, cmdArgs ...string) string {
		// #nosec G204 -- cmdArgs are from hardcoded lists, not user input
		out, err := exec.Command(cmdArgs[0], cmdArgs[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Sprintf("command failed: %v\n%s", err, out)
		}
		return string(out)
	}

	// --- Structured diagnostic report (all 30 checks) ---
	diagOpts := diagnostics.DefaultOptions()
	diagOpts.ConfigPath = defaultConfigPath
	runner := diagnostics.NewRunner(diagOpts)
	if report, diagErr := runner.Run(context.Background()); diagErr == nil {
		if data, jsonErr := json.MarshalIndent(report, "", "  "); jsonErr == nil {
			writeFile("diagnostics.json", string(data))
		}
	}

	// Collect system info.
	writeFile("system_info.txt", runCmd("uname", "uname", "-a"))
	writeFile("os_release.txt", func() string {
		data, _ := os.ReadFile("/etc/os-release")
		return string(data)
	}())
	writeFile("uptime.txt", runCmd("uptime", "uptime"))
	writeFile("dmesg.txt", runCmd("dmesg", "dmesg", "--time-format=iso", "-T"))

	// Collect lyrebird diagnostics.
	writeFile("lyrebird_status.txt", runCmd("lyrebird status", "lyrebird", "status"))
	writeFile("lyrebird_devices.txt", runCmd("lyrebird devices", "lyrebird", "devices"))

	// Collect service logs (last 500 lines).
	writeFile("journalctl.txt", runCmd("journalctl", "journalctl", "-u", "lyrebird-stream", "-n", "500", "--no-pager"))
	writeFile("journalctl_mediamtx.txt", runCmd("journalctl mediamtx", "journalctl", "-u", "mediamtx", "-n", "100", "--no-pager"))

	// Collect config (if exists).
	if data, err := os.ReadFile(defaultConfigPath); err == nil {
		writeFile("config.yaml", string(data))
	}

	// Collect health endpoint response.
	httpClient := &http.Client{Timeout: 5 * time.Second}
	if resp, err := httpClient.Get("http://127.0.0.1:9998/healthz"); err == nil { //#nosec G107 -- localhost only
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		writeFile("healthz.json", string(body))
	}
	if resp, err := httpClient.Get("http://127.0.0.1:9998/metrics"); err == nil { //#nosec G107 -- localhost only
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		writeFile("metrics.txt", string(body))
	}

	// Create tar.gz archive.
	outFile, err := os.Create(outputPath) //#nosec G304 -- outputPath is from CLI argument
	if err != nil {
		return fmt.Errorf("failed to create bundle file: %w", err)
	}
	defer outFile.Close()

	if err := createTarGz(outFile, tmpDir); err != nil {
		return fmt.Errorf("failed to create bundle archive: %w", err)
	}

	fmt.Printf("Bundle created: %s\n", outputPath)
	fmt.Println("Send this file to support for remote analysis.")
	return nil
}

// createTarGz creates a gzip-compressed tar archive of srcDir at outFile.
func createTarGz(outFile *os.File, srcDir string) error {
	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		hdr := &tar.Header{
			Name:    relPath,
			Mode:    0600,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tarWriter.WriteHeader(hdr); err != nil {
			return err
		}

		// #nosec G304,G122 -- path is from filepath.Walk on our own tmpDir
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tarWriter, f)
		return err
	})
}
