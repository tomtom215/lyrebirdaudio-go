// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"strings"
	"testing"
)

func TestEvaluateAudioConflicts(t *testing.T) {
	tests := []struct {
		name           string
		pulseInstalled bool
		pulseActive    bool
		expectedStatus CheckStatus
		expectedMsg    string
		hasSuggestion  bool
	}{
		{
			name:           "pulse running",
			pulseInstalled: true,
			pulseActive:    true,
			expectedStatus: StatusWarning,
			expectedMsg:    "PulseAudio running",
			hasSuggestion:  true,
		},
		{
			name:           "pulse installed not running",
			pulseInstalled: true,
			pulseActive:    false,
			expectedStatus: StatusOK,
			expectedMsg:    "PulseAudio installed but not running",
			hasSuggestion:  false,
		},
		{
			name:           "pulse not installed",
			pulseInstalled: false,
			pulseActive:    false,
			expectedStatus: StatusOK,
			expectedMsg:    "No audio conflicts detected",
			hasSuggestion:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, suggestions := evaluateAudioConflicts(tt.pulseInstalled, tt.pulseActive)
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
