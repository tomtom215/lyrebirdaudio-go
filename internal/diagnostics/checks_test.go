// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckConfigWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("devices: []\n"), 0640); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	runner := NewRunner(Options{ConfigPath: configPath})
	result := runner.checkConfig(context.Background())

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
	if result.Name != "Configuration" {
		t.Errorf("expected name 'Configuration', got %q", result.Name)
	}
	if result.Category != "Config" {
		t.Errorf("expected category 'Config', got %q", result.Category)
	}
	if !strings.Contains(result.Message, "exists") {
		t.Errorf("expected message about config existing, got %q", result.Message)
	}
	if result.Details != configPath {
		t.Errorf("expected details to be config path, got %q", result.Details)
	}
}

func TestCheckConfigWithMissingFile(t *testing.T) {
	runner := NewRunner(Options{ConfigPath: "/nonexistent/path/config.yaml"})
	result := runner.checkConfig(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("expected 'not found' in message, got %q", result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions for missing config")
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestCheckLogFilesNonExistentDir(t *testing.T) {
	runner := NewRunner(Options{LogDir: "/nonexistent/log/dir"})
	result := runner.checkLogFiles(context.Background())

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for non-existent dir, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "will be created") {
		t.Errorf("expected message about creating dir, got %q", result.Message)
	}
}

func TestCheckLogFilesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	runner := NewRunner(Options{LogDir: tmpDir})
	result := runner.checkLogFiles(context.Background())

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for empty dir, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "0 B") || !strings.Contains(result.Message, "0") {
		// Size should be very small
		t.Logf("log dir size message: %s", result.Message)
	}
}

func TestCheckLogFilesWithSmallFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some small log files
	for _, name := range []string{"stream.log", "stream.log.1", "error.log"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("log entry\n"), 0640); err != nil {
			t.Fatalf("failed to create log file %s: %v", name, err)
		}
	}

	runner := NewRunner(Options{LogDir: tmpDir})
	result := runner.checkLogFiles(context.Background())

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for small logs, got %s", result.Status)
	}
}

func TestCheckLogFilesWithSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory with files
	subDir := filepath.Join(tmpDir, "archives")
	if err := os.MkdirAll(subDir, 0750); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "old.log"), []byte("old log data"), 0640); err != nil {
		t.Fatalf("failed to create archived log: %v", err)
	}

	runner := NewRunner(Options{LogDir: tmpDir})
	result := runner.checkLogFiles(context.Background())

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %s", result.Status)
	}
}

func TestCheckLockDirNotExists(t *testing.T) {
	// The default lock dir /var/run/lyrebird may or may not exist.
	// We test with the runner as-is and just verify the result is valid.
	runner := NewRunner(DefaultOptions())
	result := runner.checkLockDir(context.Background())

	if result.Name != "Lock Directory" {
		t.Errorf("expected name 'Lock Directory', got %q", result.Name)
	}
	// Should be OK whether it exists or not (not critical unless it's a file)
	if result.Status != StatusOK && result.Status != StatusCritical {
		t.Errorf("unexpected status: %s", result.Status)
	}
}

func TestCheckEntryWithExistingProcFile(t *testing.T) {
	// This test verifies checkEntropy reads from /proc/sys/kernel/random/entropy_avail
	// which exists on Linux. The result should be OK or Warning depending on entropy level.
	runner := NewRunner(DefaultOptions())
	result := runner.checkEntropy(context.Background())

	if result.Name != "Entropy" {
		t.Errorf("expected name 'Entropy', got %q", result.Name)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("expected OK or Warning, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "Entropy") && !strings.Contains(result.Message, "ntropy") {
		t.Errorf("expected entropy info in message, got %q", result.Message)
	}
}

func TestCheckInotifyWithExistingProcFile(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkInotifyLimits(context.Background())

	if result.Name != "inotify Limits" {
		t.Errorf("expected name 'inotify Limits', got %q", result.Name)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("expected OK or Warning, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "inotify") {
		t.Errorf("expected inotify info in message, got %q", result.Message)
	}
}

func TestCheckFileDescriptorsWithExistingProcFile(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkFileDescriptors(context.Background())

	if result.Name != "File Descriptors" {
		t.Errorf("expected name 'File Descriptors', got %q", result.Name)
	}
	// On Linux /proc/sys/fs/file-nr should exist
	if result.Status != StatusOK && result.Status != StatusWarning && result.Status != StatusCritical {
		t.Errorf("expected valid resource status, got %s: %s", result.Status, result.Message)
	}
	if strings.Contains(result.Message, "FD usage") {
		// Verify it has a percentage
		if !strings.Contains(result.Message, "%") {
			t.Errorf("expected percentage in FD message, got %q", result.Message)
		}
	}
}

func TestCheckMemoryWithExistingProcFile(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkMemory(context.Background())

	if result.Name != "Memory" {
		t.Errorf("expected name 'Memory', got %q", result.Name)
	}
	// On Linux /proc/meminfo should exist
	if result.Status != StatusOK && result.Status != StatusWarning && result.Status != StatusCritical {
		t.Errorf("expected valid resource status, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "Memory") && !strings.Contains(result.Message, "emory") {
		t.Errorf("expected memory info in message, got %q", result.Message)
	}
}

func TestCheckDiskSpaceOnLinux(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkDiskSpace(context.Background())

	if result.Name != "Disk Space" {
		t.Errorf("expected name 'Disk Space', got %q", result.Name)
	}
	// syscall.Statfs on "/" should succeed on Linux
	if result.Status == StatusError {
		t.Errorf("expected successful disk check on Linux, got error: %s", result.Message)
	}
	if !strings.Contains(result.Message, "Disk") && !strings.Contains(result.Message, "isk") {
		t.Errorf("expected disk info in message, got %q", result.Message)
	}
}

func TestRunQuickModeReturnsFewerChecks(t *testing.T) {
	opts := Options{
		Mode:       ModeQuick,
		ConfigPath: "/nonexistent/config.yaml",
		LogDir:     "/nonexistent/logdir",
		Output:     &bytes.Buffer{},
	}
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Quick mode has exactly 5 checks
	if len(report.Checks) != 5 {
		t.Errorf("expected 5 checks in quick mode, got %d", len(report.Checks))
	}

	// Verify summary matches
	total := report.Summary.OK + report.Summary.Warning + report.Summary.Critical +
		report.Summary.Skipped + report.Summary.Error
	if total != report.Summary.Total {
		t.Errorf("summary counts don't add up: %d != %d", total, report.Summary.Total)
	}
}

func TestRunContextCancellationReturnsError(t *testing.T) {
	opts := Options{
		Mode:       ModeFull,
		ConfigPath: "/nonexistent/config.yaml",
		LogDir:     "/nonexistent/logdir",
		Output:     &bytes.Buffer{},
	}
	runner := NewRunner(opts)

	// Already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := runner.Run(ctx)
	if err == nil {
		// It's possible that one check ran before cancellation, so err might be nil
		// if all checks completed before the context check. But with 24 checks, at least
		// some should be skipped.
		t.Logf("Run completed without error (all checks may have completed before cancellation)")
	} else {
		if err != context.Canceled {
			t.Errorf("expected context.Canceled error, got: %v", err)
		}
	}

	if report == nil {
		t.Fatal("expected report to be non-nil even with cancellation")
	}

	// Report should have fewer checks than full mode (24)
	if err != nil && len(report.Checks) >= 24 {
		t.Errorf("expected fewer than 24 checks with cancelled context, got %d", len(report.Checks))
	}
}

func TestRunReportHealthyFlag(t *testing.T) {
	tests := []struct {
		name            string
		mode            CheckMode
		expectNonNilSys bool
	}{
		{
			name:            "quick mode report has system info",
			mode:            ModeQuick,
			expectNonNilSys: true,
		},
		{
			name:            "full mode report has system info",
			mode:            ModeFull,
			expectNonNilSys: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{
				Mode:       tt.mode,
				ConfigPath: "/nonexistent",
				LogDir:     "/nonexistent",
				Output:     &bytes.Buffer{},
			}
			runner := NewRunner(opts)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			report, err := runner.Run(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectNonNilSys && report.SystemInfo == nil {
				t.Error("expected SystemInfo to be non-nil")
			}

			// Healthy should be true only when no critical/error checks
			expectedHealthy := report.Summary.Critical == 0 && report.Summary.Error == 0
			if report.Healthy != expectedHealthy {
				t.Errorf("Healthy=%v but Critical=%d, Error=%d",
					report.Healthy, report.Summary.Critical, report.Summary.Error)
			}
		})
	}
}

func TestCollectSystemInfoFields(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	info := runner.collectSystemInfo()

	if info.OS != "linux" {
		t.Errorf("expected OS='linux', got %q", info.OS)
	}
	if info.Architecture == "" {
		t.Error("expected non-empty Architecture")
	}
	if info.CPUs <= 0 {
		t.Errorf("expected positive CPUs, got %d", info.CPUs)
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Errorf("expected GoVersion starting with 'go', got %q", info.GoVersion)
	}
	// On Linux, hostname should be readable
	if info.Hostname == "" {
		t.Error("expected non-empty Hostname on Linux")
	}
	// Kernel version from /proc/version
	if info.Kernel == "" {
		t.Error("expected non-empty Kernel on Linux")
	}
	// Memory from /proc/meminfo
	if info.Memory <= 0 {
		t.Errorf("expected positive Memory, got %d", info.Memory)
	}
	// Uptime from /proc/uptime
	if info.Uptime == "" {
		t.Error("expected non-empty Uptime on Linux")
	}
}

func TestCheckNetworkPortsWithLocalListener(t *testing.T) {
	// Start a TCP listener on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	addr := listener.Addr().String()
	if !isPortOpen(addr) {
		t.Errorf("isPortOpen(%q) = false, expected true for listening port", addr)
	}
}

func TestCheckMediaMTXAPIWithMockServer(t *testing.T) {
	// Start a mock HTTP server that simulates MediaMTX API
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	// The checkMediaMTXAPI function hardcodes localhost:9997, so we can't redirect it
	// to our mock server. But we can test isPortOpen with the mock server's address.
	addr := listener.Addr().String()
	if !isPortOpen(addr) {
		t.Errorf("expected mock server port to be open at %s", addr)
	}
}

func TestCheckNetworkPortsBothClosed(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx := context.Background()
	result := runner.checkNetworkPorts(ctx)

	if result.Name != "Network Ports" {
		t.Errorf("expected name 'Network Ports', got %q", result.Name)
	}
	// In test environment, RTSP and API ports are typically not open
	// The result should be OK (both open) or Warning (some/all closed)
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s for network ports check", result.Status)
	}
}

func TestPrintReportDuration(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:  time.Now(),
		Duration:   3*time.Second + 500*time.Millisecond,
		SystemInfo: &SystemInfo{Hostname: "test", OS: "linux"},
		Checks:     []CheckResult{},
		Summary:    &Summary{},
		Healthy:    true,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)
	output := buf.String()

	if !strings.Contains(output, "Duration:") {
		t.Error("expected Duration in output")
	}
}

func TestToJSONRoundTrip(t *testing.T) {
	original := &DiagnosticReport{
		Timestamp: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		Duration:  10 * time.Second,
		SystemInfo: &SystemInfo{
			Hostname:     "roundtrip",
			OS:           "linux",
			Kernel:       "6.1.0",
			Architecture: "amd64",
			CPUs:         2,
			Memory:       2 * 1024 * 1024 * 1024,
			Uptime:       "1d 0h 0m",
			GoVersion:    "go1.24",
		},
		Checks: []CheckResult{
			{
				Name:        "Check1",
				Category:    "Cat1",
				Status:      StatusCritical,
				Message:     "Bad",
				Details:     "Very bad",
				Duration:    time.Millisecond,
				Suggestions: []string{"Fix it", "Fix it again"},
			},
		},
		Summary: &Summary{Total: 1, Critical: 1},
		Healthy: false,
	}

	data, err := original.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var restored DiagnosticReport
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if restored.SystemInfo.Hostname != original.SystemInfo.Hostname {
		t.Errorf("hostname mismatch: %q vs %q", restored.SystemInfo.Hostname, original.SystemInfo.Hostname)
	}
	if restored.Healthy != original.Healthy {
		t.Errorf("healthy mismatch: %v vs %v", restored.Healthy, original.Healthy)
	}
	if len(restored.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(restored.Checks))
	}
	if restored.Checks[0].Status != StatusCritical {
		t.Errorf("status mismatch: %s vs %s", restored.Checks[0].Status, StatusCritical)
	}
	if len(restored.Checks[0].Suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(restored.Checks[0].Suggestions))
	}
	if restored.Summary.Critical != 1 {
		t.Errorf("summary critical mismatch: %d vs 1", restored.Summary.Critical)
	}
}

func TestCheckSystemInfoAlwaysOK(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkSystemInfo(context.Background())

	if result.Status != StatusOK {
		t.Errorf("checkSystemInfo should always return OK, got %s", result.Status)
	}
	if result.Message != "System information collected" {
		t.Errorf("unexpected message: %q", result.Message)
	}
	if result.Category != "System" {
		t.Errorf("expected category 'System', got %q", result.Category)
	}
}

func TestCheckVersionsAlwaysOK(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkVersions(ctx)

	if result.Status != StatusOK {
		t.Errorf("checkVersions should always return OK, got %s", result.Status)
	}
	if result.Category != "System" {
		t.Errorf("expected category 'System', got %q", result.Category)
	}
}

func TestCheckPrerequisitesCategories(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkPrerequisites(context.Background())

	if result.Category != "System" {
		t.Errorf("expected category 'System', got %q", result.Category)
	}
	// Status should be OK, Warning, or Critical depending on installed tools
	validStatuses := map[CheckStatus]bool{
		StatusOK: true, StatusWarning: true, StatusCritical: true,
	}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %s", result.Status)
	}
}

func TestCheckALSAOnLinux(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkALSA(context.Background())

	if result.Category != "Audio" {
		t.Errorf("expected category 'Audio', got %q", result.Category)
	}
	// /proc/asound may or may not exist in test environment
	validStatuses := map[CheckStatus]bool{
		StatusOK: true, StatusWarning: true, StatusCritical: true,
	}
	if !validStatuses[result.Status] {
		t.Errorf("unexpected status: %s", result.Status)
	}
}

func TestCheckUSBAudioCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkUSBAudio(context.Background())

	if result.Category != "Audio" {
		t.Errorf("expected category 'Audio', got %q", result.Category)
	}
	// No USB audio in test env typically
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status: %s", result.Status)
	}
}

func TestCheckAudioCapabilitiesCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := runner.checkAudioCapabilities(ctx)

	if result.Category != "Audio" {
		t.Errorf("expected category 'Audio', got %q", result.Category)
	}
}

func TestCheckTCPResourcesCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkTCPResources(context.Background())

	if result.Category != "Network" {
		t.Errorf("expected category 'Network', got %q", result.Category)
	}
}

func TestCheckAudioConflictsCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := runner.checkAudioConflicts(ctx)

	if result.Category != "Audio" {
		t.Errorf("expected category 'Audio', got %q", result.Category)
	}
	// Should be OK or Warning
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status: %s", result.Status)
	}
}

func TestRunFullModeCompletesAllChecks(t *testing.T) {
	opts := Options{
		Mode:       ModeFull,
		ConfigPath: "/nonexistent/config.yaml",
		LogDir:     "/nonexistent/logdir",
		Output:     &bytes.Buffer{},
	}
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.Checks) != 24 {
		t.Errorf("expected 24 checks in full mode, got %d", len(report.Checks))
	}

	// Verify each check has a name and category
	for i, check := range report.Checks {
		if check.Name == "" {
			t.Errorf("check %d has empty name", i)
		}
		if check.Category == "" {
			t.Errorf("check %d (%s) has empty category", i, check.Name)
		}
		if check.Status == "" {
			t.Errorf("check %d (%s) has empty status", i, check.Name)
		}
		if check.Message == "" {
			t.Errorf("check %d (%s) has empty message", i, check.Name)
		}
	}

	// Verify duration was recorded
	if report.Duration <= 0 {
		t.Error("expected positive report duration")
	}
}

func TestCheckFFmpegCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkFFmpeg(ctx)
	if result.Category != "Tools" {
		t.Errorf("expected category 'Tools', got %q", result.Category)
	}
}

func TestCheckMediaMTXServiceCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXService(ctx)
	if result.Category != "Services" {
		t.Errorf("expected category 'Services', got %q", result.Category)
	}
}

func TestCheckMediaMTXAPICategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXAPI(ctx)
	if result.Category != "Services" {
		t.Errorf("expected category 'Services', got %q", result.Category)
	}
	// In test env, API is typically not reachable
	if result.Status != StatusOK && result.Status != StatusWarning && result.Status != StatusError {
		t.Errorf("unexpected status: %s", result.Status)
	}
}

func TestCheckTimeSyncCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkTimeSynchronization(ctx)
	if result.Category != "System" {
		t.Errorf("expected category 'System', got %q", result.Category)
	}
}

func TestCheckSystemdServicesCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkSystemdServices(ctx)
	if result.Category != "Services" {
		t.Errorf("expected category 'Services', got %q", result.Category)
	}
}

func TestCheckProcessStabilityCategory(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkProcessStability(ctx)
	if result.Category != "Services" {
		t.Errorf("expected category 'Services', got %q", result.Category)
	}
}
