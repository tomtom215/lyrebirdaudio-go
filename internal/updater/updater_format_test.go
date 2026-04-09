package updater

import (
	"strings"
	"testing"
	"time"
)

func TestFormatReleaseInfo(t *testing.T) {
	release := &Release{
		TagName:     "v1.0.0",
		Name:        "First Release",
		PublishedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Body:        "Initial release with basic features",
		Assets: []Asset{
			{Name: "lyrebird-linux-amd64.tar.gz"},
		},
	}

	info := FormatReleaseInfo(release)

	if !containsString(info, "v1.0.0") {
		t.Error("Info should contain version")
	}
	if !containsString(info, "First Release") {
		t.Error("Info should contain name")
	}
	if !containsString(info, "Initial release") {
		t.Error("Info should contain release notes")
	}
}

func TestFormatReleaseInfoPrerelease(t *testing.T) {
	release := &Release{
		TagName:     "v2.0.0-rc1",
		PublishedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Prerelease:  true,
	}

	info := FormatReleaseInfo(release)

	if !containsString(info, "Pre-release") {
		t.Error("Info should indicate pre-release")
	}
}

func TestFormatReleaseInfoMinimal(t *testing.T) {
	release := &Release{
		TagName:     "v1.0.0",
		PublishedAt: time.Now(),
	}

	info := FormatReleaseInfo(release)

	if !containsString(info, "v1.0.0") {
		t.Error("Info should contain version")
	}
}

func TestFormatUpdateInfo(t *testing.T) {
	info := &UpdateInfo{
		CurrentVersion:  "v1.0.0",
		LatestVersion:   "v1.1.0",
		UpdateAvailable: true,
	}

	output := FormatUpdateInfo(info)

	if !containsString(output, "v1.0.0") {
		t.Error("Output should contain current version")
	}
	if !containsString(output, "v1.1.0") {
		t.Error("Output should contain latest version")
	}
	if !containsString(output, "Update available") {
		t.Error("Output should indicate update available")
	}
}

func TestFormatUpdateInfoNoUpdate(t *testing.T) {
	info := &UpdateInfo{
		CurrentVersion:  "v1.1.0",
		LatestVersion:   "v1.1.0",
		UpdateAvailable: false,
	}

	output := FormatUpdateInfo(info)

	if !containsString(output, "latest version") {
		t.Error("Output should indicate running latest version")
	}
}

// TestFormatUpdateInfoPublishedAt verifies the PublishedAt date is printed
// only when it is a non-zero time (fixes the inverted IsZero bug).
func TestFormatUpdateInfoPublishedAt(t *testing.T) {
	t.Run("non-zero published date is shown", func(t *testing.T) {
		info := &UpdateInfo{
			CurrentVersion:  "v1.0.0",
			LatestVersion:   "v1.1.0",
			UpdateAvailable: true,
			PublishedAt:     time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		}
		output := FormatUpdateInfo(info)
		if !strings.Contains(output, "Published: 2025-06-15") {
			t.Errorf("Expected published date in output, got: %s", output)
		}
	})

	t.Run("zero published date is omitted", func(t *testing.T) {
		info := &UpdateInfo{
			CurrentVersion:  "v1.0.0",
			LatestVersion:   "v1.1.0",
			UpdateAvailable: true,
			PublishedAt:     time.Time{}, // zero value
		}
		output := FormatUpdateInfo(info)
		if strings.Contains(output, "Published:") {
			t.Errorf("Zero PublishedAt should not produce 'Published:' line, got: %s", output)
		}
	})
}
