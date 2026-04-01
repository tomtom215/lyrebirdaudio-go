// SPDX-License-Identifier: MIT

package updater

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestParseSemverInvalidPatch covers the strconv.Atoi error in parseSemver when
// the patch component is not a valid integer.
func TestParseSemverInvalidPatch(t *testing.T) {
	_, _, _, _, ok := parseSemver("1.2.x")
	if ok {
		t.Error("expected ok=false for version with non-numeric patch, got true")
	}
}

// TestIsNewerVersionCurrentUnparseable covers the cOk=false branch in
// isNewerVersion when the current version cannot be parsed as semver.
func TestIsNewerVersionCurrentUnparseable(t *testing.T) {
	// latest is valid, current is not parseable as semver → should return true.
	result := isNewerVersion("1.0.0", "not-a-version")
	if !result {
		t.Error("expected isNewerVersion=true when current is unparseable, got false")
	}
}

// TestExtractBinaryFromTarGzDestIsDir covers the os.Create error path in
// extractBinaryFromTarGz. When the destDir already contains a directory named
// "lyrebird", os.Create fails (is a directory), triggering the error return.
func TestExtractBinaryFromTarGzDestIsDir(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "release.tar.gz")

	// Create a valid tar.gz that contains a "lyrebird" binary entry.
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	content := []byte("fake binary content")
	hdr := &tar.Header{
		Name:     "lyrebird",
		Mode:     0755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file Close: %v", err)
	}

	// Place a directory at destDir/lyrebird so os.Create fails.
	destDir := t.TempDir()
	binaryDir := filepath.Join(destDir, "lyrebird")
	if err := os.Mkdir(binaryDir, 0750); err != nil {
		t.Fatalf("Mkdir binary: %v", err)
	}

	_, err = extractBinaryFromTarGz(archivePath, destDir)
	if err == nil {
		t.Error("expected error when destPath is a directory, got nil")
	}
}

// TestParseSemverInvalidMinor covers the strconv.Atoi error for the minor
// component in parseSemver.
func TestParseSemverInvalidMinor(t *testing.T) {
	_, _, _, _, ok := parseSemver("1.x.0")
	if ok {
		t.Error("expected ok=false for version with non-numeric minor, got true")
	}
}
