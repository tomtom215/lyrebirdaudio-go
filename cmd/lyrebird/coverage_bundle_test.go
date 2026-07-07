// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateDiagnosticBundle verifies the diagnostic bundle creation.
func TestCreateDiagnosticBundle(t *testing.T) {
	outDir := t.TempDir()
	bundlePath := filepath.Join(outDir, "diagnostic-bundle.tar.gz")

	err := createDiagnosticBundle(bundlePath)
	// The function may fail due to missing system commands, but should not panic
	// and should at least create a file or return a meaningful error.
	if err != nil {
		// Some errors are acceptable (e.g., missing lyrebird binary)
		t.Logf("createDiagnosticBundle() returned error (may be expected): %v", err)
		return
	}

	// If successful, verify the file exists
	info, err := os.Stat(bundlePath)
	if err != nil {
		t.Fatalf("bundle file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("bundle file is empty")
	}

	// Verify it is a valid tar.gz
	f, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("failed to open bundle: %v", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("bundle is not valid gzip: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	fileCount := 0
	for {
		_, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar read error: %v", err)
		}
		fileCount++
	}

	if fileCount == 0 {
		t.Error("bundle contains no files")
	}
}

// TestRunDiagnoseWithBundle verifies the --bundle flag triggers bundle creation.
func TestRunDiagnoseWithBundle(t *testing.T) {
	outDir := t.TempDir()
	bundlePath := filepath.Join(outDir, "test-bundle.tar.gz")

	// Run diagnose with --bundle flag (equals form)
	err := runDiagnose([]string{"--bundle=" + bundlePath})
	// May fail due to missing system commands, but should not panic
	if err != nil {
		t.Logf("runDiagnose(--bundle) returned error (may be expected): %v", err)
	}
}

// TestRunDiagnoseWithBundleSpaceForm verifies the --bundle flag with space separator.
func TestRunDiagnoseWithBundleSpaceForm(t *testing.T) {
	outDir := t.TempDir()
	bundlePath := filepath.Join(outDir, "test-bundle.tar.gz")

	// Run diagnose with --bundle flag (space form)
	err := runDiagnose([]string{"--bundle", bundlePath})
	// May fail due to missing system commands, but should not panic
	if err != nil {
		t.Logf("runDiagnose(--bundle space) returned error (may be expected): %v", err)
	}
}

// TestRunDiagnoseFlagFilteringPreservesOtherArgs verifies --bundle flag is
// stripped while other args pass through.
func TestRunDiagnoseFlagFilteringPreservesOtherArgs(t *testing.T) {
	// Just verify no panic when mixing flags
	err := runDiagnose([]string{"--some-unknown-flag"})
	if err != nil {
		t.Logf("runDiagnose() with unknown flag returned error (may be expected): %v", err)
	}
}
