//go:build linux

package stream

import (
	"os"
	"strings"
	"testing"
)

func TestParseThreadCount(t *testing.T) {
	tests := []struct {
		stat string
		want int
	}{
		{"1 (test) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 5 0 1 0\n", 5},
		{"", 0},        // Returns 0 for empty input
		{"invalid", 0}, // Returns 0 for invalid input (no ")")
	}

	for _, tt := range tests {
		got := parseThreadCount(tt.stat)
		if got != tt.want {
			t.Errorf("parseThreadCount(%q) = %d, want %d", tt.stat[:min(20, len(tt.stat))], got, tt.want)
		}
	}
}

func TestParseMemoryBytes(t *testing.T) {
	pageSize := int64(os.Getpagesize())

	tests := []struct {
		statm string
		want  int64
	}{
		// parseMemoryBytes uses field[1] (resident set size), not field[0] (total size)
		{"1000 500 100 10 0 500 0", 500 * pageSize},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		got := parseMemoryBytes(tt.statm)
		if got != tt.want {
			t.Errorf("parseMemoryBytes(%q) = %d, want %d", tt.statm, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
		{1536, "1.5 KiB"},                           // 1.5 KB
		{2 * 1024 * 1024, "2.0 MiB"},                // 2 MB
		{1024*1024*1024 + 512*1024*1024, "1.5 GiB"}, // 1.5 GB
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestParseThreadCountEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		stat string
		want int
	}{
		{"normal", "1 (test) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 5 0 1 0\n", 5},
		{"empty", "", 0},
		{"no_paren", "invalid", 0},
		{"insufficient_fields", "1 (test) S 1 2", 0},
		{"non_numeric_thread", "1 (test) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 abc 0", 0},
		{"with_spaces_in_name", "1 (test process) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 7 0 1 0\n", 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseThreadCount(tt.stat)
			if got != tt.want {
				t.Errorf("parseThreadCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseMemoryBytesEdgeCases(t *testing.T) {
	pageSize := int64(os.Getpagesize())

	tests := []struct {
		name  string
		statm string
		want  int64
	}{
		{"normal", "1000 500 100 10 0 500 0", 500 * pageSize},
		{"empty", "", 0},
		{"single_field", "1000", 0},
		{"non_numeric", "abc def", 0},
		{"zero_rss", "1000 0 100 10 0 500 0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMemoryBytes(tt.statm)
			if got != tt.want {
				t.Errorf("parseMemoryBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

// FuzzParseThreadCount fuzz tests parseThreadCount with arbitrary /proc/pid/stat content.
//
// Invariants verified:
//   - No panics on any input
//   - Return value is always >= 0
func FuzzParseThreadCount(f *testing.F) {
	// Seed corpus: realistic /proc/pid/stat formats and edge cases
	seeds := []string{
		// Valid stat lines with various thread counts
		"1 (test) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 5 0 1 0\n",
		"12345 (test) S 1 12345 12345 0 -1 4194304 100 0 0 0 10 5 0 0 20 0 3 0 1000 1000000 100\n",
		"999 (ffmpeg) R 1 999 999 0 -1 0 0 0 0 0 100 50 0 0 20 0 12 0 5000 2000000 200\n",
		"1 (test process) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 7 0 1 0\n",

		// Edge cases
		"",
		"invalid",
		"no_closing_paren",
		"1 (test) S 1 2", // Too few fields after comm
		"1 (test) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 abc 0", // Non-numeric thread count
		"1 () S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0\n",      // Empty comm field
		") S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0\n",         // Leading closing paren
		"1 (a)b) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 2 0 1 0\n",   // Paren in comm name
		"1 (test) S",                             // Minimal fields
		"1 (test)",                               // Just pid and comm
		"\n\n\n",                                 // Only newlines
		"1 (test) S " + strings.Repeat("0 ", 50), // Many fields
		"1 (test) S " + strings.Repeat("999999999 ", 20), // Large numeric values
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, stat string) {
		result := parseThreadCount(stat)

		// Invariant 1: Return value must be >= 0
		if result < 0 {
			t.Errorf("parseThreadCount(%q) = %d, want >= 0", stat, result)
		}
	})
}

// FuzzParseMemoryBytes fuzz tests parseMemoryBytes with arbitrary /proc/pid/statm content.
//
// Invariants verified:
//   - No panics on any input
//   - Return value is always >= 0
func FuzzParseMemoryBytes(f *testing.F) {
	// Seed corpus: realistic /proc/pid/statm formats and edge cases
	seeds := []string{
		// Valid statm lines: size resident shared text lib data dt
		"1000 500 100 10 0 500 0",
		"50000 25000 5000 100 0 20000 0",
		"0 0 0 0 0 0 0",
		"1 1 0 0 0 0 0",

		// Edge cases
		"",
		"invalid",
		"abc def",
		"1000",                                // Single field, no resident field
		"1000 abc",                            // Non-numeric resident field
		"-1 -500 0 0 0 0 0",                   // Negative values
		"0 0",                                 // Minimal valid (two fields)
		"999999999999 999999999999 0 0 0 0 0", // Very large values
		"\t100\t200\t300",                     // Tab-separated
		"  100  200  300  ",                   // Extra whitespace
		"\n",                                  // Just newline
		"100 200\n",                           // Trailing newline (common in /proc)
		"100 200 300 400 500 600 700 800 900", // Extra fields
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, statm string) {
		result := parseMemoryBytes(statm)

		// Invariant 1: Return value must be >= 0
		if result < 0 {
			t.Errorf("parseMemoryBytes(%q) = %d, want >= 0", statm, result)
		}
	})
}
