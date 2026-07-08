// SPDX-License-Identifier: MIT

package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractBinaryFromTarGz extracts the binary from a tar.gz archive.
func extractBinaryFromTarGz(archivePath, destDir string) (string, error) {
	// #nosec G304 -- archivePath is from controlled temp directory
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)

	// Maximum binary size: 100MB (protection against decompression bombs)
	const maxBinarySize = 100 * 1024 * 1024

	var binaryPath string
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}

		// Look for the main binary
		name := filepath.Base(header.Name)
		if name == "lyrebird" || name == "lyrebird-stream" {
			destPath := filepath.Join(destDir, name)
			outFile, err := os.Create(destPath) // #nosec G304 G703 -- destPath is constructed from controlled destDir
			if err != nil {
				return "", err
			}

			// Limit copy size to prevent decompression bombs
			limitReader := io.LimitReader(tarReader, maxBinarySize)
			if _, err := io.Copy(outFile, limitReader); err != nil {
				_ = outFile.Close()
				return "", err
			}
			if err := outFile.Close(); err != nil {
				return "", err
			}

			if name == "lyrebird" {
				binaryPath = destPath
			}
		}
	}

	if binaryPath == "" {
		return "", fmt.Errorf("binary not found in archive")
	}

	return binaryPath, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	// #nosec G304 -- src is from controlled paths (binary backup/restore)
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	// Get source file info for permissions
	info, err := source.Stat()
	if err != nil {
		return err
	}

	dest, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode()) // #nosec G304 G703 -- dst is from controlled paths (binary backup/restore)
	if err != nil {
		return err
	}
	defer func() { _ = dest.Close() }()

	_, err = io.Copy(dest, source)
	return err
}

// verifyChecksumFromURL downloads checksums.txt and verifies the downloaded
// asset matches the expected SHA256 hash.
func (u *Updater) verifyChecksumFromURL(ctx context.Context, checksumURL, assetName, downloadedPath string) error {
	// Download checksums.txt to a temp file
	tmpFile, err := os.CreateTemp("", "lyrebird-checksums-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file for checksums: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := u.Download(ctx, checksumURL, tmpPath, nil); err != nil {
		return fmt.Errorf("failed to download checksums.txt: %w", err)
	}

	// Read checksums file
	// #nosec G304 -- path is a controlled temp file created above
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to read checksums file: %w", err)
	}

	// Parse expected checksum for this asset
	expectedHash, err := ParseChecksumFile(string(data), assetName)
	if err != nil {
		return fmt.Errorf("checksum not found for %s: %w", assetName, err)
	}

	// Compute actual SHA256 of the downloaded file
	// #nosec G304 -- downloadedPath is a controlled temp path
	fileData, err := os.ReadFile(downloadedPath)
	if err != nil {
		return fmt.Errorf("failed to read downloaded file: %w", err)
	}
	sum := sha256.Sum256(fileData)
	actualHash := hex.EncodeToString(sum[:])

	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, expectedHash, actualHash)
	}

	return nil
}

// progressReader wraps a reader to report progress.
type progressReader struct {
	reader     io.ReadCloser
	onProgress func(int64)
}

func (r *progressReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 && r.onProgress != nil {
		r.onProgress(int64(n))
	}
	return
}

func (r *progressReader) Close() error {
	return r.reader.Close()
}
