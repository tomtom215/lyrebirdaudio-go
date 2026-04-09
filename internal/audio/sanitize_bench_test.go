package audio

import (
	"strings"
	"testing"
)

// BenchmarkSanitizeDeviceName measures performance for hot path.
func BenchmarkSanitizeDeviceName(b *testing.B) {
	testCases := []string{
		"Blue Yeti",
		"USB Audio Device",
		"Blue Microphones Yeti Stereo Microphone REV8_00",
		"Device!@#$%^&*()",
		strings.Repeat("a", 100),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			SanitizeDeviceName(tc)
		}
	}
}
