package updater

import (
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		// Basic comparisons
		{"patch bump", "v1.1.0", "v1.0.0", true},
		{"same version", "v1.0.0", "v1.0.0", false},
		{"current is newer", "v1.0.0", "v1.1.0", false},
		{"no v prefix", "1.1.0", "1.0.0", true},
		{"major version bump", "v2.0.0", "v1.9.9", true},

		// Special versions
		{"dev version always updates", "v1.0.0", "dev", true},
		{"unknown version always updates", "v1.0.0", "unknown", true},

		// CRITICAL: Multi-digit component comparisons (string comparison bug)
		{"0.10.0 is newer than 0.9.0", "0.10.0", "0.9.0", true},
		{"0.9.0 is NOT newer than 0.10.0", "0.9.0", "0.10.0", false},
		{"1.0.0 is newer than 0.99.99", "1.0.0", "0.99.99", true},
		{"2.0.0 is newer than 1.99.99", "2.0.0", "1.99.99", true},
		{"1.10.0 vs 1.9.0", "v1.10.0", "v1.9.0", true},
		{"1.9.0 vs 1.10.0", "v1.9.0", "v1.10.0", false},
		{"1.0.10 vs 1.0.9", "v1.0.10", "v1.0.9", true},
		{"1.0.9 vs 1.0.10", "v1.0.9", "v1.0.10", false},

		// Equal versions (not newer)
		{"equal versions no prefix", "1.0.0", "1.0.0", false},
		{"equal versions with prefix", "v2.5.3", "v2.5.3", false},
		{"equal versions mixed prefix", "v1.0.0", "1.0.0", false},

		// Pre-release suffixes
		{"pre-release vs release", "v1.0.0-rc1", "v1.0.0", false},
		{"release vs pre-release", "v1.0.0", "v0.9.0-rc1", true},
		{"pre-release newer major", "v2.0.0-beta1", "v1.0.0", true},
		{"pre-release same base older current", "v1.1.0-alpha", "v1.0.0", true},

		// Edge cases
		{"latest is dev", "dev", "v1.0.0", false},
		{"both dev", "dev", "dev", true},
		{"latest is unknown", "unknown", "v1.0.0", false},
		{"non-semver latest", "notaversion", "v1.0.0", false},
		{"single component", "2", "1", true},
		{"two components", "1.1", "1.0", true},
		{"large version numbers", "v100.200.300", "v99.199.299", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v",
					tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMajor int
		wantMinor int
		wantPatch int
		wantPre   string
		wantOk    bool
	}{
		{"basic version", "1.2.3", 1, 2, 3, "", true},
		{"with v prefix", "v1.2.3", 1, 2, 3, "", true},
		{"with prerelease", "1.0.0-rc1", 1, 0, 0, "rc1", true},
		{"with v and prerelease", "v2.0.0-beta.1", 2, 0, 0, "beta.1", true},
		{"two components", "1.2", 1, 2, 0, "", true},
		{"single component", "3", 3, 0, 0, "", true},
		{"large numbers", "100.200.300", 100, 200, 300, "", true},
		{"zero version", "0.0.0", 0, 0, 0, "", true},
		{"non-numeric", "abc", 0, 0, 0, "", false},
		{"empty string", "", 0, 0, 0, "", false},
		{"dev", "dev", 0, 0, 0, "", false},
		{"partial non-numeric", "1.abc.3", 0, 0, 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, patch, pre, ok := parseSemver(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseSemver(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if !ok {
				return
			}
			if major != tt.wantMajor {
				t.Errorf("parseSemver(%q) major = %d, want %d", tt.input, major, tt.wantMajor)
			}
			if minor != tt.wantMinor {
				t.Errorf("parseSemver(%q) minor = %d, want %d", tt.input, minor, tt.wantMinor)
			}
			if patch != tt.wantPatch {
				t.Errorf("parseSemver(%q) patch = %d, want %d", tt.input, patch, tt.wantPatch)
			}
			if pre != tt.wantPre {
				t.Errorf("parseSemver(%q) pre = %q, want %q", tt.input, pre, tt.wantPre)
			}
		})
	}
}

func TestGetAssetName(t *testing.T) {
	name := getAssetName()
	if name == "" {
		t.Error("getAssetName() returned empty string")
	}

	// Should contain os and arch
	if !containsString(name, "lyrebird-") {
		t.Errorf("Asset name should start with 'lyrebird-', got %q", name)
	}
}

func TestConstants(t *testing.T) {
	if DefaultOwner == "" {
		t.Error("DefaultOwner should not be empty")
	}
	if DefaultRepo == "" {
		t.Error("DefaultRepo should not be empty")
	}
	if DefaultTimeout <= 0 {
		t.Error("DefaultTimeout should be positive")
	}
	if GitHubAPIURL == "" {
		t.Error("GitHubAPIURL should not be empty")
	}
}
