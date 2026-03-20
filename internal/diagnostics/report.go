// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Helper functions

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func isPortOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// PrintReport prints a formatted diagnostic report.
func PrintReport(w io.Writer, report *DiagnosticReport) {
	_, _ = fmt.Fprintf(w, "LyreBirdAudio Diagnostics Report\n")
	_, _ = fmt.Fprintf(w, "================================\n\n")

	_, _ = fmt.Fprintf(w, "System: %s (%s/%s)\n", report.SystemInfo.Hostname, report.SystemInfo.OS, report.SystemInfo.Architecture)
	_, _ = fmt.Fprintf(w, "Kernel: %s\n", report.SystemInfo.Kernel)
	_, _ = fmt.Fprintf(w, "Uptime: %s\n", report.SystemInfo.Uptime)
	_, _ = fmt.Fprintf(w, "Time: %s\n\n", report.Timestamp.Format(time.RFC3339))

	// Group checks by category
	categories := make(map[string][]CheckResult)
	for _, check := range report.Checks {
		categories[check.Category] = append(categories[check.Category], check)
	}

	for category, checks := range categories {
		_, _ = fmt.Fprintf(w, "\n%s\n%s\n", category, strings.Repeat("-", len(category)))
		for _, check := range checks {
			status := "✓"
			switch check.Status {
			case StatusWarning:
				status = "⚠"
			case StatusCritical:
				status = "✗"
			case StatusError:
				status = "!"
			case StatusSkipped:
				status = "○"
			}
			_, _ = fmt.Fprintf(w, "[%s] %s: %s\n", status, check.Name, check.Message)
			if check.Details != "" {
				_, _ = fmt.Fprintf(w, "    %s\n", check.Details)
			}
			for _, suggestion := range check.Suggestions {
				_, _ = fmt.Fprintf(w, "    → %s\n", suggestion)
			}
		}
	}

	_, _ = fmt.Fprintf(w, "\n\nSummary\n-------\n")
	_, _ = fmt.Fprintf(w, "Total: %d | OK: %d | Warning: %d | Critical: %d | Error: %d | Skipped: %d\n",
		report.Summary.Total, report.Summary.OK, report.Summary.Warning,
		report.Summary.Critical, report.Summary.Error, report.Summary.Skipped)
	_, _ = fmt.Fprintf(w, "Duration: %v\n", report.Duration)

	if report.Healthy {
		_, _ = fmt.Fprintf(w, "\nSystem Status: HEALTHY\n")
	} else {
		_, _ = fmt.Fprintf(w, "\nSystem Status: ISSUES DETECTED\n")
	}
}

// ToJSON converts the report to JSON format.
func (r *DiagnosticReport) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
