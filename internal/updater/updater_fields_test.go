package updater

import (
	"testing"
	"time"
)

func TestAssetFields(t *testing.T) {
	asset := Asset{
		Name:               "lyrebird-linux-amd64.tar.gz",
		Size:               1024,
		BrowserDownloadURL: "https://example.com/download",
		ContentType:        "application/gzip",
	}

	if asset.Name != "lyrebird-linux-amd64.tar.gz" {
		t.Error("Asset Name mismatch")
	}
	if asset.Size != 1024 {
		t.Error("Asset Size mismatch")
	}
}

func TestReleaseFields(t *testing.T) {
	release := Release{
		TagName:    "v1.0.0",
		Name:       "Release 1.0.0",
		Draft:      false,
		Prerelease: false,
		Body:       "Release notes",
		HTMLURL:    "https://github.com/test/repo/releases/v1.0.0",
	}

	if release.TagName != "v1.0.0" {
		t.Error("Release TagName mismatch")
	}
	if release.HTMLURL != "https://github.com/test/repo/releases/v1.0.0" {
		t.Error("Release HTMLURL mismatch")
	}
}

func TestUpdateInfoFields(t *testing.T) {
	info := UpdateInfo{
		CurrentVersion:  "v1.0.0",
		LatestVersion:   "v1.1.0",
		UpdateAvailable: true,
		ReleaseNotes:    "Bug fixes",
		DownloadURL:     "https://example.com/download",
		AssetName:       "lyrebird-linux-amd64.tar.gz",
		PublishedAt:     time.Now(),
	}

	if info.CurrentVersion != "v1.0.0" {
		t.Error("UpdateInfo CurrentVersion mismatch")
	}
	if !info.UpdateAvailable {
		t.Error("UpdateInfo UpdateAvailable should be true")
	}
}
