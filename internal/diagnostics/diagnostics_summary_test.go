//go:build linux

package diagnostics

import (
	"testing"
)

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
