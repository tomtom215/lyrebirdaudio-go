//go:build linux

package stream

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

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
