// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"strings"
	"testing"
)

func TestEvaluateDiskUsage(t *testing.T) {
	tests := []struct {
		name           string
		usedPercent    float64
		available      uint64
		expectedStatus CheckStatus
		expectedMsg    string
		hasSuggestion  bool
	}{
		{
			name:           "normal usage 50%",
			usedPercent:    50.0,
			available:      500 * 1024 * 1024 * 1024,
			expectedStatus: StatusOK,
			expectedMsg:    "Disk usage: 50.0%",
			hasSuggestion:  false,
		},
		{
			name:           "warning usage 90%",
			usedPercent:    90.0,
			available:      100 * 1024 * 1024 * 1024,
			expectedStatus: StatusWarning,
			expectedMsg:    "Disk usage high: 90.0%",
			hasSuggestion:  false,
		},
		{
			name:           "critical usage 97%",
			usedPercent:    97.0,
			available:      30 * 1024 * 1024 * 1024,
			expectedStatus: StatusCritical,
			expectedMsg:    "Disk usage critical: 97.0%",
			hasSuggestion:  true,
		},
		{
			name:           "exactly at warning threshold",
			usedPercent:    85.1,
			available:      150 * 1024 * 1024 * 1024,
			expectedStatus: StatusWarning,
			expectedMsg:    "Disk usage high",
			hasSuggestion:  false,
		},
		{
			name:           "exactly at critical threshold",
			usedPercent:    95.1,
			available:      50 * 1024 * 1024 * 1024,
			expectedStatus: StatusCritical,
			expectedMsg:    "Disk usage critical",
			hasSuggestion:  true,
		},
		{
			name:           "zero usage",
			usedPercent:    0.0,
			available:      1024 * 1024 * 1024 * 1024,
			expectedStatus: StatusOK,
			expectedMsg:    "Disk usage: 0.0%",
			hasSuggestion:  false,
		},
		{
			name:           "just under warning",
			usedPercent:    84.9,
			available:      150 * 1024 * 1024 * 1024,
			expectedStatus: StatusOK,
			expectedMsg:    "Disk usage: 84.9%",
			hasSuggestion:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, suggestions := evaluateDiskUsage(tt.usedPercent, tt.available)
			if status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, status)
			}
			if !strings.Contains(msg, tt.expectedMsg) {
				t.Errorf("expected message containing %q, got %q", tt.expectedMsg, msg)
			}
			if tt.hasSuggestion && len(suggestions) == 0 {
				t.Error("expected suggestions")
			}
			if !tt.hasSuggestion && len(suggestions) > 0 {
				t.Errorf("expected no suggestions, got %v", suggestions)
			}
		})
	}
}
