// SPDX-License-Identifier: MIT

//go:build linux

package stream

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCPUPercent(t *testing.T) {
	tests := []struct {
		name    string
		jiffies uint64
		dt      time.Duration
		want    float64
	}{
		{"full core one second", 100, time.Second, 100},
		{"half core one second", 50, time.Second, 50},
		{"two cores one second", 200, time.Second, 200},
		{"full core over two seconds", 100, 2 * time.Second, 50},
		{"no work", 0, time.Second, 0},
		{"zero interval guarded", 100, 0, 0},
		{"negative interval guarded", 100, -time.Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cpuPercent(tt.jiffies, tt.dt)
			if got != tt.want {
				t.Errorf("cpuPercent(%d, %v) = %v, want %v", tt.jiffies, tt.dt, got, tt.want)
			}
		})
	}
}

func TestParseCPUJiffies(t *testing.T) {
	tests := []struct {
		name   string
		stat   string
		want   uint64
		wantOK bool
	}{
		{
			// comm contains spaces AND nested parens — must skip to the last ')'.
			name:   "comm with embedded parens",
			stat:   "1234 (my (weird) proc) S 1 1 1 0 -1 0 0 0 0 0 500 250 0 0 20 0 3 0 999 0 0",
			want:   750,
			wantOK: true,
		},
		{
			name:   "simple comm",
			stat:   "42 (ffmpeg) S 1 1 1 0 -1 0 0 0 0 0 10 5 0 0 20 0 2 0 100 0 0",
			want:   15,
			wantOK: true,
		},
		{"no comm close paren", "42 (ffmpeg S 1 2 3", 0, false},
		{"too few fields", "42 (ffmpeg) S 1 2 3", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseCPUJiffies(tt.stat)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("parseCPUJiffies(%q) = (%d, %v), want (%d, %v)", tt.stat, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// writeFakeProc creates a minimal /proc/<pid>/ tree with a stat file carrying
// the given utime+stime, plus statm and an fd dir, under a fake proc root.
func writeFakeProc(t *testing.T, procRoot string, pid int, utime, stime uint64) {
	t.Helper()
	pidDir := filepath.Join(procRoot, strconv.Itoa(pid))
	if err := os.MkdirAll(filepath.Join(pidDir, "fd"), 0o755); err != nil {
		t.Fatalf("mkdir fd: %v", err)
	}
	// Fields: pid (comm) state ppid pgrp session tty tpgid flags minflt cminflt
	// majflt cmajflt utime stime cutime cstime prio nice threads itreal starttime
	stat := fmt.Sprintf("%d (ffmpeg) S 1 1 1 0 -1 0 0 0 0 0 %d %d 0 0 20 0 2 0 500 0 0", pid, utime, stime)
	if err := os.WriteFile(filepath.Join(pidDir, "stat"), []byte(stat), 0o644); err != nil {
		t.Fatalf("write stat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "statm"), []byte("100 20 10 0 0 0 0"), 0o644); err != nil {
		t.Fatalf("write statm: %v", err)
	}
}

// TestGetMetricsComputesCPUPercent verifies the delta wiring end-to-end: the
// first sample reports 0% (no interval yet), and a second sample with advanced
// jiffies over a seeded ~1s interval reports a sensible per-core rate.
func TestGetMetricsComputesCPUPercent(t *testing.T) {
	procRoot := t.TempDir()
	const pid = 4242

	m := NewResourceMonitor(WithProcPath(procRoot))

	// First sample: 1000 jiffies, no prior → CPUPercent must be 0.
	writeFakeProc(t, procRoot, pid, 600, 400) // utime+stime = 1000
	first, err := m.GetMetrics(pid)
	if err != nil {
		t.Fatalf("GetMetrics (first): %v", err)
	}
	if first.CPUPercent != 0 {
		t.Errorf("first sample CPUPercent = %v, want 0", first.CPUPercent)
	}

	// Seed the previous sample ~1s in the past so the interval is deterministic,
	// then advance by 100 jiffies (= 1 CPU-second at userHZ=100 → ~100%).
	m.mu.Lock()
	m.prevCPU[pid] = cpuSample{jiffies: 1000, at: time.Now().Add(-time.Second)}
	m.mu.Unlock()

	writeFakeProc(t, procRoot, pid, 660, 440) // utime+stime = 1100 (delta 100)
	second, err := m.GetMetrics(pid)
	if err != nil {
		t.Fatalf("GetMetrics (second): %v", err)
	}
	if second.CPUPercent < 50 || second.CPUPercent > 200 {
		t.Errorf("second sample CPUPercent = %v, want a sane per-core rate near 100", second.CPUPercent)
	}
}

// TestGetMetricsCPUPercentPIDReuse verifies a jiffies decrease (PID reuse) does
// not produce a bogus negative/huge spike — it is skipped, reporting 0.
func TestGetMetricsCPUPercentPIDReuse(t *testing.T) {
	procRoot := t.TempDir()
	const pid = 777

	m := NewResourceMonitor(WithProcPath(procRoot))
	m.mu.Lock()
	m.prevCPU[pid] = cpuSample{jiffies: 5000, at: time.Now().Add(-time.Second)}
	m.mu.Unlock()

	writeFakeProc(t, procRoot, pid, 100, 100) // 200 < previous 5000 → reuse
	got, err := m.GetMetrics(pid)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if got.CPUPercent != 0 {
		t.Errorf("CPUPercent on PID reuse = %v, want 0", got.CPUPercent)
	}
}
