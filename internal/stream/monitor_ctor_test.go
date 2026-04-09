//go:build linux

package stream

import (
	"io"
	"log/slog"
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
