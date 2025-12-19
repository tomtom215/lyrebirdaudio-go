package stream

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNewResourceMonitor(t *testing.T) {
	m := NewResourceMonitor()
	if m == nil {
		t.Fatal("NewResourceMonitor() returned nil")
	}

	if m.procPath != "/proc" {
		t.Errorf("procPath = %q, want /proc", m.procPath)
	}
}

func TestNewResourceMonitorWithProcPath(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewResourceMonitor(WithProcPath(tmpDir))

	if m.procPath != tmpDir {
		t.Errorf("procPath = %q, want %q", m.procPath, tmpDir)
	}
}

func TestDefaultThresholds(t *testing.T) {
	thresholds := DefaultThresholds()

	if thresholds.FDWarning != 500 {
		t.Errorf("FDWarning = %d, want 500", thresholds.FDWarning)
	}
	if thresholds.FDCritical != 1000 {
		t.Errorf("FDCritical = %d, want 1000", thresholds.FDCritical)
	}
	if thresholds.CPUWarning != 20.0 {
		t.Errorf("CPUWarning = %f, want 20.0", thresholds.CPUWarning)
	}
	if thresholds.CPUCritical != 40.0 {
		t.Errorf("CPUCritical = %f, want 40.0", thresholds.CPUCritical)
	}
	if thresholds.MemoryWarning != 512*1024*1024 {
		t.Errorf("MemoryWarning = %d, want %d", thresholds.MemoryWarning, 512*1024*1024)
	}
	if thresholds.MemoryCritical != 1024*1024*1024 {
		t.Errorf("MemoryCritical = %d, want %d", thresholds.MemoryCritical, 1024*1024*1024)
	}
}

func TestResourceMetricsFields(t *testing.T) {
	metrics := &ResourceMetrics{
		PID:             1234,
		FileDescriptors: 10,
		ThreadCount:     5,
		MemoryBytes:     1024 * 1024,
		CPUPercent:      5.5,
	}

	if metrics.PID != 1234 {
		t.Errorf("PID = %d, want 1234", metrics.PID)
	}
	if metrics.FileDescriptors != 10 {
		t.Errorf("FileDescriptors = %d, want 10", metrics.FileDescriptors)
	}
	if metrics.ThreadCount != 5 {
		t.Errorf("ThreadCount = %d, want 5", metrics.ThreadCount)
	}
	if metrics.MemoryBytes != 1024*1024 {
		t.Errorf("MemoryBytes = %d, want %d", metrics.MemoryBytes, 1024*1024)
	}
	if metrics.CPUPercent != 5.5 {
		t.Errorf("CPUPercent = %f, want 5.5", metrics.CPUPercent)
	}
}

func TestResourceAlertFields(t *testing.T) {
	alert := ResourceAlert{
		Level:    AlertWarning,
		Resource: "file_descriptors",
		Message:  "Too many FDs",
		Value:    500,
	}

	if alert.Level != AlertWarning {
		t.Errorf("Level = %v, want %v", alert.Level, AlertWarning)
	}
	if alert.Resource != "file_descriptors" {
		t.Errorf("Resource = %q, want file_descriptors", alert.Resource)
	}
}

func TestCheckThresholdsNoAlerts(t *testing.T) {
	m := NewResourceMonitor()

	metrics := &ResourceMetrics{
		FileDescriptors: 10,
		CPUPercent:      5.0,
		MemoryBytes:     1024,
	}

	alerts := m.CheckThresholds(metrics)
	if len(alerts) != 0 {
		t.Errorf("Expected no alerts, got %d", len(alerts))
	}
}

func TestCheckThresholdsWithAlerts(t *testing.T) {
	m := NewResourceMonitor()

	// Test FD warning
	metrics := &ResourceMetrics{
		FileDescriptors: 600,
		CPUPercent:      5.0,
		MemoryBytes:     1024,
	}

	alerts := m.CheckThresholds(metrics)
	hasWarning := false
	for _, a := range alerts {
		if a.Level == AlertWarning && a.Resource == "fd" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("Expected FD warning alert")
	}

	// Test FD critical
	metrics.FileDescriptors = 1100
	alerts = m.CheckThresholds(metrics)
	hasCritical := false
	for _, a := range alerts {
		if a.Level == AlertCritical && a.Resource == "fd" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("Expected FD critical alert")
	}
}

func TestCheckThresholdsCPU(t *testing.T) {
	m := NewResourceMonitor()

	// Test CPU warning
	metrics := &ResourceMetrics{
		FileDescriptors: 10,
		CPUPercent:      25.0,
		MemoryBytes:     1024,
	}

	alerts := m.CheckThresholds(metrics)
	hasWarning := false
	for _, a := range alerts {
		if a.Level == AlertWarning && a.Resource == "cpu" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("Expected CPU warning alert")
	}

	// Test CPU critical
	metrics.CPUPercent = 50.0
	alerts = m.CheckThresholds(metrics)
	hasCritical := false
	for _, a := range alerts {
		if a.Level == AlertCritical && a.Resource == "cpu" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("Expected CPU critical alert")
	}
}

func TestCheckThresholdsMemory(t *testing.T) {
	m := NewResourceMonitor()

	// Test memory warning (default threshold is 512MB)
	metrics := &ResourceMetrics{
		FileDescriptors: 10,
		CPUPercent:      5.0,
		MemoryBytes:     600 * 1024 * 1024, // Above 512MB warning
	}

	alerts := m.CheckThresholds(metrics)
	hasWarning := false
	for _, a := range alerts {
		if a.Level == AlertWarning && a.Resource == "memory" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("Expected memory warning alert")
	}

	// Test memory critical (default threshold is 1GB)
	metrics.MemoryBytes = 1100 * 1024 * 1024 // Above 1GB critical
	alerts = m.CheckThresholds(metrics)
	hasCritical := false
	for _, a := range alerts {
		if a.Level == AlertCritical && a.Resource == "memory" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("Expected memory critical alert")
	}
}

func TestWithThresholds(t *testing.T) {
	customThresholds := ResourceThresholds{
		FDWarning:      100,
		FDCritical:     200,
		CPUWarning:     10.0,
		CPUCritical:    20.0,
		MemoryWarning:  10 * 1024 * 1024,
		MemoryCritical: 20 * 1024 * 1024,
	}

	m := NewResourceMonitor(WithThresholds(customThresholds))

	// Check that custom thresholds are applied
	metrics := &ResourceMetrics{
		FileDescriptors: 150, // Between 100 and 200
		CPUPercent:      5.0,
		MemoryBytes:     1024,
	}

	alerts := m.CheckThresholds(metrics)
	hasWarning := false
	for _, a := range alerts {
		if a.Level == AlertWarning && a.Resource == "fd" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("Expected FD warning alert with custom threshold")
	}
}

func TestGetMetricsNonexistentPID(t *testing.T) {
	// Use a mock proc directory
	tmpDir := t.TempDir()
	m := NewResourceMonitor(WithProcPath(tmpDir))

	metrics, err := m.GetMetrics(99999)
	if err == nil {
		t.Error("Expected error for nonexistent PID")
	}
	if metrics != nil {
		t.Error("Expected nil metrics for nonexistent PID")
	}
}

func TestGetMetricsWithMockProc(t *testing.T) {
	// Create mock /proc structure
	tmpDir := t.TempDir()
	pid := 12345
	procDir := filepath.Join(tmpDir, strconv.Itoa(pid))

	// Create fd directory with some entries
	fdDir := filepath.Join(procDir, "fd")
	if err := os.MkdirAll(fdDir, 0755); err != nil {
		t.Fatalf("Failed to create fd dir: %v", err)
	}

	// Create some fake fd symlinks
	for i := 0; i < 5; i++ {
		fdPath := filepath.Join(fdDir, strconv.Itoa(i))
		if err := os.WriteFile(fdPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create fd file: %v", err)
		}
	}

	// Create stat file
	// Format: pid (comm) state ppid pgrp session tty_nr tpgid flags minflt cminflt majflt cmajflt utime stime cutime cstime priority nice num_threads itrealvalue starttime ...
	statContent := "12345 (test) S 1 12345 12345 0 -1 4194304 100 0 0 0 10 5 0 0 20 0 3 0 1000 1000000 100 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte(statContent), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}

	// Create statm file (pages: total shared resident etc)
	statmContent := "1000 500 100 10 0 500 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "statm"), []byte(statmContent), 0644); err != nil {
		t.Fatalf("Failed to create statm file: %v", err)
	}

	m := NewResourceMonitor(WithProcPath(tmpDir))
	metrics, err := m.GetMetrics(pid)

	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}

	if metrics.PID != pid {
		t.Errorf("PID = %d, want %d", metrics.PID, pid)
	}

	if metrics.FileDescriptors != 5 {
		t.Errorf("FileDescriptors = %d, want 5", metrics.FileDescriptors)
	}

	if metrics.ThreadCount != 3 {
		t.Errorf("ThreadCount = %d, want 3", metrics.ThreadCount)
	}
}

func TestClearMetrics(t *testing.T) {
	m := NewResourceMonitor()

	// Add some metrics
	m.mu.Lock()
	m.metrics[1234] = &ResourceMetrics{PID: 1234}
	m.mu.Unlock()

	// Clear them
	m.ClearMetrics(1234)

	// Check they're gone
	m.mu.Lock()
	_, exists := m.metrics[1234]
	m.mu.Unlock()

	if exists {
		t.Error("Expected metrics to be cleared")
	}
}

func TestGetSystemFDLimits(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock file-nr
	sysDir := filepath.Join(tmpDir, "sys", "fs")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatalf("Failed to create sys dir: %v", err)
	}

	// Format: allocated free max
	fileNrContent := "5000\t0\t100000\n"
	if err := os.WriteFile(filepath.Join(sysDir, "file-nr"), []byte(fileNrContent), 0644); err != nil {
		t.Fatalf("Failed to create file-nr: %v", err)
	}

	current, max, err := GetSystemFDLimits(tmpDir)
	if err != nil {
		t.Fatalf("GetSystemFDLimits failed: %v", err)
	}

	if current != 5000 {
		t.Errorf("current = %d, want 5000", current)
	}
	if max != 100000 {
		t.Errorf("max = %d, want 100000", max)
	}
}

func TestAlertLevelString(t *testing.T) {
	if AlertWarning.String() != "WARNING" {
		t.Errorf("AlertWarning.String() = %q, want WARNING", AlertWarning.String())
	}
	if AlertCritical.String() != "CRITICAL" {
		t.Errorf("AlertCritical.String() = %q, want CRITICAL", AlertCritical.String())
	}
	if AlertNone.String() != "OK" {
		t.Errorf("AlertNone.String() = %q, want OK", AlertNone.String())
	}
}

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
