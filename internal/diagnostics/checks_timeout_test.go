// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRunCheckTimesOutAndMarksError verifies that a check which ignores its
// context (simulating a subprocess wedged on a stuck D-Bus/journald) does not
// stall the runner: runCheck must return within the per-check timeout and mark
// the check as a degraded ERROR result carrying the check's name.
func TestRunCheckTimesOutAndMarksError(t *testing.T) {
	opts := DefaultOptions()
	opts.PerCheckTimeout = 20 * time.Millisecond
	r := NewRunner(opts)

	// release keeps the slow check alive until the test ends, proving runCheck
	// returns even while the check goroutine is still blocked.
	release := make(chan struct{})
	defer close(release)

	slow := namedCheck{
		name: "Slow Check",
		fn: func(_ context.Context) CheckResult {
			<-release // ignore ctx entirely
			return CheckResult{Name: "Slow Check", Status: StatusOK}
		},
	}

	start := time.Now()
	res := r.runCheck(context.Background(), slow)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("runCheck did not honor per-check timeout; took %s", elapsed)
	}
	if res.Status != StatusError {
		t.Errorf("expected StatusError for timed-out check, got %s (%s)", res.Status, res.Message)
	}
	if res.Name != "Slow Check" {
		t.Errorf("expected timeout result to carry check name, got %q", res.Name)
	}
	if !strings.Contains(res.Message, "timed out") {
		t.Errorf("expected 'timed out' in message, got %q", res.Message)
	}
	if res.Duration <= 0 {
		t.Error("expected positive Duration on timeout result")
	}
}

// TestRunCheckPassesResultThrough verifies a fast check's own result is returned
// unchanged (the timeout path must not interfere with normal completion).
func TestRunCheckPassesResultThrough(t *testing.T) {
	opts := DefaultOptions()
	opts.PerCheckTimeout = time.Second
	r := NewRunner(opts)

	c := namedCheck{
		name: "Fast",
		fn: func(_ context.Context) CheckResult {
			return CheckResult{Name: "Fast", Status: StatusWarning, Message: "quick"}
		},
	}

	res := r.runCheck(context.Background(), c)
	if res.Status != StatusWarning || res.Message != "quick" {
		t.Errorf("fast check result not passed through: %+v", res)
	}
}

// TestRunCheckPassesBoundedContext verifies each check receives a context that
// carries the per-check deadline, so checks that shell out via
// exec.CommandContext have their subprocess killed at the deadline.
func TestRunCheckPassesBoundedContext(t *testing.T) {
	opts := DefaultOptions()
	opts.PerCheckTimeout = 250 * time.Millisecond
	r := NewRunner(opts)

	c := namedCheck{
		name: "Deadline",
		fn: func(ctx context.Context) CheckResult {
			res := CheckResult{Name: "Deadline", Status: StatusOK}
			if _, ok := ctx.Deadline(); ok {
				res.Details = "has-deadline"
			}
			return res
		},
	}

	res := r.runCheck(context.Background(), c)
	if res.Details != "has-deadline" {
		t.Errorf("expected check to receive a context with a deadline, got Details=%q", res.Details)
	}
}

// TestPerCheckTimeoutFallback verifies the default is used when unset/non-positive.
func TestPerCheckTimeoutFallback(t *testing.T) {
	tests := []struct {
		name string
		set  time.Duration
		want time.Duration
	}{
		{"zero uses default", 0, DefaultPerCheckTimeout},
		{"negative uses default", -1, DefaultPerCheckTimeout},
		{"positive honored", 750 * time.Millisecond, 750 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultOptions()
			opts.PerCheckTimeout = tt.set
			r := NewRunner(opts)
			if got := r.perCheckTimeout(); got != tt.want {
				t.Errorf("perCheckTimeout() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestRunWithTinyTimeoutStillCompletes verifies the timeout is wired into Run:
// even with an aggressive per-check budget the full run returns promptly with a
// complete set of results (some may be ERROR) instead of hanging.
func TestRunWithTinyTimeoutStillCompletes(t *testing.T) {
	opts := DefaultOptions()
	opts.Mode = ModeFull
	opts.PerCheckTimeout = time.Millisecond
	r := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	var report *DiagnosticReport
	go func() {
		report, _ = r.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not complete under an aggressive per-check timeout")
	}

	if report == nil || len(report.Checks) != 31 {
		t.Fatalf("expected 31 check results, got %v", report)
	}
	sum := report.Summary.OK + report.Summary.Warning + report.Summary.Critical +
		report.Summary.Error + report.Summary.Skipped
	if sum != report.Summary.Total {
		t.Errorf("summary counts don't add up: %d != Total %d", sum, report.Summary.Total)
	}
}
