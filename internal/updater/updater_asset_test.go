// SPDX-License-Identifier: MIT

package updater

import (
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// TestSelectAsset verifies that release assets are matched by exact base name
// (file name minus a known archive extension) and never by substring. The
// headline case is the reported defect: a 32-bit ARM platform must never select
// the arm64 asset even though "lyrebird-linux-arm64" contains
// "lyrebird-linux-arm" as a prefix.
func TestSelectAsset(t *testing.T) {
	// Assets named the way the build actually publishes them: distinct 32-bit
	// ARM variants (armv6/armv7) alongside arm64.
	realAssets := []Asset{
		{Name: "lyrebird-linux-amd64.tar.gz", BrowserDownloadURL: "u-amd64"},
		{Name: "lyrebird-linux-arm64.tar.gz", BrowserDownloadURL: "u-arm64"},
		{Name: "lyrebird-linux-armv7.tar.gz", BrowserDownloadURL: "u-armv7"},
		{Name: "lyrebird-linux-armv6.tar.gz", BrowserDownloadURL: "u-armv6"},
		{Name: "checksums.txt", BrowserDownloadURL: "u-sums"},
	}

	// Minimal release that ships a bare "arm" asset next to "arm64", matching
	// the scenario in the defect report.
	armAndArm64 := []Asset{
		{Name: "lyrebird-linux-arm.tar.gz", BrowserDownloadURL: "u-arm"},
		{Name: "lyrebird-linux-arm64.tar.gz", BrowserDownloadURL: "u-arm64"},
	}

	tests := []struct {
		name      string
		assets    []Asset
		platform  string
		wantURL   string
		wantAsset string
	}{
		{
			name:      "arm must not match arm64 (the reported bug)",
			assets:    armAndArm64,
			platform:  "lyrebird-linux-arm",
			wantURL:   "u-arm",
			wantAsset: "lyrebird-linux-arm.tar.gz",
		},
		{
			name:      "arm64 selects arm64 when both present",
			assets:    armAndArm64,
			platform:  "lyrebird-linux-arm64",
			wantURL:   "u-arm64",
			wantAsset: "lyrebird-linux-arm64.tar.gz",
		},
		{
			name:      "armv7 selects armv7 not arm64",
			assets:    realAssets,
			platform:  "lyrebird-linux-armv7",
			wantURL:   "u-armv7",
			wantAsset: "lyrebird-linux-armv7.tar.gz",
		},
		{
			name:      "armv6 selects armv6 not armv7 or arm64",
			assets:    realAssets,
			platform:  "lyrebird-linux-armv6",
			wantURL:   "u-armv6",
			wantAsset: "lyrebird-linux-armv6.tar.gz",
		},
		{
			name:      "amd64 selects amd64",
			assets:    realAssets,
			platform:  "lyrebird-linux-amd64",
			wantURL:   "u-amd64",
			wantAsset: "lyrebird-linux-amd64.tar.gz",
		},
		{
			name:      "tgz extension supported",
			assets:    []Asset{{Name: "lyrebird-linux-amd64.tgz", BrowserDownloadURL: "u"}},
			platform:  "lyrebird-linux-amd64",
			wantURL:   "u",
			wantAsset: "lyrebird-linux-amd64.tgz",
		},
		{
			name:      "raw binary without archive extension",
			assets:    []Asset{{Name: "lyrebird-linux-amd64", BrowserDownloadURL: "u"}},
			platform:  "lyrebird-linux-amd64",
			wantURL:   "u",
			wantAsset: "lyrebird-linux-amd64",
		},
		{
			name:      "no matching asset yields empty",
			assets:    realAssets,
			platform:  "lyrebird-darwin-amd64",
			wantURL:   "",
			wantAsset: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotAsset := selectAsset(tt.assets, tt.platform)
			if gotURL != tt.wantURL || gotAsset != tt.wantAsset {
				t.Errorf("selectAsset(_, %q) = (%q, %q), want (%q, %q)",
					tt.platform, gotURL, gotAsset, tt.wantURL, tt.wantAsset)
			}
		})
	}
}

// TestAssetBaseName verifies a single known archive extension is stripped and
// nothing else is altered.
func TestAssetBaseName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"lyrebird-linux-arm64.tar.gz", "lyrebird-linux-arm64"},
		{"lyrebird-linux-amd64.tgz", "lyrebird-linux-amd64"},
		{"lyrebird-linux-amd64", "lyrebird-linux-amd64"},
		{"checksums.txt", "checksums.txt"},
		{"weird.tar.gz.tar.gz", "weird.tar.gz"}, // only one extension stripped
	}
	for _, tt := range tests {
		if got := assetBaseName(tt.in); got != tt.want {
			t.Errorf("assetBaseName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestArchComponent verifies the architecture component, in particular that
// 32-bit ARM carries a GOARM "vN" suffix and falls back to the most compatible
// v6 when the level is unknown.
func TestArchComponent(t *testing.T) {
	tests := []struct {
		goarch string
		goarm  string
		want   string
	}{
		{"amd64", "", "amd64"},
		{"arm64", "", "arm64"},
		{"386", "", "386"},
		{"arm", "7", "armv7"},
		{"arm", "6", "armv6"},
		{"arm", "5", "armv5"},
		{"arm", "", "armv6"}, // unknown GOARM falls back to compatible v6
	}
	for _, tt := range tests {
		if got := archComponent(tt.goarch, tt.goarm); got != tt.want {
			t.Errorf("archComponent(%q, %q) = %q, want %q", tt.goarch, tt.goarm, got, tt.want)
		}
	}
}

// TestGoarmFromSettings verifies GOARM extraction from build settings,
// including stripping the float-mode suffix and handling absence. This path
// cannot run on a non-ARM test host, so it is exercised directly.
func TestGoarmFromSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"plain level", []debug.BuildSetting{{Key: "GOARM", Value: "7"}}, "7"},
		{"float-mode suffix stripped", []debug.BuildSetting{{Key: "GOARM", Value: "7,hardfloat"}}, "7"},
		{"first-match wins after skips", []debug.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "GOARM", Value: "6"}}, "6"},
		{"absent", []debug.BuildSetting{{Key: "GOOS", Value: "linux"}}, ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := goarmFromSettings(tt.settings); got != tt.want {
				t.Errorf("goarmFromSettings(%v) = %q, want %q", tt.settings, got, tt.want)
			}
		})
	}
}

// TestDetectGOARMHost exercises the build-info read path. On the (non-ARM) test
// host GOARM is not recorded, so the result must be empty or purely numeric.
func TestDetectGOARMHost(t *testing.T) {
	got := detectGOARM()
	for _, r := range got {
		if r < '0' || r > '9' {
			t.Fatalf("detectGOARM() = %q, want empty or numeric", got)
		}
	}
}

// TestGetAssetNameMatchesHostAsset verifies that the name getAssetName produces
// for the running platform round-trips through selectAsset against the asset the
// build would publish for this GOOS/GOARCH. This guards against getAssetName and
// the matcher drifting apart.
func TestGetAssetNameMatchesHostAsset(t *testing.T) {
	name := getAssetName()
	if !strings.HasPrefix(name, "lyrebird-"+runtime.GOOS+"-") {
		t.Fatalf("getAssetName() = %q, want prefix %q", name, "lyrebird-"+runtime.GOOS+"-")
	}
	if runtime.GOARCH == "arm" && !strings.Contains(name, "armv") {
		t.Errorf("getAssetName() = %q on GOARCH=arm, want an armvN component", name)
	}

	assets := []Asset{{Name: name + ".tar.gz", BrowserDownloadURL: "host"}}
	gotURL, gotAsset := selectAsset(assets, name)
	if gotURL != "host" || gotAsset != name+".tar.gz" {
		t.Errorf("selectAsset did not match host asset %q for platform %q; got (%q, %q)",
			name+".tar.gz", name, gotURL, gotAsset)
	}
}
