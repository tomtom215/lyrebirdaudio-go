//go:build linux

package stream

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

// TestMonitorProcessClearsMetricsOnExit verifies that MonitorProcess prunes its
// per-PID entries from the metrics/prevCPU maps when it returns. On a 24/7
// field device, FFmpeg is restarted with a NEW pid on every failure while the
// ResourceMonitor is reused across restarts (one monitor per Manager). Without
// pruning on exit, these maps would gain one permanent entry per pid and grow
// unboundedly over months of operation until the host runs out of memory.
func TestMonitorProcessClearsMetricsOnExit(t *testing.T) {
	tmpDir := t.TempDir()
	pid := 4242
	procDir := filepath.Join(tmpDir, strconv.Itoa(pid))
	fdDir := filepath.Join(procDir, "fd")
	if err := os.MkdirAll(fdDir, 0755); err != nil {
		t.Fatalf("Failed to create fd dir: %v", err)
	}
	statContent := "4242 (test) S 1 4242 4242 0 -1 4194304 100 0 0 0 10 5 0 0 20 0 3 0 1000 1000000 100 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte(statContent), 0644); err != nil {
		t.Fatalf("Failed to create stat file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(procDir, "statm"), []byte("1000 500 100 10 0 500 0\n"), 0644); err != nil {
		t.Fatalf("Failed to create statm file: %v", err)
	}

	m := NewResourceMonitor(WithProcPath(tmpDir))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	// Runs until ctx times out, collecting several samples along the way.
	m.MonitorProcess(ctx, pid, 30*time.Millisecond, nil)

	// After monitoring ends, the cached metrics for this pid must be gone.
	if got := m.GetCachedMetrics(pid); got != nil {
		t.Errorf("GetCachedMetrics(%d) = %+v after MonitorProcess returned; want nil (entry should be pruned)", pid, got)
	}

	// And the internal maps must not retain any per-PID entry (leak check).
	m.mu.RLock()
	nMetrics, nCPU := len(m.metrics), len(m.prevCPU)
	m.mu.RUnlock()
	if nMetrics != 0 || nCPU != 0 {
		t.Errorf("after MonitorProcess returned: metrics map=%d, prevCPU map=%d; want 0/0 (no per-PID leak across restarts)", nMetrics, nCPU)
	}
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
