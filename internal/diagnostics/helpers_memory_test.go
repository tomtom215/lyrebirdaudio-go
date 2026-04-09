// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"strings"
	"testing"
)

func TestEvaluateMemoryUsage(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		expectedStatus CheckStatus
		expectedMsg    string
	}{
		{
			name: "normal usage",
			data: `MemTotal:       16000000 kB
MemFree:         8000000 kB
MemAvailable:   12000000 kB`,
			expectedStatus: StatusOK,
			expectedMsg:    "Memory usage: 25.0%",
		},
		{
			name: "warning usage 80%",
			data: `MemTotal:       16000000 kB
MemFree:         1000000 kB
MemAvailable:    3200000 kB`,
			expectedStatus: StatusWarning,
			expectedMsg:    "Memory usage elevated: 80.0%",
		},
		{
			name: "critical usage 95%",
			data: `MemTotal:       16000000 kB
MemFree:          100000 kB
MemAvailable:     800000 kB`,
			expectedStatus: StatusCritical,
			expectedMsg:    "Memory usage critical: 95.0%",
		},
		{
			name:           "empty data",
			data:           "",
			expectedStatus: StatusError,
			expectedMsg:    "Could not determine total memory",
		},
		{
			name: "missing MemAvailable",
			data: `MemTotal:       16000000 kB
MemFree:         8000000 kB`,
			expectedStatus: StatusCritical,
			expectedMsg:    "Memory usage critical",
		},
		{
			name: "zero available memory",
			data: `MemTotal:       16000000 kB
MemAvailable:          0 kB`,
			expectedStatus: StatusCritical,
			expectedMsg:    "Memory usage critical: 100.0%",
		},
		{
			name: "exactly at warning threshold 76%",
			data: `MemTotal:       10000000 kB
MemAvailable:    2400000 kB`,
			expectedStatus: StatusWarning,
			expectedMsg:    "Memory usage elevated",
		},
		{
			name: "just under warning threshold",
			data: `MemTotal:       10000000 kB
MemAvailable:    2600000 kB`,
			expectedStatus: StatusOK,
			expectedMsg:    "Memory usage: 74.0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := evaluateMemoryUsage(tt.data)
			if status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s (msg: %s)", tt.expectedStatus, status, msg)
			}
			if !strings.Contains(msg, tt.expectedMsg) {
				t.Errorf("expected message containing %q, got %q", tt.expectedMsg, msg)
			}
		})
	}
}
