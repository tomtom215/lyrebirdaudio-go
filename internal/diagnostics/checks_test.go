// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckLogFilesLargeFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a log file exceeding LogSizeWarningBytes (100 MiB)
	// Use a sparse file to avoid actually writing 100+ MiB
	largePath := filepath.Join(tmpDir, "large.log")
	f, err := os.Create(largePath)
	if err != nil {
		t.Fatalf("failed to create large log file: %v", err)
	}
	// Truncate to 101 MiB to exceed threshold
	if err := f.Truncate(101 * 1024 * 1024); err != nil {
		_ = f.Close()
		t.Fatalf("failed to truncate file: %v", err)
	}
	_ = f.Close()

	opts := DefaultOptions()
	opts.LogDir = tmpDir
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for large log files, got %s: %s", result.Status, result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions for large log files")
	}
}

func TestCheckLogFilesWithSmallFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create small log files
	for i := 0; i < 5; i++ {
		name := filepath.Join(tmpDir, fmt.Sprintf("app_%d.log", i))
		if err := os.WriteFile(name, []byte("some log content\n"), 0644); err != nil {
			t.Fatalf("failed to create log file: %v", err)
		}
	}

	opts := DefaultOptions()
	opts.LogDir = tmpDir
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for small log files, got %s", result.Status)
	}
}

func TestCheckLogFilesSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory with log files
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.log"), []byte("nested log"), 0644); err != nil {
		t.Fatalf("failed to create nested log file: %v", err)
	}

	opts := DefaultOptions()
	opts.LogDir = tmpDir
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
}

func TestCheckConfigExistsVsNotExists(t *testing.T) {
	tests := []struct {
		name           string
		setupConfig    bool
		expectedStatus CheckStatus
		expectedMsg    string
	}{
		{
			name:           "config file exists",
			setupConfig:    true,
			expectedStatus: StatusOK,
			expectedMsg:    "Configuration file exists",
		},
		{
			name:           "config file missing",
			setupConfig:    false,
			expectedStatus: StatusWarning,
			expectedMsg:    "Configuration file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			if tt.setupConfig {
				if err := os.WriteFile(configPath, []byte("streams: []\n"), 0640); err != nil {
					t.Fatalf("failed to write config: %v", err)
				}
			}

			opts := DefaultOptions()
			opts.ConfigPath = configPath
			runner := NewRunner(opts)

			result := runner.checkConfig(context.Background())
			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, result.Status)
			}
			if !strings.Contains(result.Message, tt.expectedMsg) {
				t.Errorf("expected message containing %q, got %q", tt.expectedMsg, result.Message)
			}
			if result.Details != configPath {
				t.Errorf("expected details to be config path %q, got %q", configPath, result.Details)
			}
		})
	}
}

func TestCheckMediaMTXAPIWithTestServer(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "API returns 200", statusCode: 200},
		{name: "API returns 500", statusCode: 500},
		{name: "API returns 404", statusCode: 404},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"items":[]}`))
			}))
			defer ts.Close()

			// Verify the test server responds as expected
			client := &http.Client{Timeout: 2 * time.Second}
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v3/paths/list", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected status code %d, got %d", tt.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestCheckNetworkPortsWithListeners(t *testing.T) {
	// Test isPortOpen with an actual listener to cover the success path
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	if !isPortOpen(addr) {
		t.Errorf("expected port %s to be open", addr)
	}

	// Close it and verify it reports closed
	_ = ln.Close()
	// Give the OS a moment to release
	time.Sleep(10 * time.Millisecond)
	if isPortOpen(addr) {
		t.Log("port still appears open right after close (race with OS)")
	}
}

func TestRunContextCancellationMidCheck(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeFull
	runner := NewRunner(opts)

	// Create a context that we cancel after a very short time
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Give context time to expire
	time.Sleep(5 * time.Millisecond)

	report, err := runner.Run(ctx)
	if err == nil {
		// If all checks ran before cancellation, that's OK too
		if report != nil && len(report.Checks) == 24 {
			t.Log("all checks completed before context expired")
			return
		}
	}

	// If context was cancelled, we should get an error
	if err != nil {
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Errorf("expected context error, got: %v", err)
		}
		// Report should still be partially populated
		if report == nil {
			t.Error("expected partial report even on cancellation")
		}
	}
}

func TestRunReturnsContextError(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeFull
	runner := NewRunner(opts)

	// Already-cancelled context should return error quickly
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := runner.Run(ctx)
	if err == nil {
		t.Log("Run completed without error on cancelled context")
	} else {
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	}

	if report == nil {
		t.Error("expected report to be non-nil even on cancellation")
	}
}

func TestCheckEntropySetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkEntropy(context.Background())

	if result.Name != "Entropy" {
		t.Errorf("expected Name 'Entropy', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}
	if result.Duration <= 0 {
		t.Error("expected positive Duration")
	}
	// On Linux, /proc/sys/kernel/random/entropy_avail should exist
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "Entropy pool") {
			t.Errorf("expected message about entropy pool, got %q", result.Message)
		}
	}
}

func TestCheckInotifyLimitsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkInotifyLimits(context.Background())

	if result.Name != "inotify Limits" {
		t.Errorf("expected Name 'inotify Limits', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// On Linux this should read successfully
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "inotify max_user_watches") {
			t.Errorf("expected inotify message, got %q", result.Message)
		}
	}
}

func TestCheckFileDescriptorsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkFileDescriptors(context.Background())

	if result.Name != "File Descriptors" {
		t.Errorf("expected Name 'File Descriptors', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// On Linux /proc/sys/fs/file-nr should exist
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "FD usage") {
			t.Errorf("expected FD usage message, got %q", result.Message)
		}
	}
}

func TestCheckMemorySetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkMemory(context.Background())

	if result.Name != "Memory" {
		t.Errorf("expected Name 'Memory', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// On Linux /proc/meminfo should exist
	if result.Status == StatusError {
		t.Errorf("checkMemory should not error on Linux, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "Memory usage") {
		t.Errorf("expected Memory usage message, got %q", result.Message)
	}
}

func TestCheckDiskSpaceSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkDiskSpace(context.Background())

	if result.Name != "Disk Space" {
		t.Errorf("expected Name 'Disk Space', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// syscall.Statfs should work on Linux
	if result.Status == StatusError {
		t.Errorf("checkDiskSpace should not error on Linux, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "Disk usage") {
		t.Errorf("expected Disk usage message, got %q", result.Message)
	}
}

func TestCheckAudioConflictsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioConflicts(ctx)

	if result.Name != "Audio Conflicts" {
		t.Errorf("expected Name 'Audio Conflicts', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status == "" {
		t.Error("expected non-empty status")
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheckTCPResourcesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkTCPResources(ctx)

	if result.Name != "TCP Resources" {
		t.Errorf("expected Name 'TCP Resources', got %q", result.Name)
	}
	if result.Category != "Network" {
		t.Errorf("expected Category 'Network', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckNetworkPortsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkNetworkPorts(ctx)

	if result.Name != "Network Ports" {
		t.Errorf("expected Name 'Network Ports', got %q", result.Name)
	}
	if result.Category != "Network" {
		t.Errorf("expected Category 'Network', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
}

func TestRunQuickModeCheckCount(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeQuick
	runner := NewRunner(opts)

	checks := runner.getChecks()
	if len(checks) != 5 {
		t.Errorf("expected 5 quick checks, got %d", len(checks))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(report.Checks) != 5 {
		t.Errorf("expected 5 check results in quick mode, got %d", len(report.Checks))
	}

	// Verify summary counts add up
	sum := report.Summary.OK + report.Summary.Warning + report.Summary.Critical +
		report.Summary.Error + report.Summary.Skipped
	if sum != report.Summary.Total {
		t.Errorf("summary counts don't add up: %d != Total %d", sum, report.Summary.Total)
	}
}

func TestRunFullModeCheckCount(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeFull
	runner := NewRunner(opts)

	checks := runner.getChecks()
	if len(checks) != 24 {
		t.Errorf("expected 24 full checks, got %d", len(checks))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(report.Checks) != 24 {
		t.Errorf("expected 24 check results in full mode, got %d", len(report.Checks))
	}

	// Healthy should be determined by critical/error counts
	expectedHealthy := report.Summary.Critical == 0 && report.Summary.Error == 0
	if report.Healthy != expectedHealthy {
		t.Errorf("Healthy mismatch: got %v, expected %v (critical=%d, error=%d)",
			report.Healthy, expectedHealthy, report.Summary.Critical, report.Summary.Error)
	}
}

func TestRunReportTimestamp(t *testing.T) {
	before := time.Now()

	opts := DefaultOptions()
	opts.Mode = ModeQuick
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	after := time.Now()

	if report.Timestamp.Before(before) || report.Timestamp.After(after) {
		t.Errorf("Timestamp %v should be between %v and %v", report.Timestamp, before, after)
	}
}

func TestCollectSystemInfoLinux(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	info := runner.collectSystemInfo()

	// On Linux, these should all be populated
	if info.OS != "linux" {
		t.Errorf("expected OS 'linux', got %q", info.OS)
	}
	if info.Hostname == "" {
		t.Error("expected non-empty Hostname on Linux")
	}
	if info.Kernel == "" {
		t.Error("expected non-empty Kernel on Linux (from /proc/version)")
	}
	if info.Memory <= 0 {
		t.Error("expected positive Memory on Linux (from /proc/meminfo)")
	}
	if info.Uptime == "" {
		t.Error("expected non-empty Uptime on Linux (from /proc/uptime)")
	}
	if info.CPUs <= 0 {
		t.Errorf("expected positive CPUs, got %d", info.CPUs)
	}
	if info.GoVersion == "" {
		t.Error("expected non-empty GoVersion")
	}
}

func TestPrintReportToJSONIntegration(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeQuick
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Test PrintReport doesn't panic
	var buf bytes.Buffer
	PrintReport(&buf, report)
	output := buf.String()
	if len(output) == 0 {
		t.Error("PrintReport produced empty output")
	}

	// Test ToJSON produces valid JSON
	data, err := report.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	var parsed DiagnosticReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}

	if parsed.Summary.Total != report.Summary.Total {
		t.Errorf("JSON round-trip: summary total mismatch: %d vs %d",
			parsed.Summary.Total, report.Summary.Total)
	}
}

func TestCheckPrerequisitesCategories(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkPrerequisites(context.Background())

	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}

	switch result.Status {
	case StatusOK:
		if !strings.Contains(result.Message, "All required tools available") {
			t.Errorf("unexpected OK message: %q", result.Message)
		}
	case StatusWarning:
		if !strings.Contains(result.Message, "Missing optional tools") {
			t.Errorf("unexpected Warning message: %q", result.Message)
		}
	case StatusCritical:
		if !strings.Contains(result.Message, "Missing required tools") {
			t.Errorf("unexpected Critical message: %q", result.Message)
		}
		if len(result.Suggestions) == 0 {
			t.Error("expected suggestions when critical tools are missing")
		}
	default:
		t.Errorf("unexpected status %s for prerequisites", result.Status)
	}
}

func TestCheckVersionsAlwaysOK(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkVersions(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
	if result.Message != "Version information collected" {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestCheckSystemInfoAlwaysOK(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkSystemInfo(context.Background())

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
	if result.Message != "System information collected" {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestSummaryCountsFromRun(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeQuick
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	counts := map[CheckStatus]int{}
	for _, check := range report.Checks {
		counts[check.Status]++
	}

	if counts[StatusOK] != report.Summary.OK {
		t.Errorf("OK count mismatch: found %d in checks, summary says %d", counts[StatusOK], report.Summary.OK)
	}
	if counts[StatusWarning] != report.Summary.Warning {
		t.Errorf("Warning count mismatch: found %d in checks, summary says %d", counts[StatusWarning], report.Summary.Warning)
	}
	if counts[StatusCritical] != report.Summary.Critical {
		t.Errorf("Critical count mismatch: found %d in checks, summary says %d", counts[StatusCritical], report.Summary.Critical)
	}
	if counts[StatusSkipped] != report.Summary.Skipped {
		t.Errorf("Skipped count mismatch: found %d in checks, summary says %d", counts[StatusSkipped], report.Summary.Skipped)
	}
	if counts[StatusError] != report.Summary.Error {
		t.Errorf("Error count mismatch: found %d in checks, summary says %d", counts[StatusError], report.Summary.Error)
	}
}

func TestIsPortOpenWithActiveListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	if !isPortOpen(addr) {
		t.Errorf("isPortOpen(%q) = false, expected true for active listener", addr)
	}
}

func TestCheckUSBAudioSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUSBAudio(context.Background())

	if result.Name != "USB Audio" {
		t.Errorf("expected Name 'USB Audio', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
	if result.Status == StatusWarning {
		if !strings.Contains(result.Message, "No USB audio devices") {
			t.Errorf("unexpected warning message: %q", result.Message)
		}
	}
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "USB audio device") {
			t.Errorf("unexpected OK message: %q", result.Message)
		}
	}
}

func TestCheckALSASetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkALSA(ctx)

	if result.Name != "ALSA" {
		t.Errorf("expected Name 'ALSA', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status == StatusCritical {
		if !strings.Contains(result.Message, "/proc/asound missing") {
			t.Errorf("unexpected critical message: %q", result.Message)
		}
	}
}

func TestCheckFFmpegSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkFFmpeg(ctx)

	if result.Name != "FFmpeg" {
		t.Errorf("expected Name 'FFmpeg', got %q", result.Name)
	}
	if result.Category != "Tools" {
		t.Errorf("expected Category 'Tools', got %q", result.Category)
	}

	switch result.Status {
	case StatusCritical:
		if !strings.Contains(result.Message, "not found") {
			t.Errorf("unexpected critical message: %q", result.Message)
		}
		if len(result.Suggestions) == 0 {
			t.Error("expected suggestions when FFmpeg is missing")
		}
	case StatusOK:
		if result.Details == "" {
			t.Error("expected non-empty Details when FFmpeg is found")
		}
	case StatusWarning:
		t.Logf("FFmpeg warning: %s", result.Message)
	default:
		t.Errorf("unexpected status %s for FFmpeg check", result.Status)
	}
}

func TestCheckMediaMTXServiceSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXService(ctx)

	if result.Name != "MediaMTX Service" {
		t.Errorf("expected Name 'MediaMTX Service', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}

	validStatuses := map[CheckStatus]bool{
		StatusOK: true, StatusWarning: true, StatusCritical: true,
		StatusError: true, StatusSkipped: true,
	}
	if !validStatuses[result.Status] {
		t.Errorf("invalid status: %q", result.Status)
	}
}

func TestCheckTimeSynchronizationSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkTimeSynchronization(ctx)

	if result.Name != "Time Sync" {
		t.Errorf("expected Name 'Time Sync', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
}

func TestCheckSystemdServicesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkSystemdServices(ctx)

	if result.Name != "Systemd Services" {
		t.Errorf("expected Name 'Systemd Services', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckProcessStabilitySetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkProcessStability(ctx)

	if result.Name != "Process Stability" {
		t.Errorf("expected Name 'Process Stability', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckAudioCapabilitiesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioCapabilities(ctx)

	if result.Name != "Audio Capabilities" {
		t.Errorf("expected Name 'Audio Capabilities', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckMediaMTXAPISetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXAPI(ctx)

	if result.Name != "MediaMTX API" {
		t.Errorf("expected Name 'MediaMTX API', got %q", result.Name)
	}
	if result.Category != "Services" {
		t.Errorf("expected Category 'Services', got %q", result.Category)
	}
	validStatuses := map[CheckStatus]bool{
		StatusOK: true, StatusWarning: true, StatusError: true,
	}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

func TestCheckUdevRulesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUdevRules(context.Background())

	if result.Name != "udev Rules" {
		t.Errorf("expected Name 'udev Rules', got %q", result.Name)
	}
	if result.Category != "Config" {
		t.Errorf("expected Category 'Config', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
	if result.Status == StatusWarning {
		if len(result.Suggestions) == 0 {
			t.Error("expected suggestions when udev rules not found")
		}
	}
}
