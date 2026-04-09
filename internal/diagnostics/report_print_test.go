// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

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
