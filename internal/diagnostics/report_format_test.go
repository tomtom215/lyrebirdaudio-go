// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"testing"
	"time"
)

func TestFormatDurationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "zero duration",
			duration: 0,
			want:     "0m",
		},
		{
			name:     "less than a minute",
			duration: 30 * time.Second,
			want:     "0m",
		},
		{
			name:     "exactly one minute",
			duration: time.Minute,
			want:     "1m",
		},
		{
			name:     "minutes only",
			duration: 45 * time.Minute,
			want:     "45m",
		},
		{
			name:     "exactly one hour",
			duration: time.Hour,
			want:     "1h 0m",
		},
		{
			name:     "hours and minutes",
			duration: 3*time.Hour + 42*time.Minute,
			want:     "3h 42m",
		},
		{
			name:     "exactly one day",
			duration: 24 * time.Hour,
			want:     "1d 0h 0m",
		},
		{
			name:     "days hours minutes",
			duration: 2*24*time.Hour + 5*time.Hour + 17*time.Minute,
			want:     "2d 5h 17m",
		},
		{
			name:     "many days",
			duration: 100*24*time.Hour + 11*time.Hour + 59*time.Minute,
			want:     "100d 11h 59m",
		},
		{
			name:     "23 hours 59 minutes",
			duration: 23*time.Hour + 59*time.Minute,
			want:     "23h 59m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestFormatBytesEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
		{
			name:     "one byte",
			bytes:    1,
			expected: "1 B",
		},
		{
			name:     "just under 1 KiB",
			bytes:    1023,
			expected: "1023 B",
		},
		{
			name:     "exactly 1 KiB",
			bytes:    1024,
			expected: "1.0 KiB",
		},
		{
			name:     "1.5 KiB",
			bytes:    1536,
			expected: "1.5 KiB",
		},
		{
			name:     "exactly 1 MiB",
			bytes:    1024 * 1024,
			expected: "1.0 MiB",
		},
		{
			name:     "exactly 1 GiB",
			bytes:    1024 * 1024 * 1024,
			expected: "1.0 GiB",
		},
		{
			name:     "exactly 1 TiB",
			bytes:    1024 * 1024 * 1024 * 1024,
			expected: "1.0 TiB",
		},
		{
			name:     "100 MB real world",
			bytes:    100 * 1024 * 1024,
			expected: "100.0 MiB",
		},
		{
			name:     "500 bytes",
			bytes:    500,
			expected: "500 B",
		},
		{
			name:     "2.5 GiB",
			bytes:    int64(2.5 * 1024 * 1024 * 1024),
			expected: "2.5 GiB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}
