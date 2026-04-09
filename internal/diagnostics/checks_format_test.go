// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"
)

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
