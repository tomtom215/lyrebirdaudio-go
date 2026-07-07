// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realDefaultConfigSnippet mirrors the top-level shape of the stock MediaMTX
// mediamtx.yml (captured from v1.19.2): api disabled by default, plus other
// top-level keys and a nested "api"-named key that must NOT be touched.
const realDefaultConfigSnippet = `logLevel: info
api: false
apiAddress: :9997
metrics: false
metricsAddress: :9998
paths:
  cam:
    api: false
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mediamtx.yml")
	if err := os.WriteFile(p, []byte(content), 0640); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

func TestEnableMediaMTXAPI(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantChanged bool
		wantLine    string // expected top-level api line after the call
	}{
		{"stock false", "logLevel: info\napi: false\n", true, "api: yes"},
		{"lowercase no", "api: no\n", true, "api: yes"},
		{"already yes", "api: yes\n", false, "api: yes"},
		{"already true", "api: true\n", false, "api: true"},
		{"inline comment", "api: false # control API\n", true, "api: yes"},
		{"commented out key ignored", "#api: false\n", false, ""},
		{"realistic default", realDefaultConfigSnippet, true, "api: yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTemp(t, tt.in)
			changed, err := enableMediaMTXAPI(p)
			if err != nil {
				t.Fatalf("enableMediaMTXAPI() error: %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			data, _ := os.ReadFile(p) //nolint:errcheck // temp file just written
			// The nested "  api: false" under paths.cam must remain untouched.
			if strings.Contains(tt.in, "cam:") && !strings.Contains(string(data), "    api: false") {
				t.Errorf("nested api key was modified:\n%s", data)
			}
			if tt.wantLine != "" {
				foundTopLevel := false
				for _, line := range strings.Split(string(data), "\n") {
					if isTopLevelYAMLKey(line, "api") {
						foundTopLevel = true
						if line != tt.wantLine {
							t.Errorf("top-level api line = %q, want %q", line, tt.wantLine)
						}
					}
				}
				if !foundTopLevel {
					t.Errorf("no top-level api line found in:\n%s", data)
				}
			}
		})
	}
}

func TestMediaMTXAPIEnabled(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"disabled", "api: false\n", false},
		{"enabled yes", "api: yes\n", true},
		{"enabled true", "api: true\n", true},
		{"no key defaults false", "logLevel: info\n", false},
		{"nested only defaults false", "paths:\n  cam:\n    api: true\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTemp(t, tt.in)
			got, err := mediaMTXAPIEnabled(p)
			if err != nil {
				t.Fatalf("mediaMTXAPIEnabled() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("mediaMTXAPIEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnableMediaMTXAPIMissingFile(t *testing.T) {
	if _, err := enableMediaMTXAPI(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Error("expected error for missing file")
	}
}
