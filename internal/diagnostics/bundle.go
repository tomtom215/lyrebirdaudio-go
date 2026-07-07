// SPDX-License-Identifier: MIT

//go:build linux

// bundle.go provides the ExportBundle function that writes a compressed diagnostic
// archive suitable for sharing with field-support engineers.
//
// Archive layout (all within a top-level directory named lyrebird-diag-<timestamp>/):
//
//	diagnostics.json   – full DiagnosticReport serialised as JSON
//	system-info.txt    – human-readable system snapshot (os-release, uname, uptime)
//	logs/              – last 50 lines of each *.log in opts.LogDir (if present)
//
// The archive is a gzip-compressed tar stream written to the destination path.
package diagnostics

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BundleOptions extends Options with settings specific to bundle export.
type BundleOptions struct {
	Options
	// MaxLogLines is the maximum number of trailing lines captured from each log file.
	// Zero or negative means 200.
	MaxLogLines int
}

// ExportBundle runs all diagnostic checks and writes a .tar.gz bundle to dst.
// The file at dst is created with mode 0600 (owner-read/write only).
func ExportBundle(ctx context.Context, opts BundleOptions, dst string) error {
	if opts.MaxLogLines <= 0 {
		opts.MaxLogLines = 200
	}

	// Run diagnostics.
	runner := NewRunner(opts.Options)
	report, err := runner.Run(ctx)
	if err != nil {
		return fmt.Errorf("diagnostics run: %w", err)
	}

	// Serialise the report.
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	// Build the system-info text block.
	sysInfo := buildSystemInfoText(opts.Options)

	// Collect log snippets.
	logSnippets := collectLogSnippets(opts.LogDir, opts.MaxLogLines)

	// Create the destination file.
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600) //#nosec G304 -- dst is caller-supplied output path
	if err != nil {
		return fmt.Errorf("create bundle file: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	// Top-level directory name inside the archive.
	stamp := time.Now().UTC().Format("20060102-150405")
	prefix := "lyrebird-diag-" + stamp + "/"

	// Write diagnostics.json
	if err := writeTarEntry(tw, prefix+"diagnostics.json", reportJSON); err != nil {
		return fmt.Errorf("write diagnostics.json: %w", err)
	}

	// Write system-info.txt
	if err := writeTarEntry(tw, prefix+"system-info.txt", []byte(sysInfo)); err != nil {
		return fmt.Errorf("write system-info.txt: %w", err)
	}

	// Write log snippets
	for name, content := range logSnippets {
		entryPath := prefix + "logs/" + filepath.Base(name)
		if err := writeTarEntry(tw, entryPath, []byte(content)); err != nil {
			return fmt.Errorf("write log %s: %w", name, err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush tar: %w", err)
	}
	if err := gz.Flush(); err != nil {
		return fmt.Errorf("flush gzip: %w", err)
	}
	return nil
}

// writeTarEntry appends a regular file entry to the tar archive.
func writeTarEntry(tw *tar.Writer, path string, data []byte) error {
	hdr := &tar.Header{
		Name:     path,
		Mode:     0644,
		Size:     int64(len(data)),
		ModTime:  time.Now().UTC(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// buildSystemInfoText assembles a brief human-readable system snapshot.
func buildSystemInfoText(opts Options) string {
	var b strings.Builder

	b.WriteString("=== LyreBirdAudio Diagnostic Bundle ===\n")
	b.WriteString("Generated: " + time.Now().UTC().Format(time.RFC3339) + "\n\n")

	// /etc/os-release (best-effort)
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		b.WriteString("--- OS Release ---\n")
		b.WriteString(string(data))
		b.WriteString("\n")
	}

	// /proc/version
	if data, err := os.ReadFile(opts.ProcFS + "/version"); err == nil {
		b.WriteString("--- Kernel ---\n")
		b.WriteString(strings.TrimSpace(string(data)))
		b.WriteString("\n\n")
	}

	// /proc/uptime
	if data, err := os.ReadFile(opts.ProcFS + "/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			b.WriteString("--- Uptime ---\n")
			if secs := parseFloat(fields[0]); secs > 0 {
				b.WriteString(formatDuration(time.Duration(secs) * time.Second))
			} else {
				b.WriteString(fields[0] + "s")
			}
			b.WriteString("\n\n")
		}
	}

	// /proc/meminfo summary (MemTotal + MemAvailable)
	if data, err := os.ReadFile(opts.ProcFS + "/meminfo"); err == nil {
		b.WriteString("--- Memory ---\n")
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") || strings.HasPrefix(line, "MemAvailable:") {
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("--- Config ---\n")
	b.WriteString("ConfigPath: " + opts.ConfigPath + "\n")
	b.WriteString("LogDir:     " + opts.LogDir + "\n")
	b.WriteString("LockDir:    " + opts.LockDir + "\n")

	return b.String()
}

// collectLogSnippets returns the last maxLines lines from each *.log file in logDir.
// Files are read best-effort; errors are silently skipped.
func collectLogSnippets(logDir string, maxLines int) map[string]string {
	result := make(map[string]string)

	if logDir == "" {
		return result
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return result
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		fullPath := filepath.Join(logDir, e.Name())
		content := tailFile(fullPath, maxLines)
		if content != "" {
			result[e.Name()] = content
		}
	}
	return result
}

// tailFile reads the last n lines from path.
func tailFile(path string, n int) string {
	data, err := os.ReadFile(path) //#nosec G304 -- path is derived from ReadDir within logDir
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// parseFloat parses a float64, returning 0 on error.
func parseFloat(s string) float64 {
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	if err != nil {
		return 0
	}
	return v
}

// BundleSize returns the byte size of the bundle file at dst, or an error.
// This is a convenience for post-export reporting.
func BundleSize(dst string) (int64, error) {
	info, err := os.Stat(dst)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// io writer alias for tests.
var _ io.Writer = (*tar.Writer)(nil)
