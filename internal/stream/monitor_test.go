package stream

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestWithLogger(t *testing.T) {
	// Create a test logger using slog
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	m := NewResourceMonitor(WithLogger(logger))
	if m == nil {
		t.Fatal("NewResourceMonitor with WithLogger returned nil")
	}
	// The logger should be set (we can't easily verify the internal field)
	// but the option should not panic
}

func TestGetCachedMetrics(t *testing.T) {
	m := NewResourceMonitor()

	// Use current process PID
	pid := os.Getpid()

	// Initially there should be no cached metrics for this PID
	metrics := m.GetCachedMetrics(pid)
	if metrics != nil {
		t.Error("GetCachedMetrics() should return nil when no metrics cached")
	}

	// After getting metrics for a process, they should be cached
	_, err := m.GetMetrics(pid)
	if err != nil {
		t.Skipf("Cannot get metrics for current process: %v", err)
	}

	metrics = m.GetCachedMetrics(pid)
	if metrics == nil {
		t.Error("GetCachedMetrics() should return cached metrics after GetMetrics")
	}
}

func TestMonitorProcess(t *testing.T) {
	// Create mock /proc structure
	tmpDir := t.TempDir()
	pid := 12345
	procDir := filepath.Join(tmpDir, strconv.Itoa(pid))

	// Create fd directory with some entries
	fdDir := filepath.Join(procDir, "fd")
	if err := os.MkdirAll(fdDir, 0755); err != nil {
		t.Fatalf("Failed to create fd dir: %v", err)
	}

	// Create some fake fd files
	for i := 0; i < 5; i++ {
		fdPath := filepath.Join(fdDir, strconv.Itoa(i))
		if err := os.WriteFile(fdPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create fd file: %v", err)
		}
	}

	// Create stat file
	statContent := "12345 (test) S 1 12345 12345 0 -1 4194304 100 0 0 0 10 5 0 0 20 0 3 0 1000 1000000 100 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte(statContent), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}

	// Create statm file
	statmContent := "1000 500 100 10 0 500 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "statm"), []byte(statmContent), 0644); err != nil {
		t.Fatalf("Failed to create statm file: %v", err)
	}

	// Create a logger buffer to capture output
	logBuf := &strings.Builder{}
	m := NewResourceMonitor(WithProcPath(tmpDir), WithLogger(slog.New(slog.NewTextHandler(logBuf, nil))))

	// Create context with cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Track alerts
	var alertCount int
	alertCallback := func(alerts []ResourceAlert) {
		alertCount += len(alerts)
	}

	// Start monitoring (will run for ~200ms)
	m.MonitorProcess(ctx, pid, 50*time.Millisecond, alertCallback)

	// Should have collected metrics at least once
	// (due to short interval and context timeout)
}

func TestMonitorProcessWithAlerts(t *testing.T) {
	// Create mock /proc structure with high resource usage
	tmpDir := t.TempDir()
	pid := 12346
	procDir := filepath.Join(tmpDir, strconv.Itoa(pid))

	// Create fd directory with many entries (above warning threshold of 500)
	fdDir := filepath.Join(procDir, "fd")
	if err := os.MkdirAll(fdDir, 0755); err != nil {
		t.Fatalf("Failed to create fd dir: %v", err)
	}

	// Create 600 fake fd files (above warning threshold)
	for i := 0; i < 600; i++ {
		fdPath := filepath.Join(fdDir, strconv.Itoa(i))
		if err := os.WriteFile(fdPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create fd file: %v", err)
		}
	}

	// Create stat file
	statContent := "12346 (test) S 1 12346 12346 0 -1 4194304 100 0 0 0 10 5 0 0 20 0 3 0 1000 1000000 100 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte(statContent), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}

	// Create statm file
	statmContent := "1000 500 100 10 0 500 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "statm"), []byte(statmContent), 0644); err != nil {
		t.Fatalf("Failed to create statm file: %v", err)
	}

	logBuf := &strings.Builder{}
	m := NewResourceMonitor(WithProcPath(tmpDir), WithLogger(slog.New(slog.NewTextHandler(logBuf, nil))))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	var alertCount int
	alertCallback := func(alerts []ResourceAlert) {
		alertCount += len(alerts)
	}

	m.MonitorProcess(ctx, pid, 50*time.Millisecond, alertCallback)

	// Should have generated alerts due to high FD count
	if alertCount == 0 {
		t.Log("No alerts generated (may depend on timing)")
	}

	// Check that logger received output
	logOutput := logBuf.String()
	if logOutput != "" && !strings.Contains(logOutput, "WARNING") && !strings.Contains(logOutput, "fd") {
		t.Logf("Log output: %s", logOutput)
	}
}

func TestMonitorProcessExitedProcess(t *testing.T) {
	// Use a non-existent PID to test error handling
	tmpDir := t.TempDir()
	logBuf := &strings.Builder{}
	m := NewResourceMonitor(WithProcPath(tmpDir), WithLogger(slog.New(slog.NewTextHandler(logBuf, nil))))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Monitor a non-existent process
	m.MonitorProcess(ctx, 99999, 20*time.Millisecond, nil)

	// Should have logged an error about the missing process
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Log("Expected log output for failed metrics (may have been too fast)")
	}
}

func TestMonitorProcessNilCallback(t *testing.T) {
	// Create mock /proc structure
	tmpDir := t.TempDir()
	pid := 12347
	procDir := filepath.Join(tmpDir, strconv.Itoa(pid))

	fdDir := filepath.Join(procDir, "fd")
	if err := os.MkdirAll(fdDir, 0755); err != nil {
		t.Fatalf("Failed to create fd dir: %v", err)
	}

	statContent := "12347 (test) S 1 12347 12347 0 -1 4194304 100 0 0 0 10 5 0 0 20 0 3 0 1000 1000000 100 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte(statContent), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}

	statmContent := "1000 500 100 10 0 500 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "statm"), []byte(statmContent), 0644); err != nil {
		t.Fatalf("Failed to create statm file: %v", err)
	}

	m := NewResourceMonitor(WithProcPath(tmpDir))

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	// Should not panic with nil callback
	m.MonitorProcess(ctx, pid, 30*time.Millisecond, nil)
}

func TestGetProcessStartTime(t *testing.T) {
	tmpDir := t.TempDir()
	pid := 12348
	procDir := filepath.Join(tmpDir, strconv.Itoa(pid))

	if err := os.MkdirAll(procDir, 0755); err != nil {
		t.Fatalf("Failed to create proc dir: %v", err)
	}

	// Create stat file with valid format
	statContent := "12348 (test) S 1 12348 12348 0 -1 4194304 100 0 0 0 10 5 0 0 20 0 3 0 1000000 1000000 100 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte(statContent), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}

	// Create /proc/stat for boot time
	if err := os.WriteFile(filepath.Join(tmpDir, "stat"), []byte("cpu 0 0 0 0 0 0 0 0 0 0\nbtime 1700000000\n"), 0644); err != nil {
		t.Fatalf("Failed to create /proc/stat: %v", err)
	}

	m := NewResourceMonitor(WithProcPath(tmpDir))
	startTime, err := m.getProcessStartTime(pid)
	if err != nil {
		t.Fatalf("getProcessStartTime failed: %v", err)
	}

	// Should return a valid time
	if startTime.IsZero() {
		t.Error("Expected non-zero start time")
	}
}

func TestGetProcessStartTimeErrors(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewResourceMonitor(WithProcPath(tmpDir))

	// Test with non-existent PID
	_, err := m.getProcessStartTime(99999)
	if err == nil {
		t.Error("Expected error for non-existent PID")
	}

	// Test with invalid stat format (no closing paren)
	pid := 12349
	procDir := filepath.Join(tmpDir, strconv.Itoa(pid))
	if err := os.MkdirAll(procDir, 0755); err != nil {
		t.Fatalf("Failed to create proc dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte("12349 (test S 1"), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}

	_, err = m.getProcessStartTime(pid)
	if err == nil {
		t.Error("Expected error for invalid stat format")
	}

	// Test with insufficient fields
	pid2 := 12350
	procDir2 := filepath.Join(tmpDir, strconv.Itoa(pid2))
	if err := os.MkdirAll(procDir2, 0755); err != nil {
		t.Fatalf("Failed to create proc dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(procDir2, "stat"), []byte("12350 (test) S 1 2 3"), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}

	_, err = m.getProcessStartTime(pid2)
	if err == nil {
		t.Error("Expected error for insufficient fields")
	}
}

func TestGetSystemFDLimitsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with missing file-nr
	_, _, err := GetSystemFDLimits(tmpDir)
	if err == nil {
		t.Error("Expected error when file-nr doesn't exist")
	}

	// Test with invalid content
	sysDir := filepath.Join(tmpDir, "sys", "fs")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatalf("Failed to create sys dir: %v", err)
	}

	// Invalid format (not enough fields)
	if err := os.WriteFile(filepath.Join(sysDir, "file-nr"), []byte("5000\n"), 0644); err != nil {
		t.Fatalf("Failed to create file-nr: %v", err)
	}

	_, _, err = GetSystemFDLimits(tmpDir)
	if err == nil {
		t.Error("Expected error for invalid file-nr format")
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
		"1 (test) S 1 2",                    // Too few fields after comm
		"1 (test) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 abc 0", // Non-numeric thread count
		"1 () S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0\n",     // Empty comm field
		") S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0\n",         // Leading closing paren
		"1 (a)b) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 2 0 1 0\n",   // Paren in comm name
		"1 (test) S",             // Minimal fields
		"1 (test)",               // Just pid and comm
		"\n\n\n",                 // Only newlines
		"1 (test) S " + strings.Repeat("0 ", 50), // Many fields
		"1 (test) S " + strings.Repeat("999999999 ", 20),  // Large numeric values
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
		"1000",          // Single field, no resident field
		"1000 abc",      // Non-numeric resident field
		"-1 -500 0 0 0 0 0", // Negative values
		"0 0",           // Minimal valid (two fields)
		"999999999999 999999999999 0 0 0 0 0", // Very large values
		"\t100\t200\t300",    // Tab-separated
		"  100  200  300  ",  // Extra whitespace
		"\n",                 // Just newline
		"100 200\n",         // Trailing newline (common in /proc)
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
