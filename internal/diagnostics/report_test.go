// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatDurationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "zero duration",
			duration: 0,
			want:     "0m",
		},
		{
			name:     "less than a minute",
			duration: 30 * time.Second,
			want:     "0m",
		},
		{
			name:     "exactly one minute",
			duration: time.Minute,
			want:     "1m",
		},
		{
			name:     "minutes only",
			duration: 45 * time.Minute,
			want:     "45m",
		},
		{
			name:     "exactly one hour",
			duration: time.Hour,
			want:     "1h 0m",
		},
		{
			name:     "hours and minutes",
			duration: 3*time.Hour + 42*time.Minute,
			want:     "3h 42m",
		},
		{
			name:     "exactly one day",
			duration: 24 * time.Hour,
			want:     "1d 0h 0m",
		},
		{
			name:     "days hours minutes",
			duration: 2*24*time.Hour + 5*time.Hour + 17*time.Minute,
			want:     "2d 5h 17m",
		},
		{
			name:     "many days",
			duration: 100*24*time.Hour + 11*time.Hour + 59*time.Minute,
			want:     "100d 11h 59m",
		},
		{
			name:     "23 hours 59 minutes",
			duration: 23*time.Hour + 59*time.Minute,
			want:     "23h 59m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestFormatBytesEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
		{
			name:     "one byte",
			bytes:    1,
			expected: "1 B",
		},
		{
			name:     "just under 1 KiB",
			bytes:    1023,
			expected: "1023 B",
		},
		{
			name:     "exactly 1 KiB",
			bytes:    1024,
			expected: "1.0 KiB",
		},
		{
			name:     "1.5 KiB",
			bytes:    1536,
			expected: "1.5 KiB",
		},
		{
			name:     "exactly 1 MiB",
			bytes:    1024 * 1024,
			expected: "1.0 MiB",
		},
		{
			name:     "exactly 1 GiB",
			bytes:    1024 * 1024 * 1024,
			expected: "1.0 GiB",
		},
		{
			name:     "exactly 1 TiB",
			bytes:    1024 * 1024 * 1024 * 1024,
			expected: "1.0 TiB",
		},
		{
			name:     "100 MB real world",
			bytes:    100 * 1024 * 1024,
			expected: "100.0 MiB",
		},
		{
			name:     "500 bytes",
			bytes:    500,
			expected: "500 B",
		},
		{
			name:     "2.5 GiB",
			bytes:    int64(2.5 * 1024 * 1024 * 1024),
			expected: "2.5 GiB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestIsPortOpenInvalidAddresses(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{name: "empty address", addr: ""},
		{name: "no port", addr: "localhost"},
		{name: "invalid host", addr: "this-host-does-not-exist.invalid:80"},
		{name: "port zero", addr: "127.0.0.1:0"},
		{name: "high unused port", addr: "127.0.0.1:59999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPortOpen(tt.addr)
			if result {
				t.Errorf("isPortOpen(%q) = true, expected false for unreachable address", tt.addr)
			}
		})
	}
}

func TestPrintReportHealthy(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Duration:  2 * time.Second,
		SystemInfo: &SystemInfo{
			Hostname:     "prod-server",
			OS:           "linux",
			Kernel:       "6.1.0",
			Architecture: "amd64",
			CPUs:         8,
			Memory:       16 * 1024 * 1024 * 1024,
			Uptime:       "5d 3h 12m",
			GoVersion:    "go1.24",
		},
		Checks: []CheckResult{
			{Name: "FFmpeg", Category: "Tools", Status: StatusOK, Message: "FFmpeg available"},
			{Name: "Config", Category: "Config", Status: StatusOK, Message: "Config exists"},
			{Name: "Memory", Category: "Resources", Status: StatusOK, Message: "Memory OK"},
		},
		Summary: &Summary{Total: 3, OK: 3},
		Healthy: true,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)
	output := buf.String()

	// Verify header
	if !strings.Contains(output, "LyreBirdAudio Diagnostics Report") {
		t.Error("missing report title")
	}
	if !strings.Contains(output, "================================") {
		t.Error("missing title separator")
	}

	// Verify system info
	if !strings.Contains(output, "prod-server") {
		t.Error("missing hostname")
	}
	if !strings.Contains(output, "linux/amd64") {
		t.Error("missing OS/arch")
	}
	if !strings.Contains(output, "6.1.0") {
		t.Error("missing kernel version")
	}
	if !strings.Contains(output, "5d 3h 12m") {
		t.Error("missing uptime")
	}

	// Verify summary
	if !strings.Contains(output, "Total: 3") {
		t.Error("missing total count")
	}
	if !strings.Contains(output, "OK: 3") {
		t.Error("missing OK count")
	}
	if !strings.Contains(output, "HEALTHY") {
		t.Error("missing HEALTHY status")
	}

	// Verify categories are printed
	if !strings.Contains(output, "Tools") {
		t.Error("missing Tools category")
	}
	if !strings.Contains(output, "Config") {
		t.Error("missing Config category")
	}
	if !strings.Contains(output, "Resources") {
		t.Error("missing Resources category")
	}
}

func TestPrintReportUnhealthy(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "broken-server",
			OS:       "linux",
		},
		Checks: []CheckResult{
			{
				Name:        "Disk Space",
				Category:    "Resources",
				Status:      StatusCritical,
				Message:     "Disk usage critical: 97.5%",
				Suggestions: []string{"Free up disk space", "Remove old logs"},
			},
			{
				Name:     "Memory",
				Category: "Resources",
				Status:   StatusError,
				Message:  "Failed to read memory info",
			},
			{
				Name:     "Time Sync",
				Category: "System",
				Status:   StatusSkipped,
				Message:  "Skipped - timedatectl not available",
			},
			{
				Name:     "FFmpeg",
				Category: "Tools",
				Status:   StatusOK,
				Message:  "FFmpeg available",
			},
			{
				Name:        "Config",
				Category:    "Config",
				Status:      StatusWarning,
				Message:     "Config file not found",
				Details:     "/etc/lyrebird/config.yaml",
				Suggestions: []string{"Run: lyrebird setup"},
			},
		},
		Summary: &Summary{Total: 5, OK: 1, Warning: 1, Critical: 1, Error: 1, Skipped: 1},
		Healthy: false,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)
	output := buf.String()

	// All status symbols present
	if !strings.Contains(output, "✓") {
		t.Error("missing OK symbol")
	}
	if !strings.Contains(output, "⚠") {
		t.Error("missing warning symbol")
	}
	if !strings.Contains(output, "✗") {
		t.Error("missing critical symbol")
	}
	if !strings.Contains(output, "!") {
		t.Error("missing error symbol")
	}
	if !strings.Contains(output, "○") {
		t.Error("missing skipped symbol")
	}

	// Suggestions with arrow
	if !strings.Contains(output, "→ Free up disk space") {
		t.Error("missing suggestion with arrow prefix")
	}
	if !strings.Contains(output, "→ Remove old logs") {
		t.Error("missing second suggestion")
	}
	if !strings.Contains(output, "→ Run: lyrebird setup") {
		t.Error("missing config suggestion")
	}

	// Details line
	if !strings.Contains(output, "/etc/lyrebird/config.yaml") {
		t.Error("missing details line")
	}

	// Summary counts
	if !strings.Contains(output, "Critical: 1") {
		t.Error("missing Critical count in summary")
	}
	if !strings.Contains(output, "Error: 1") {
		t.Error("missing Error count in summary")
	}
	if !strings.Contains(output, "Skipped: 1") {
		t.Error("missing Skipped count in summary")
	}

	// Unhealthy status
	if !strings.Contains(output, "ISSUES DETECTED") {
		t.Error("missing ISSUES DETECTED status")
	}
}

func TestPrintReportEmptyChecks(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:  time.Now(),
		Duration:   time.Millisecond,
		SystemInfo: &SystemInfo{Hostname: "empty", OS: "linux"},
		Checks:     []CheckResult{},
		Summary:    &Summary{},
		Healthy:    true,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)
	output := buf.String()

	if !strings.Contains(output, "Total: 0") {
		t.Error("expected total of 0 checks")
	}
	if !strings.Contains(output, "HEALTHY") {
		t.Error("empty report should show HEALTHY")
	}
}

func TestToJSONComplete(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		Duration:  5 * time.Second,
		SystemInfo: &SystemInfo{
			Hostname:     "json-host",
			OS:           "linux",
			Kernel:       "6.1.0",
			Architecture: "arm64",
			CPUs:         4,
			Memory:       4 * 1024 * 1024 * 1024,
			Uptime:       "10d 2h 30m",
			GoVersion:    "go1.24",
		},
		Checks: []CheckResult{
			{
				Name:        "Test Check",
				Category:    "Test",
				Status:      StatusWarning,
				Message:     "Something warned",
				Details:     "detail info",
				Duration:    100 * time.Millisecond,
				Suggestions: []string{"Fix it"},
			},
			{
				Name:     "OK Check",
				Category: "Test",
				Status:   StatusOK,
				Message:  "All good",
				Duration: 50 * time.Millisecond,
			},
		},
		Summary: &Summary{Total: 2, OK: 1, Warning: 1},
		Healthy: true,
	}

	data, err := report.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() returned error: %v", err)
	}

	// Verify it is valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}

	// Verify key fields
	jsonStr := string(data)
	expectedFields := []string{
		`"hostname": "json-host"`,
		`"os": "linux"`,
		`"architecture": "arm64"`,
		`"healthy": true`,
		`"total": 2`,
		`"ok": 1`,
		`"warning": 1`,
		`"name": "Test Check"`,
		`"status": "WARNING"`,
		`"details": "detail info"`,
		`"suggestions"`,
		`"Fix it"`,
		`"name": "OK Check"`,
		`"status": "OK"`,
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("JSON missing expected field: %s", field)
		}
	}

	// Verify omitempty: OK Check should not have "details" key
	// Re-parse checks to verify
	var fullReport DiagnosticReport
	if err := json.Unmarshal(data, &fullReport); err != nil {
		t.Fatalf("failed to unmarshal back: %v", err)
	}

	if len(fullReport.Checks) != 2 {
		t.Errorf("expected 2 checks after round-trip, got %d", len(fullReport.Checks))
	}

	if fullReport.SystemInfo.Hostname != "json-host" {
		t.Errorf("hostname mismatch after round-trip: got %q", fullReport.SystemInfo.Hostname)
	}

	if fullReport.Summary.Total != 2 {
		t.Errorf("summary total mismatch after round-trip: got %d", fullReport.Summary.Total)
	}
}

func TestToJSONMinimal(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:  time.Now(),
		SystemInfo: &SystemInfo{},
		Summary:    &Summary{},
	}

	data, err := report.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error on minimal report: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON for minimal report")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("minimal report produced invalid JSON: %v", err)
	}
}

func TestPrintReportMultipleCategories(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "multi-cat",
			OS:       "linux",
		},
		Checks: []CheckResult{
			{Name: "FFmpeg", Category: "Tools", Status: StatusOK, Message: "Available"},
			{Name: "ALSA", Category: "Audio", Status: StatusOK, Message: "Available"},
			{Name: "Config", Category: "Config", Status: StatusOK, Message: "Exists"},
			{Name: "Memory", Category: "Resources", Status: StatusWarning, Message: "High usage", Details: "85% used"},
			{Name: "Disk", Category: "Resources", Status: StatusOK, Message: "OK"},
			{Name: "MediaMTX", Category: "Services", Status: StatusCritical, Message: "Not running"},
		},
		Summary: &Summary{Total: 6, OK: 4, Warning: 1, Critical: 1},
		Healthy: false,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)
	output := buf.String()

	// Verify category headers with separators
	categories := []string{"Tools", "Audio", "Config", "Resources", "Services"}
	for _, cat := range categories {
		if !strings.Contains(output, cat) {
			t.Errorf("missing category: %s", cat)
		}
		// Category should have a dash separator line
		if !strings.Contains(output, strings.Repeat("-", len(cat))) {
			t.Errorf("missing separator for category: %s", cat)
		}
	}

	// Details should be indented
	if !strings.Contains(output, "    85% used") {
		t.Error("missing indented details line")
	}
}
