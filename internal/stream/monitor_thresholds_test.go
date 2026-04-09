//go:build linux

package stream

import (
	"testing"
)

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
