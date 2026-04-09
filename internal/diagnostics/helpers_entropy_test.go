// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"strings"
	"testing"
)

func TestEvaluateEntropy(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		expectedStatus CheckStatus
		expectedMsg    string
		hasSuggestion  bool
	}{
		{
			name:           "sufficient entropy",
			data:           "3000",
			expectedStatus: StatusOK,
			expectedMsg:    "Entropy pool: 3000",
			hasSuggestion:  false,
		},
		{
			name:           "low entropy",
			data:           "100",
			expectedStatus: StatusWarning,
			expectedMsg:    "Entropy pool low: 100",
			hasSuggestion:  true,
		},
		{
			name:           "exactly at threshold",
			data:           "256",
			expectedStatus: StatusOK,
			expectedMsg:    "Entropy pool: 256",
			hasSuggestion:  false,
		},
		{
			name:           "just below threshold",
			data:           "255",
			expectedStatus: StatusWarning,
			expectedMsg:    "Entropy pool low: 255",
			hasSuggestion:  true,
		},
		{
			name:           "invalid data",
			data:           "not-a-number",
			expectedStatus: StatusError,
			expectedMsg:    "Could not parse entropy",
			hasSuggestion:  false,
		},
		{
			name:           "zero entropy",
			data:           "0",
			expectedStatus: StatusWarning,
			expectedMsg:    "Entropy pool low: 0",
			hasSuggestion:  true,
		},
		{
			name:           "data with whitespace",
			data:           "  512\n",
			expectedStatus: StatusOK,
			expectedMsg:    "Entropy pool: 512",
			hasSuggestion:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, suggestions := evaluateEntropy(tt.data)
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
				t.Error("expected no suggestions")
			}
		})
	}
}
