//go:build linux

package diagnostics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1536, "1.5 KiB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, expected %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestPrintReport(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname:     "test-host",
			OS:           "linux",
			Kernel:       "5.4.0",
			Architecture: "amd64",
			CPUs:         4,
			Memory:       8 * 1024 * 1024 * 1024,
			GoVersion:    "go1.23",
		},
		Checks: []CheckResult{
			{
				Name:     "Test Check",
				Category: "Test",
				Status:   StatusOK,
				Message:  "All good",
				Duration: 100 * time.Millisecond,
			},
			{
				Name:        "Warning Check",
				Category:    "Test",
				Status:      StatusWarning,
				Message:     "Something to look at",
				Duration:    50 * time.Millisecond,
				Suggestions: []string{"Fix this", "Fix that"},
			},
		},
		Summary: &Summary{
			Total:   2,
			OK:      1,
			Warning: 1,
		},
		Healthy: true,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)

	output := buf.String()

	// Check that key elements are present
	if !strings.Contains(output, "LyreBirdAudio Diagnostics Report") {
		t.Error("expected output to contain title")
	}
	if !strings.Contains(output, "test-host") {
		t.Error("expected output to contain hostname")
	}
	if !strings.Contains(output, "Test Check") {
		t.Error("expected output to contain check name")
	}
	// PrintReport uses symbols (✓, ⚠) not text status
	if !strings.Contains(output, "✓") {
		t.Error("expected output to contain OK symbol ✓")
	}
	if !strings.Contains(output, "⚠") {
		t.Error("expected output to contain Warning symbol ⚠")
	}
	// Summary shows counts
	if !strings.Contains(output, "Warning: 1") {
		t.Error("expected output to contain Warning count")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{30 * time.Minute, "30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{26*time.Hour + 30*time.Minute, "1d 2h 30m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.duration)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}

func TestToJSON(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "test",
			OS:       "linux",
		},
		Checks: []CheckResult{
			{Name: "Test", Status: StatusOK},
		},
		Summary: &Summary{Total: 1, OK: 1},
		Healthy: true,
	}

	data, err := report.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}

	if !strings.Contains(string(data), "test") {
		t.Error("expected JSON to contain hostname")
	}
}

func TestPrintReportWithErrors(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "test-host",
			OS:       "linux",
			Kernel:   "5.4.0",
		},
		Checks: []CheckResult{
			{Name: "Critical Check", Category: "Test", Status: StatusCritical, Message: "Critical issue"},
			{Name: "Error Check", Category: "Test", Status: StatusError, Message: "Error occurred"},
			{Name: "Skipped Check", Category: "Test", Status: StatusSkipped, Message: "Skipped"},
		},
		Summary: &Summary{Total: 3, Critical: 1, Error: 1, Skipped: 1},
		Healthy: false,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)

	output := buf.String()

	if !strings.Contains(output, "✗") {
		t.Error("expected output to contain critical symbol ✗")
	}
	if !strings.Contains(output, "!") {
		t.Error("expected output to contain error symbol !")
	}
	if !strings.Contains(output, "○") {
		t.Error("expected output to contain skipped symbol ○")
	}
	if !strings.Contains(output, "ISSUES DETECTED") {
		t.Error("expected output to indicate issues detected")
	}
}

func TestPrintReportWithDetails(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "test-host",
			OS:       "linux",
		},
		Checks: []CheckResult{
			{
				Name:        "Detail Check",
				Category:    "Test",
				Status:      StatusWarning,
				Message:     "Warning message",
				Details:     "Detailed information here",
				Suggestions: []string{"Fix suggestion 1", "Fix suggestion 2"},
			},
		},
		Summary: &Summary{Total: 1, Warning: 1},
		Healthy: true,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)

	output := buf.String()

	if !strings.Contains(output, "Detailed information") {
		t.Error("expected output to contain details")
	}
	if !strings.Contains(output, "Fix suggestion 1") {
		t.Error("expected output to contain suggestion")
	}
}
