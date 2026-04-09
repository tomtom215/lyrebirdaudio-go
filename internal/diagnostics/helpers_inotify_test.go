// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"strings"
	"testing"
)

func TestEvaluateInotifyLimits(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		expectedStatus CheckStatus
		expectedMsg    string
		hasSuggestion  bool
	}{
		{
			name:           "sufficient watches",
			data:           "65536",
			expectedStatus: StatusOK,
			expectedMsg:    "inotify max_user_watches: 65536",
			hasSuggestion:  false,
		},
		{
			name:           "low watches",
			data:           "1024",
			expectedStatus: StatusWarning,
			expectedMsg:    "inotify max_user_watches low: 1024",
			hasSuggestion:  true,
		},
		{
			name:           "exactly at threshold",
			data:           "8192",
			expectedStatus: StatusOK,
			expectedMsg:    "inotify max_user_watches: 8192",
			hasSuggestion:  false,
		},
		{
			name:           "just below threshold",
			data:           "8191",
			expectedStatus: StatusWarning,
			expectedMsg:    "inotify max_user_watches low: 8191",
			hasSuggestion:  true,
		},
		{
			name:           "invalid data",
			data:           "abc",
			expectedStatus: StatusError,
			expectedMsg:    "Could not parse inotify",
			hasSuggestion:  false,
		},
		{
			name:           "data with whitespace",
			data:           "  130044\n",
			expectedStatus: StatusOK,
			expectedMsg:    "inotify max_user_watches: 130044",
			hasSuggestion:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, suggestions := evaluateInotifyLimits(tt.data)
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
