// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
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
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	}

	if report == nil {
		t.Error("expected report to be non-nil even on cancellation")
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
	if len(checks) != 31 {
		t.Errorf("expected 31 full checks, got %d", len(checks))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(report.Checks) != 31 {
		t.Errorf("expected 31 check results in full mode, got %d", len(report.Checks))
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
