package diagnostics

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Mode != ModeFull {
		t.Errorf("expected Mode to be %q, got %q", ModeFull, opts.Mode)
	}
	if opts.ConfigPath != "/etc/lyrebird/config.yaml" {
		t.Errorf("expected ConfigPath to be /etc/lyrebird/config.yaml, got %q", opts.ConfigPath)
	}
	if opts.LogDir != "/var/log/lyrebird" {
		t.Errorf("expected LogDir to be /var/log/lyrebird, got %q", opts.LogDir)
	}
	if opts.Output == nil {
		t.Error("expected Output to be os.Stdout by default")
	}
}

func TestNewRunner(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	if runner == nil {
		t.Fatal("expected runner to be non-nil")
	}
	if runner.opts.Mode != opts.Mode {
		t.Errorf("expected Mode to be %q, got %q", opts.Mode, runner.opts.Mode)
	}
}

func TestCheckStatus(t *testing.T) {
	tests := []struct {
		status   CheckStatus
		expected string
	}{
		{StatusOK, "OK"},
		{StatusWarning, "WARNING"},
		{StatusCritical, "CRITICAL"},
		{StatusSkipped, "SKIPPED"},
		{StatusError, "ERROR"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, string(tt.status))
		}
	}
}

func TestCheckMode(t *testing.T) {
	tests := []struct {
		mode     CheckMode
		expected string
	}{
		{ModeQuick, "quick"},
		{ModeFull, "full"},
		{ModeDebug, "debug"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, string(tt.mode))
		}
	}
}

func TestSummaryCalculation(t *testing.T) {
	results := []CheckResult{
		{Status: StatusOK},
		{Status: StatusOK},
		{Status: StatusWarning},
		{Status: StatusCritical},
		{Status: StatusSkipped},
		{Status: StatusError},
	}

	summary := &Summary{}
	summary.Total = len(results)
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			summary.OK++
		case StatusWarning:
			summary.Warning++
		case StatusCritical:
			summary.Critical++
		case StatusSkipped:
			summary.Skipped++
		case StatusError:
			summary.Error++
		}
	}

	if summary.Total != 6 {
		t.Errorf("expected Total to be 6, got %d", summary.Total)
	}
	if summary.OK != 2 {
		t.Errorf("expected OK to be 2, got %d", summary.OK)
	}
	if summary.Warning != 1 {
		t.Errorf("expected Warning to be 1, got %d", summary.Warning)
	}
	if summary.Critical != 1 {
		t.Errorf("expected Critical to be 1, got %d", summary.Critical)
	}
	if summary.Skipped != 1 {
		t.Errorf("expected Skipped to be 1, got %d", summary.Skipped)
	}
	if summary.Error != 1 {
		t.Errorf("expected Error to be 1, got %d", summary.Error)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1536, "1.5 KiB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, expected %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestIsPortOpen(t *testing.T) {
	// Test with invalid address
	if isPortOpen("invalid:address:999") {
		t.Error("expected isPortOpen to return false for invalid address")
	}

	// Test with non-routable address (should timeout/fail)
	if isPortOpen("192.0.2.1:9999") {
		t.Error("expected isPortOpen to return false for non-routable address")
	}
}

func TestCollectSystemInfo(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	info := runner.collectSystemInfo()

	if info == nil {
		t.Fatal("expected system info to be non-nil")
	}

	if info.OS == "" {
		t.Error("expected OS to be non-empty")
	}

	if info.Architecture == "" {
		t.Error("expected Architecture to be non-empty")
	}

	if info.CPUs <= 0 {
		t.Error("expected CPUs to be positive")
	}

	if info.GoVersion == "" {
		t.Error("expected GoVersion to be non-empty")
	}
}

func TestRunQuickMode(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeQuick
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report == nil {
		t.Fatal("expected report to be non-nil")
	}

	if report.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}

	if report.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}

	if report.SystemInfo == nil {
		t.Error("expected SystemInfo to be non-nil")
	}

	if report.Summary == nil {
		t.Error("expected Summary to be non-nil")
	}

	if len(report.Checks) == 0 {
		t.Error("expected at least one check result")
	}

	// Verify summary matches checks
	if report.Summary.Total != len(report.Checks) {
		t.Errorf("expected Summary.Total (%d) to match len(Checks) (%d)",
			report.Summary.Total, len(report.Checks))
	}
}

func TestPrintReport(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname:     "test-host",
			OS:           "linux",
			Kernel:       "5.4.0",
			Architecture: "amd64",
			CPUs:         4,
			Memory:       8 * 1024 * 1024 * 1024,
			GoVersion:    "go1.23",
		},
		Checks: []CheckResult{
			{
				Name:     "Test Check",
				Category: "Test",
				Status:   StatusOK,
				Message:  "All good",
				Duration: 100 * time.Millisecond,
			},
			{
				Name:        "Warning Check",
				Category:    "Test",
				Status:      StatusWarning,
				Message:     "Something to look at",
				Duration:    50 * time.Millisecond,
				Suggestions: []string{"Fix this", "Fix that"},
			},
		},
		Summary: &Summary{
			Total:   2,
			OK:      1,
			Warning: 1,
		},
		Healthy: true,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)

	output := buf.String()

	// Check that key elements are present
	if !strings.Contains(output, "LyreBirdAudio Diagnostics Report") {
		t.Error("expected output to contain title")
	}
	if !strings.Contains(output, "test-host") {
		t.Error("expected output to contain hostname")
	}
	if !strings.Contains(output, "Test Check") {
		t.Error("expected output to contain check name")
	}
	// PrintReport uses symbols (✓, ⚠) not text status
	if !strings.Contains(output, "✓") {
		t.Error("expected output to contain OK symbol ✓")
	}
	if !strings.Contains(output, "⚠") {
		t.Error("expected output to contain Warning symbol ⚠")
	}
	// Summary shows counts
	if !strings.Contains(output, "Warning: 1") {
		t.Error("expected output to contain Warning count")
	}
}

func TestCheckResultFields(t *testing.T) {
	result := CheckResult{
		Name:        "Test",
		Category:    "Unit Test",
		Status:      StatusOK,
		Message:     "Test passed",
		Details:     "Additional info",
		Duration:    100 * time.Millisecond,
		Suggestions: []string{"Suggestion 1"},
	}

	if result.Name != "Test" {
		t.Errorf("expected Name to be 'Test', got %q", result.Name)
	}
	if result.Category != "Unit Test" {
		t.Errorf("expected Category to be 'Unit Test', got %q", result.Category)
	}
	if result.Status != StatusOK {
		t.Errorf("expected Status to be OK, got %q", result.Status)
	}
	if result.Message != "Test passed" {
		t.Errorf("expected Message to be 'Test passed', got %q", result.Message)
	}
	if result.Details != "Additional info" {
		t.Errorf("expected Details to be 'Additional info', got %q", result.Details)
	}
	if len(result.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(result.Suggestions))
	}
}

func TestDiagnosticReportHealthy(t *testing.T) {
	// Report with only OK checks should be healthy
	report := &DiagnosticReport{
		Checks: []CheckResult{
			{Status: StatusOK},
			{Status: StatusOK},
		},
		Summary: &Summary{
			Total: 2,
			OK:    2,
		},
	}
	report.Healthy = report.Summary.Critical == 0 && report.Summary.Error == 0

	if !report.Healthy {
		t.Error("expected report to be healthy")
	}

	// Report with critical check should not be healthy
	report2 := &DiagnosticReport{
		Checks: []CheckResult{
			{Status: StatusOK},
			{Status: StatusCritical},
		},
		Summary: &Summary{
			Total:    2,
			OK:       1,
			Critical: 1,
		},
	}
	report2.Healthy = report2.Summary.Critical == 0 && report2.Summary.Error == 0

	if report2.Healthy {
		t.Error("expected report to not be healthy with critical check")
	}
}

func TestRunnerWithCustomOptions(t *testing.T) {
	tmpDir := t.TempDir()

	opts := Options{
		Mode:       ModeQuick,
		ConfigPath: "/nonexistent/config.yaml",
		LogDir:     tmpDir,
		Output:     os.Stdout,
		Verbose:    true,
	}

	runner := NewRunner(opts)

	if runner.opts.Mode != ModeQuick {
		t.Errorf("expected Mode to be %q, got %q", ModeQuick, runner.opts.Mode)
	}
	if runner.opts.ConfigPath != "/nonexistent/config.yaml" {
		t.Errorf("expected ConfigPath to match, got %q", runner.opts.ConfigPath)
	}
	if runner.opts.LogDir != tmpDir {
		t.Errorf("expected LogDir to match, got %q", runner.opts.LogDir)
	}
	if !runner.opts.Verbose {
		t.Error("expected Verbose to be true")
	}
}

func TestContextCancellation(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeQuick
	runner := NewRunner(opts)

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run should complete quickly without hanging
	done := make(chan bool)
	go func() {
		_, _ = runner.Run(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Good, completed
	case <-time.After(5 * time.Second):
		t.Error("Run did not complete within timeout after context cancellation")
	}
}

func TestSystemInfoFields(t *testing.T) {
	info := &SystemInfo{
		Hostname:     "test",
		OS:           "linux",
		Kernel:       "5.4.0",
		Architecture: "amd64",
		CPUs:         4,
		Memory:       8 * 1024 * 1024 * 1024,
		Uptime:       "1 day",
		GoVersion:    "go1.23",
	}

	if info.Hostname != "test" {
		t.Errorf("expected Hostname to be 'test', got %q", info.Hostname)
	}
	if info.OS != "linux" {
		t.Errorf("expected OS to be 'linux', got %q", info.OS)
	}
	if info.CPUs != 4 {
		t.Errorf("expected CPUs to be 4, got %d", info.CPUs)
	}
	if info.Memory != 8*1024*1024*1024 {
		t.Errorf("expected Memory to be 8GB, got %d", info.Memory)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{30 * time.Minute, "30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{26*time.Hour + 30*time.Minute, "1d 2h 30m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.duration)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}

func TestRunFullMode(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeFull
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report == nil {
		t.Fatal("expected report to be non-nil")
	}

	// Full mode should have more checks than quick mode
	if len(report.Checks) < 10 {
		t.Errorf("expected at least 10 checks in full mode, got %d", len(report.Checks))
	}
}

func TestToJSON(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "test",
			OS:       "linux",
		},
		Checks: []CheckResult{
			{Name: "Test", Status: StatusOK},
		},
		Summary: &Summary{Total: 1, OK: 1},
		Healthy: true,
	}

	data, err := report.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}

	if !strings.Contains(string(data), "test") {
		t.Error("expected JSON to contain hostname")
	}
}

func TestPrintReportWithErrors(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "test-host",
			OS:       "linux",
			Kernel:   "5.4.0",
		},
		Checks: []CheckResult{
			{Name: "Critical Check", Category: "Test", Status: StatusCritical, Message: "Critical issue"},
			{Name: "Error Check", Category: "Test", Status: StatusError, Message: "Error occurred"},
			{Name: "Skipped Check", Category: "Test", Status: StatusSkipped, Message: "Skipped"},
		},
		Summary: &Summary{Total: 3, Critical: 1, Error: 1, Skipped: 1},
		Healthy: false,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)

	output := buf.String()

	if !strings.Contains(output, "✗") {
		t.Error("expected output to contain critical symbol ✗")
	}
	if !strings.Contains(output, "!") {
		t.Error("expected output to contain error symbol !")
	}
	if !strings.Contains(output, "○") {
		t.Error("expected output to contain skipped symbol ○")
	}
	if !strings.Contains(output, "ISSUES DETECTED") {
		t.Error("expected output to indicate issues detected")
	}
}

func TestPrintReportWithDetails(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp: time.Now(),
		Duration:  time.Second,
		SystemInfo: &SystemInfo{
			Hostname: "test-host",
			OS:       "linux",
		},
		Checks: []CheckResult{
			{
				Name:        "Detail Check",
				Category:    "Test",
				Status:      StatusWarning,
				Message:     "Warning message",
				Details:     "Detailed information here",
				Suggestions: []string{"Fix suggestion 1", "Fix suggestion 2"},
			},
		},
		Summary: &Summary{Total: 1, Warning: 1},
		Healthy: true,
	}

	var buf bytes.Buffer
	PrintReport(&buf, report)

	output := buf.String()

	if !strings.Contains(output, "Detailed information") {
		t.Error("expected output to contain details")
	}
	if !strings.Contains(output, "Fix suggestion 1") {
		t.Error("expected output to contain suggestion")
	}
}

func TestGetChecks(t *testing.T) {
	// Test quick mode
	optsQuick := DefaultOptions()
	optsQuick.Mode = ModeQuick
	runnerQuick := NewRunner(optsQuick)
	quickChecks := runnerQuick.getChecks()

	if len(quickChecks) != 5 {
		t.Errorf("expected 5 quick checks, got %d", len(quickChecks))
	}

	// Test full mode
	optsFull := DefaultOptions()
	optsFull.Mode = ModeFull
	runnerFull := NewRunner(optsFull)
	fullChecks := runnerFull.getChecks()

	if len(fullChecks) != 24 {
		t.Errorf("expected 24 full checks, got %d", len(fullChecks))
	}

	// Test debug mode (same as full)
	optsDebug := DefaultOptions()
	optsDebug.Mode = ModeDebug
	runnerDebug := NewRunner(optsDebug)
	debugChecks := runnerDebug.getChecks()

	if len(debugChecks) != 24 {
		t.Errorf("expected 24 debug checks, got %d", len(debugChecks))
	}
}

func TestCheckConfig(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		configPath     string
		createConfig   bool
		expectedStatus CheckStatus
	}{
		{
			name:           "config exists",
			configPath:     tmpDir + "/config.yaml",
			createConfig:   true,
			expectedStatus: StatusOK,
		},
		{
			name:           "config not found",
			configPath:     tmpDir + "/nonexistent.yaml",
			createConfig:   false,
			expectedStatus: StatusWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.createConfig {
				err := os.WriteFile(tt.configPath, []byte("test: true"), 0644)
				if err != nil {
					t.Fatalf("failed to create config file: %v", err)
				}
			}

			opts := DefaultOptions()
			opts.ConfigPath = tt.configPath
			runner := NewRunner(opts)

			result := runner.checkConfig(context.Background())
			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, result.Status)
			}
		})
	}
}

func TestCheckLogFiles(t *testing.T) {
	// Test with non-existent log directory
	opts := DefaultOptions()
	opts.LogDir = "/nonexistent/log/dir"
	runner := NewRunner(opts)

	result := runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected status OK for non-existent log dir, got %s", result.Status)
	}

	// Test with existing log directory
	tmpDir := t.TempDir()
	opts.LogDir = tmpDir
	runner = NewRunner(opts)

	result = runner.checkLogFiles(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected status OK for empty log dir, got %s", result.Status)
	}

	// Create some log files
	_ = os.WriteFile(tmpDir+"/test.log", []byte("log content"), 0644)
	_ = os.WriteFile(tmpDir+"/test.log.1", []byte("rotated log"), 0644)

	result = runner.checkLogFiles(context.Background())
	// Status depends on size thresholds
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s for log files", result.Status)
	}
}

func TestCheckPrerequisites(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkPrerequisites(context.Background())
	if result.Name != "Prerequisites" {
		t.Errorf("expected Name 'Prerequisites', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckVersions(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkVersions(ctx)
	if result.Name != "Versions" {
		t.Errorf("expected Name 'Versions', got %q", result.Name)
	}
	if result.Status != StatusOK {
		t.Errorf("expected status OK, got %s", result.Status)
	}
}

func TestCheckSystemInfo(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkSystemInfo(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected status OK, got %s", result.Status)
	}
	if result.Name != "System Info" {
		t.Errorf("expected Name 'System Info', got %q", result.Name)
	}
}

func TestCheckUSBAudio(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUSBAudio(context.Background())
	if result.Name != "USB Audio" {
		t.Errorf("expected Name 'USB Audio', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckUdevRules(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUdevRules(context.Background())
	if result.Name != "udev Rules" {
		t.Errorf("expected Name 'udev Rules', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckLockDir(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkLockDir(context.Background())
	if result.Name != "Lock Directory" {
		t.Errorf("expected Name 'Lock Directory', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkDiskSpace(context.Background())
	if result.Name != "Disk Space" {
		t.Errorf("expected Name 'Disk Space', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckMemory(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkMemory(context.Background())
	if result.Name != "Memory" {
		t.Errorf("expected Name 'Memory', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckNetworkPorts(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkNetworkPorts(ctx)
	if result.Name != "Network Ports" {
		t.Errorf("expected Name 'Network Ports', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckFFmpeg(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkFFmpeg(ctx)
	if result.Name != "FFmpeg" {
		t.Errorf("expected Name 'FFmpeg', got %q", result.Name)
	}
	if result.Duration <= 0 {
		t.Error("expected Duration to be positive")
	}
}

func TestCheckALSA(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkALSA(ctx)
	if result.Name != "ALSA" {
		t.Errorf("expected Name 'ALSA', got %q", result.Name)
	}
}

func TestCheckMediaMTXService(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXService(ctx)
	if result.Name != "MediaMTX Service" {
		t.Errorf("expected Name 'MediaMTX Service', got %q", result.Name)
	}
}

func TestCheckMediaMTXAPI(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkMediaMTXAPI(ctx)
	if result.Name != "MediaMTX API" {
		t.Errorf("expected Name 'MediaMTX API', got %q", result.Name)
	}
}

func TestCheckTimeSynchronization(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkTimeSynchronization(ctx)
	if result.Name != "Time Sync" {
		t.Errorf("expected Name 'Time Sync', got %q", result.Name)
	}
}

func TestCheckSystemdServices(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkSystemdServices(ctx)
	if result.Name != "Systemd Services" {
		t.Errorf("expected Name 'Systemd Services', got %q", result.Name)
	}
}

func TestCheckProcessStability(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkProcessStability(ctx)
	if result.Name != "Process Stability" {
		t.Errorf("expected Name 'Process Stability', got %q", result.Name)
	}
}

func TestCheckAudioConflicts(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioConflicts(ctx)
	if result.Name != "Audio Conflicts" {
		t.Errorf("expected Name 'Audio Conflicts', got %q", result.Name)
	}
}

func TestCheckInotifyLimits(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkInotifyLimits(context.Background())
	if result.Name != "inotify Limits" {
		t.Errorf("expected Name 'inotify Limits', got %q", result.Name)
	}
}

func TestCheckTCPResources(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkTCPResources(context.Background())
	if result.Name != "TCP Resources" {
		t.Errorf("expected Name 'TCP Resources', got %q", result.Name)
	}
}

func TestCheckEntropy(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkEntropy(context.Background())
	if result.Name != "Entropy" {
		t.Errorf("expected Name 'Entropy', got %q", result.Name)
	}
}

func TestCheckFileDescriptors(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkFileDescriptors(context.Background())
	if result.Name != "File Descriptors" {
		t.Errorf("expected Name 'File Descriptors', got %q", result.Name)
	}
}

func TestCheckAudioCapabilities(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioCapabilities(ctx)
	if result.Name != "Audio Capabilities" {
		t.Errorf("expected Name 'Audio Capabilities', got %q", result.Name)
	}
}

func TestRunDebugMode(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeDebug
	opts.Verbose = true
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report == nil {
		t.Fatal("expected report to be non-nil")
	}

	// Debug mode should have same checks as full mode
	if len(report.Checks) < 10 {
		t.Errorf("expected at least 10 checks in debug mode, got %d", len(report.Checks))
	}
}

func TestIsPortOpenWithValidAddress(t *testing.T) {
	// Test with localhost on a typically closed port
	result := isPortOpen("127.0.0.1:1")
	if result {
		t.Log("Port 1 appears open (unexpected)")
	}

	// Test with explicit localhost port
	result = isPortOpen("localhost:65534")
	if result {
		t.Log("Port 65534 appears open (unexpected)")
	}
}
