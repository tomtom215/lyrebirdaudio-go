// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"fmt"
	"strings"
	"testing"
)

func TestEvaluateNetworkPorts(t *testing.T) {
	tests := []struct {
		name           string
		rtspOpen       bool
		apiOpen        bool
		expectedStatus CheckStatus
		expectedMsg    string
		hasSuggestion  bool
	}{
		{
			name:           "both open",
			rtspOpen:       true,
			apiOpen:        true,
			expectedStatus: StatusOK,
			expectedMsg:    fmt.Sprintf("RTSP (%d) and API (%d) ports accessible", DefaultRTSPPort, DefaultAPIPort),
			hasSuggestion:  false,
		},
		{
			name:           "both closed",
			rtspOpen:       false,
			apiOpen:        false,
			expectedStatus: StatusWarning,
			expectedMsg:    "RTSP and API ports not accessible",
			hasSuggestion:  true,
		},
		{
			name:           "only RTSP open",
			rtspOpen:       true,
			apiOpen:        false,
			expectedStatus: StatusWarning,
			expectedMsg:    fmt.Sprintf("API (%d)", DefaultAPIPort),
			hasSuggestion:  false,
		},
		{
			name:           "only API open",
			rtspOpen:       false,
			apiOpen:        true,
			expectedStatus: StatusWarning,
			expectedMsg:    fmt.Sprintf("RTSP (%d)", DefaultRTSPPort),
			hasSuggestion:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, suggestions := evaluateNetworkPorts(tt.rtspOpen, tt.apiOpen)
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

func TestEvaluateTCPResources(t *testing.T) {
	tests := []struct {
		name           string
		ssOutput       string
		expectedStatus CheckStatus
		expectedMsg    string
	}{
		{
			name:           "no connections",
			ssOutput:       "State\tRecv-Q\tSend-Q\n",
			expectedStatus: StatusOK,
			expectedMsg:    "TIME_WAIT connections: 0",
		},
		{
			name:           "few connections",
			ssOutput:       "header\nconn1\nconn2\nconn3\n",
			expectedStatus: StatusOK,
			expectedMsg:    "TIME_WAIT connections: 3",
		},
		{
			name:           "empty output",
			ssOutput:       "",
			expectedStatus: StatusOK,
			expectedMsg:    "TIME_WAIT connections: 0",
		},
		{
			name:           "high connections over threshold",
			ssOutput:       "header\n" + strings.Repeat("connection\n", 1001),
			expectedStatus: StatusWarning,
			expectedMsg:    "High TIME_WAIT connections: 1001",
		},
		{
			name:           "exactly at threshold",
			ssOutput:       "header\n" + strings.Repeat("conn\n", 1000),
			expectedStatus: StatusOK,
			expectedMsg:    "TIME_WAIT connections: 1000",
		},
		{
			name:           "just over threshold",
			ssOutput:       "header\n" + strings.Repeat("conn\n", 1001),
			expectedStatus: StatusWarning,
			expectedMsg:    "High TIME_WAIT connections: 1001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := evaluateTCPResources(tt.ssOutput)
			if status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, status)
			}
			if !strings.Contains(msg, tt.expectedMsg) {
				t.Errorf("expected message containing %q, got %q", tt.expectedMsg, msg)
			}
		})
	}
}
