// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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
