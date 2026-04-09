//go:build linux

package diagnostics

import (
	"context"
	"os"
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
