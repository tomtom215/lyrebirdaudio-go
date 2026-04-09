// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"strings"
	"testing"
)

func TestEvaluateFDUsage(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		expectedStatus CheckStatus
		expectedMsg    string
	}{
		{
			name:           "normal usage",
			data:           "100\t0\t1000000",
			expectedStatus: StatusOK,
			expectedMsg:    "FD usage normal",
		},
		{
			name:           "warning usage over 50%",
			data:           "600000\t0\t1000000",
			expectedStatus: StatusWarning,
			expectedMsg:    "FD usage elevated",
		},
		{
			name:           "critical usage over 80%",
			data:           "900000\t0\t1000000",
			expectedStatus: StatusCritical,
			expectedMsg:    "FD usage critical",
		},
		{
			name:           "exactly at warning threshold",
			data:           "500001\t0\t1000000",
			expectedStatus: StatusWarning,
			expectedMsg:    "FD usage elevated",
		},
		{
			name:           "exactly at critical threshold",
			data:           "800001\t0\t1000000",
			expectedStatus: StatusCritical,
			expectedMsg:    "FD usage critical",
		},
		{
			name:           "invalid format too few fields",
			data:           "100",
			expectedStatus: StatusError,
			expectedMsg:    "Invalid file-nr format",
		},
		{
			name:           "invalid format empty",
			data:           "",
			expectedStatus: StatusError,
			expectedMsg:    "Invalid file-nr format",
		},
		{
			name:           "max is zero",
			data:           "100\t0\t0",
			expectedStatus: StatusError,
			expectedMsg:    "Invalid max file descriptors (0)",
		},
		{
			name:           "zero usage",
			data:           "0\t0\t1000000",
			expectedStatus: StatusOK,
			expectedMsg:    "FD usage normal: 0.0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := evaluateFDUsage(tt.data)
			if status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, status)
			}
			if !strings.Contains(msg, tt.expectedMsg) {
				t.Errorf("expected message containing %q, got %q", tt.expectedMsg, msg)
			}
		})
	}
}
