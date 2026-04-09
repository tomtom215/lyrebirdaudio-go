//go:build linux

package diagnostics

import (
	"testing"
	"time"
)

func TestCheckStatus(t *testing.T) {
	tests := []struct {
		status   CheckStatus
		expected string
	}{
		{StatusOK, "OK"},
		{StatusWarning, "WARNING"},
		{StatusCritical, "CRITICAL"},
		{StatusSkipped, "SKIPPED"},
		{StatusError, "ERROR"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, string(tt.status))
		}
	}
}

func TestCheckMode(t *testing.T) {
	tests := []struct {
		mode     CheckMode
		expected string
	}{
		{ModeQuick, "quick"},
		{ModeFull, "full"},
		{ModeDebug, "debug"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, string(tt.mode))
		}
	}
}

func TestCheckResultFields(t *testing.T) {
	result := CheckResult{
		Name:        "Test",
		Category:    "Unit Test",
		Status:      StatusOK,
		Message:     "Test passed",
		Details:     "Additional info",
		Duration:    100 * time.Millisecond,
		Suggestions: []string{"Suggestion 1"},
	}

	if result.Name != "Test" {
		t.Errorf("expected Name to be 'Test', got %q", result.Name)
	}
	if result.Category != "Unit Test" {
		t.Errorf("expected Category to be 'Unit Test', got %q", result.Category)
	}
	if result.Status != StatusOK {
		t.Errorf("expected Status to be OK, got %q", result.Status)
	}
	if result.Message != "Test passed" {
		t.Errorf("expected Message to be 'Test passed', got %q", result.Message)
	}
	if result.Details != "Additional info" {
		t.Errorf("expected Details to be 'Additional info', got %q", result.Details)
	}
	if len(result.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(result.Suggestions))
	}
}
