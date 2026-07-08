// SPDX-License-Identifier: MIT

package updater

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// parseSemver parses a version string into major, minor, patch components
// and an optional pre-release suffix. Returns ok=false if the string cannot
// be parsed as a valid semver-like version.
//
// Accepted formats: "1.2.3", "v1.2.3", "1.2.3-rc1", "1.2", "1"
func parseSemver(version string) (major, minor, patch int, prerelease string, ok bool) {
	// Strip 'v' prefix
	version = strings.TrimPrefix(version, "v")

	if version == "" {
		return 0, 0, 0, "", false
	}

	// Split off pre-release suffix (e.g., "1.0.0-rc1" -> "1.0.0", "rc1")
	versionCore := version
	if idx := strings.IndexByte(version, '-'); idx >= 0 {
		prerelease = version[idx+1:]
		versionCore = version[:idx]
	}

	parts := strings.Split(versionCore, ".")

	// Parse major (required)
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, "", false
	}
	major = maj

	// Parse minor (optional, defaults to 0)
	if len(parts) >= 2 {
		min, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, "", false
		}
		minor = min
	}

	// Parse patch (optional, defaults to 0)
	if len(parts) >= 3 {
		pat, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, "", false
		}
		patch = pat
	}

	return major, minor, patch, prerelease, true
}

// isNewerVersion compares two version strings.
// Returns true if latest is newer than current.
// Properly handles multi-digit version components (e.g., 0.10.0 > 0.9.0)
// and pre-release suffixes (e.g., 1.0.0 > 1.0.0-rc1).
func isNewerVersion(latest, current string) bool {
	// Handle "dev" / "unknown" current version -- always offer update
	if current == "dev" || current == "unknown" {
		return true
	}

	// Parse both versions
	lMajor, lMinor, lPatch, lPre, lOk := parseSemver(latest)
	cMajor, cMinor, cPatch, cPre, cOk := parseSemver(current)

	// If latest cannot be parsed as semver, it is not a valid newer version
	if !lOk {
		return false
	}

	// If current cannot be parsed, treat as unknown and offer update
	if !cOk {
		return true
	}

	// Compare major.minor.patch numerically
	if lMajor != cMajor {
		return lMajor > cMajor
	}
	if lMinor != cMinor {
		return lMinor > cMinor
	}
	if lPatch != cPatch {
		return lPatch > cPatch
	}

	// Same major.minor.patch: handle pre-release comparison.
	// A release version (no pre-release) is considered newer than
	// a pre-release version with the same numeric components.
	// e.g., 1.0.0 > 1.0.0-rc1
	if lPre == "" && cPre != "" {
		return true // release is newer than pre-release
	}
	if lPre != "" && cPre == "" {
		return false // pre-release is not newer than release
	}

	// Both have pre-release or both don't: same version, not newer
	return false
}

// getAssetName returns the expected release asset base name for the platform
// this binary runs on, e.g. "lyrebird-linux-amd64" or "lyrebird-linux-armv7".
//
// The name mirrors what the build produces (see Makefile and
// .github/workflows/ci.yml): the architecture component is GOARCH, except
// 32-bit ARM (GOARCH=arm) which carries its GOARM level as a "vN" suffix so the
// separately built ARMv6 and ARMv7 binaries stay distinguishable.
func getAssetName() string {
	return fmt.Sprintf("lyrebird-%s-%s", runtime.GOOS, archComponent(runtime.GOARCH, detectGOARM()))
}

// archComponent returns the architecture component of a release asset name for
// the given GOARCH/GOARM pair.
//
// For 32-bit ARM (goarch == "arm") the GOARM level is appended as "vN"
// (e.g. "armv7"). Releases ship separate ARMv6 and ARMv7 binaries, and a bare
// "arm" is an ambiguous prefix of "arm64" — the exact confusion that made
// ARMv6/v7 devices such as the Raspberry Pi download the 64-bit binary and die
// at exec with "Exec format error". When the GOARM level is unknown it falls
// back to "6": an ARMv7 CPU can execute an ARMv6 binary, but not the reverse,
// so v6 is the safe default.
func archComponent(goarch, goarm string) string {
	if goarch == "arm" {
		if goarm == "" {
			goarm = "6"
		}
		return "armv" + goarm
	}
	return goarch
}

// detectGOARM returns the numeric GOARM level ("5", "6", "7") recorded in this
// binary's build information, or "" when it is not recorded. It is only
// meaningful for GOARCH=arm builds.
func detectGOARM() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return goarmFromSettings(info.Settings)
}

// goarmFromSettings returns the numeric GOARM level from a set of build
// settings, or "" when GOARM is absent. The recorded value may carry a
// float-mode suffix (e.g. "7,hardfloat"); only the leading numeric level is
// returned.
func goarmFromSettings(settings []debug.BuildSetting) string {
	for _, s := range settings {
		if s.Key != "GOARM" {
			continue
		}
		v := s.Value
		if i := strings.IndexByte(v, ','); i >= 0 {
			v = v[:i]
		}
		return v
	}
	return ""
}

// knownArchiveExtensions lists the archive suffixes stripped from a release
// asset file name to recover its platform base name (see assetBaseName). It is
// kept in sync with the archive handling in Update.
var knownArchiveExtensions = []string{".tar.gz", ".tgz"}

// assetBaseName strips a single known archive extension from a release asset
// file name, e.g. "lyrebird-linux-arm64.tar.gz" -> "lyrebird-linux-arm64". A
// name without a known archive extension is returned unchanged.
func assetBaseName(assetFileName string) string {
	for _, ext := range knownArchiveExtensions {
		if strings.HasSuffix(assetFileName, ext) {
			return strings.TrimSuffix(assetFileName, ext)
		}
	}
	return assetFileName
}

// selectAsset returns the download URL and file name of the release asset whose
// base name (file name minus a known archive extension) equals platformName,
// or empty strings when none matches.
//
// The comparison is exact equality, never substring containment: the arm64
// asset "lyrebird-linux-arm64" contains the shorter 32-bit name
// "lyrebird-linux-arm" as a prefix, so a substring match served the 64-bit
// binary to ARMv6/v7 hardware (Raspberry Pi), which fails at exec with
// "Exec format error".
func selectAsset(assets []Asset, platformName string) (downloadURL, assetName string) {
	for _, asset := range assets {
		if assetBaseName(asset.Name) == platformName {
			return asset.BrowserDownloadURL, asset.Name
		}
	}
	return "", ""
}

// FormatReleaseInfo formats release information for display.
func FormatReleaseInfo(release *Release) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Version: %s\n", release.TagName))
	sb.WriteString(fmt.Sprintf("Published: %s\n", release.PublishedAt.Format("2006-01-02 15:04")))

	if release.Name != "" && release.Name != release.TagName {
		sb.WriteString(fmt.Sprintf("Name: %s\n", release.Name))
	}

	if release.Prerelease {
		sb.WriteString("Type: Pre-release\n")
	}

	if len(release.Assets) > 0 {
		sb.WriteString(fmt.Sprintf("Assets: %d available\n", len(release.Assets)))
	}

	if release.Body != "" {
		sb.WriteString("\nRelease Notes:\n")
		sb.WriteString(release.Body)
	}

	return sb.String()
}

// FormatUpdateInfo formats update information for display.
func FormatUpdateInfo(info *UpdateInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Current version: %s\n", info.CurrentVersion))
	sb.WriteString(fmt.Sprintf("Latest version:  %s\n", info.LatestVersion))

	if info.UpdateAvailable {
		sb.WriteString("\n✓ Update available!\n")
		if !info.PublishedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("Published: %s\n", info.PublishedAt.Format("2006-01-02")))
		}
	} else {
		sb.WriteString("\n✓ You are running the latest version.\n")
	}

	return sb.String()
}

// ParseChecksumFile parses a sha256sums-style checksum file and returns the
// hex-encoded SHA256 hash for the named asset.
//
// The expected format is one entry per line:
//
//	<sha256hex>  <filename>
//
// Both one-space and two-space separators (GNU coreutils format) are accepted.
// Returns an error if the asset is not found.
func ParseChecksumFile(content, assetName string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Fields: <hash>  <filename>  (two spaces) or <hash> <filename>
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		// The filename may have a leading '*' (binary mode marker from sha256sum -b)
		name := strings.TrimPrefix(parts[1], "*")
		// Match by basename to be robust against path prefixes
		if filepath.Base(name) == filepath.Base(assetName) {
			// Validate that it looks like a hex SHA256 (64 chars)
			if len(hash) != 64 {
				return "", fmt.Errorf("invalid SHA256 hash length %d for %s", len(hash), assetName)
			}
			return strings.ToLower(hash), nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %q in checksums file", assetName)
}
