// SPDX-License-Identifier: MIT

//go:build linux

package stream

import (
	"strings"
	"testing"
)

// FuzzParseCPUJiffies hammers the /proc/{pid}/stat CPU-time parser with
// arbitrary content. The stat file is parsed on every resource-monitor tick
// for the life of the daemon; adversarial comm fields (a process name may
// legally contain spaces, parentheses and newlines), truncated reads and
// exotic field values must fail cleanly, never panic, and never return a
// value without ok=true.
func FuzzParseCPUJiffies(f *testing.F) {
	seeds := []string{
		"", ")", "no paren at all",
		"1234 (ffmpeg) S 1 2 3 4 5 6 7 8 9 10 11 100 200 14 15 16 17 18 19 20",
		"1234 (ffmpeg) S 1 2 3 4 5 6 7 8 9 10 11 100", // too few fields
		"1234 (weird) name) R 1 2 3 4 5 6 7 8 9 10 11 100 200 1 1 1 1 1 1 1",
		"1234 (a\nb) S 1 2 3 4 5 6 7 8 9 10 11 100 200 1 1 1 1 1 1 1",
		"1234 (ffmpeg) S 1 2 3 4 5 6 7 8 9 10 11 -100 200 1 1 1 1 1 1 1",   // negative utime
		"1234 (ffmpeg) S 1 2 3 4 5 6 7 8 9 10 11 1e9 200 1 1 1 1 1 1 1",    // non-integer
		"1234 (ffmpeg) S 1 2 3 4 5 6 7 8 9 10 11 18446744073709551615 1 1", // uint64 max, short
		"1234 (ffmpeg) S 1 2 3 4 5 6 7 8 9 10 11 18446744073709551615 18446744073709551615 1 1 1 1 1 1 1",
		strings.Repeat("(", 4096),
		strings.Repeat("1 ", 4096) + ")",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, stat string) {
		jiffies, ok := parseCPUJiffies(stat)
		if !ok && jiffies != 0 {
			t.Errorf("parseCPUJiffies(%q) = (%d, false): value must be 0 when not ok", stat, jiffies)
		}
		// Parsing must be deterministic.
		j2, ok2 := parseCPUJiffies(stat)
		if j2 != jiffies || ok2 != ok {
			t.Errorf("parseCPUJiffies(%q) not deterministic: (%d,%v) vs (%d,%v)", stat, jiffies, ok, j2, ok2)
		}
		// The sibling parsers walk the same untrusted content on the same tick;
		// they must not panic on it either.
		_ = parseThreadCount(stat)
		_ = parseMemoryBytes(stat)
	})
}
