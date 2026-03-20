// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"fmt"
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
