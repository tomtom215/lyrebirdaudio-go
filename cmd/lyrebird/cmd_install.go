// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// runInstallMediaMTX installs MediaMTX RTSP server.
func runInstallMediaMTX(args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("install-mediamtx requires root privileges (run with sudo)")
	}

	// Parse flags
	version := "v1.9.3" // Known stable version
	installService := true
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--version="):
			version = strings.TrimPrefix(arg, "--version=")
		case arg == "--no-service":
			installService = false
		}
	}

	// SEC-5: Validate version format to prevent URL path injection.
	// Only allow vX.Y.Z or X.Y.Z format (with optional pre-release suffix).
	if !isValidMediaMTXVersion(version) {
		return fmt.Errorf("invalid version format %q: must be vX.Y.Z (e.g., v1.9.3)", version)
	}

	fmt.Println("MediaMTX Installation")
	fmt.Println("=====================")
	fmt.Println()

	// Detect architecture
	arch := detectArch()
	fmt.Printf("Detected architecture: %s\n", arch)

	if arch == "" {
		return fmt.Errorf("unsupported architecture")
	}

	// Check if already installed
	if existingPath, err := exec.LookPath("mediamtx"); err == nil {
		fmt.Printf("MediaMTX already installed at: %s\n", existingPath)
		fmt.Print("Reinstall? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Installation cancelled.")
			return nil
		}
	}

	// Construct download URL
	downloadURL := fmt.Sprintf(
		"https://github.com/bluenviron/mediamtx/releases/download/%s/mediamtx_%s_linux_%s.tar.gz",
		version, version, arch,
	)

	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Download URL: %s\n", downloadURL)
	fmt.Println()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mediamtx-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tarPath := filepath.Join(tmpDir, "mediamtx.tar.gz")

	// Download using curl or wget
	fmt.Println("Downloading MediaMTX...")
	if err := downloadFile(downloadURL, tarPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// P-14 fix: Verify download integrity against official checksums.
	// MediaMTX publishes checksums.sha256 in each GitHub release.
	hash, err := verifyDownloadIntegrity(tarPath)
	if err != nil {
		return fmt.Errorf("download integrity check failed: %w", err)
	}
	fmt.Printf("Download complete (SHA256: %s)\n", hash)

	// Attempt to verify against official checksums from GitHub release.
	checksumURL := fmt.Sprintf(
		"https://github.com/bluenviron/mediamtx/releases/download/%s/checksums.sha256",
		version,
	)
	checksumPath := filepath.Join(tmpDir, "checksums.sha256")
	archiveFilename := fmt.Sprintf("mediamtx_%s_linux_%s.tar.gz", version, arch)
	if dlErr := downloadFile(checksumURL, checksumPath); dlErr != nil {
		fmt.Printf("Warning: could not download checksums file: %v\n", dlErr)
		fmt.Println("Skipping checksum verification — verify manually if needed.")
	} else if verifyErr := verifyChecksumFile(checksumPath, archiveFilename, hash); verifyErr != nil {
		return fmt.Errorf("checksum verification FAILED: %w", verifyErr)
	} else {
		fmt.Println("Checksum verification passed.")
	}

	// Extract (tar -xzf validates gzip and tar structure — corrupt files fail here)
	fmt.Println("Extracting...")
	extractCmd := exec.Command("tar", "-xzf", tarPath, "-C", tmpDir) // #nosec G204 -- tarPath and tmpDir are controlled
	if output, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extraction failed (archive may be corrupt): %w: %s", err, string(output))
	}

	// Install binary
	binaryPath := filepath.Join(tmpDir, "mediamtx")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("mediamtx binary not found in archive")
	}

	fmt.Println("Installing to /usr/local/bin/mediamtx...")
	installCmd := exec.Command("install", "-m", "755", binaryPath, "/usr/local/bin/mediamtx") // #nosec G204 -- binaryPath is from controlled tmpDir
	if output, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("installation failed: %w: %s", err, string(output))
	}

	// Install config if it doesn't exist
	configSrc := filepath.Join(tmpDir, "mediamtx.yml")
	configDst := "/etc/mediamtx/mediamtx.yml"
	if _, err := os.Stat(configDst); os.IsNotExist(err) {
		fmt.Printf("Installing default config to %s...\n", configDst)
		if err := os.MkdirAll("/etc/mediamtx", 0750); err != nil { // #nosec G301 -- config dir needs to be readable
			fmt.Printf("Warning: failed to create config directory: %v\n", err)
		} else if _, err := os.Stat(configSrc); err == nil {
			copyCmd := exec.Command("cp", configSrc, configDst) // #nosec G204 -- paths are from controlled tmpDir
			if output, err := copyCmd.CombinedOutput(); err != nil {
				fmt.Printf("Warning: failed to copy config: %v: %s\n", err, string(output))
			}
		}
	} else {
		fmt.Printf("Config already exists at %s, keeping existing.\n", configDst)
	}

	// Install systemd service
	if installService {
		fmt.Println("Installing systemd service...")
		if err := installMediaMTXService(); err != nil {
			fmt.Printf("Warning: failed to install systemd service: %v\n", err)
			fmt.Println("You can start MediaMTX manually with: mediamtx")
		} else {
			fmt.Println("Systemd service installed.")
			fmt.Println("Start with: sudo systemctl start mediamtx")
			fmt.Println("Enable on boot: sudo systemctl enable mediamtx")
		}
	}

	fmt.Println()
	fmt.Println("MediaMTX installation complete!")
	fmt.Println()
	fmt.Println("Default RTSP URL: rtsp://localhost:8554")
	fmt.Println("API URL: http://localhost:9997")

	return nil
}

// detectArch returns the MediaMTX architecture string for the current system.
func detectArch() string {
	cmd := exec.Command("uname", "-m")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	machine := strings.TrimSpace(string(output))
	switch machine {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "armhf":
		return "armv7"
	case "armv6l":
		return "armv6"
	default:
		return ""
	}
}

// downloadFile downloads a file from URL to destination path.
func downloadFile(url, dest string) error {
	// Try curl first
	if _, err := exec.LookPath("curl"); err == nil {
		cmd := exec.Command("curl", "-fsSL", "-o", dest, url) // #nosec G204 G702 -- "curl" is a literal, url/dest are from config, not web input
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("curl failed: %w: %s", err, string(output))
		}
		return nil
	}

	// Fall back to wget
	if _, err := exec.LookPath("wget"); err == nil {
		cmd := exec.Command("wget", "-q", "-O", dest, url) // #nosec G204 G702 -- "wget" is a literal, url/dest are from config, not web input
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wget failed: %w: %s", err, string(output))
		}
		return nil
	}

	return fmt.Errorf("neither curl nor wget found - install one of them first")
}

// verifyDownloadIntegrity checks that a downloaded file exists, is non-empty,
// and computes its SHA256 hash. The hash is returned for operator verification
// against the official release checksums (P-14 fix).
func verifyDownloadIntegrity(path string) (string, error) {
	// #nosec G304 -- path is from controlled temp directory
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open downloaded file: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("cannot stat downloaded file: %w", err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("downloaded file is empty (0 bytes)")
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to compute hash: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyChecksumFile checks a SHA256 hash against an official checksums file.
// The checksums file format is: "<hash>  <filename>\n" (sha256sum output format).
func verifyChecksumFile(checksumPath, filename, actualHash string) error {
	// #nosec G304 -- checksumPath is from controlled temp directory
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("cannot read checksums file: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "hash  filename" or "hash *filename" (binary mode)
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		expectedHash := parts[0]
		checksumFilename := strings.TrimPrefix(parts[1], "*")
		if checksumFilename == filename {
			if !strings.EqualFold(actualHash, expectedHash) {
				return fmt.Errorf("hash mismatch for %s: expected %s, got %s", filename, expectedHash, actualHash)
			}
			return nil
		}
	}

	return fmt.Errorf("filename %q not found in checksums file", filename)
}

// isValidMediaMTXVersion checks that a version string matches the expected
// semver format (vX.Y.Z or X.Y.Z, with optional pre-release suffix like -rc1).
// SEC-5: Prevents URL path injection when constructing download URLs.
var validVersionRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)

func isValidMediaMTXVersion(v string) bool {
	return validVersionRe.MatchString(v)
}

// installMediaMTXService installs the MediaMTX systemd service.
func installMediaMTXService() error {
	serviceContent := `[Unit]
Description=MediaMTX RTSP Server
Documentation=https://github.com/bluenviron/mediamtx
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mediamtx /etc/mediamtx/mediamtx.yml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	servicePath := "/etc/systemd/system/mediamtx.service"
	// #nosec G306 - systemd service files should be world-readable
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	reloadCmd := exec.Command("systemctl", "daemon-reload")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w: %s", err, string(output))
	}

	return nil
}
