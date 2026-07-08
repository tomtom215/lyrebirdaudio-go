// SPDX-License-Identifier: MIT

//go:build linux

// Package diagnostics provides comprehensive system health checks for LyreBirdAudio.
//
// This implements the 24 diagnostic checks from the bash implementation,
// covering system resources, audio subsystem, services, and networking.
//
// Reference: lyrebird-diagnostics.sh
package diagnostics

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// CheckResult represents the result of a single diagnostic check.
type CheckResult struct {
	Name        string        `json:"name"`
	Category    string        `json:"category"`
	Status      CheckStatus   `json:"status"`
	Message     string        `json:"message"`
	Details     string        `json:"details,omitempty"`
	Duration    time.Duration `json:"duration"`
	Suggestions []string      `json:"suggestions,omitempty"`
}

// CheckStatus indicates the result of a check.
type CheckStatus string

const (
	StatusOK       CheckStatus = "OK"
	StatusWarning  CheckStatus = "WARNING"
	StatusCritical CheckStatus = "CRITICAL"
	StatusSkipped  CheckStatus = "SKIPPED"
	StatusError    CheckStatus = "ERROR"
)

// DiagnosticReport contains results from all diagnostic checks.
type DiagnosticReport struct {
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration `json:"duration"`
	SystemInfo *SystemInfo   `json:"system_info"`
	Checks     []CheckResult `json:"checks"`
	Summary    *Summary      `json:"summary"`
	Healthy    bool          `json:"healthy"`
}

// SystemInfo contains basic system information.
type SystemInfo struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Kernel       string `json:"kernel"`
	Architecture string `json:"architecture"`
	CPUs         int    `json:"cpus"`
	Memory       int64  `json:"memory_bytes"`
	Uptime       string `json:"uptime"`
	GoVersion    string `json:"go_version"`
}

// Summary contains a summary of check results.
type Summary struct {
	Total    int `json:"total"`
	OK       int `json:"ok"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
	Skipped  int `json:"skipped"`
	Error    int `json:"error"`
}

// CheckMode determines which checks to run.
type CheckMode string

const (
	ModeQuick CheckMode = "quick" // Essential checks only
	ModeFull  CheckMode = "full"  // All checks (default)
	ModeDebug CheckMode = "debug" // All checks with verbose output
)

// Diagnostic thresholds - all configurable for different deployment scenarios.
const (
	// LogSizeWarningBytes is the threshold for warning about log file sizes (100MB).
	LogSizeWarningBytes = 100 * 1024 * 1024

	// DiskUsageCriticalPercent is the disk usage percentage that triggers critical status.
	DiskUsageCriticalPercent = 95

	// DiskUsageWarningPercent is the disk usage percentage that triggers warning status.
	DiskUsageWarningPercent = 85

	// FDUsageCriticalPercent is the file descriptor usage percentage that triggers critical status.
	FDUsageCriticalPercent = 80

	// FDUsageWarningPercent is the file descriptor usage percentage that triggers warning status.
	FDUsageWarningPercent = 50

	// MemoryUsageCriticalPercent is the memory usage percentage that triggers critical status.
	MemoryUsageCriticalPercent = 90

	// MemoryUsageWarningPercent is the memory usage percentage that triggers warning status.
	MemoryUsageWarningPercent = 75

	// DefaultRTSPPort is the default MediaMTX RTSP port.
	DefaultRTSPPort = 8554

	// DefaultAPIPort is the default MediaMTX API port.
	DefaultAPIPort = 9997

	// MinInotifyWatches is the minimum recommended inotify watches.
	MinInotifyWatches = 8192

	// TimeWaitWarningThreshold is the number of TIME_WAIT connections that triggers a warning.
	TimeWaitWarningThreshold = 1000

	// MinEntropyBytes is the minimum recommended entropy pool size.
	MinEntropyBytes = 256
)

// DefaultPerCheckTimeout bounds how long any single diagnostic check may run
// before it is abandoned and reported as timed out. Several checks shell out to
// systemctl/journalctl/dmesg/timedatectl/ss; if one of those wedges (a stuck
// D-Bus or journald is exactly the condition an operator runs diagnostics to
// investigate) the whole report/bundle would otherwise hang indefinitely. This
// cap keeps each check — and therefore the overall run — bounded.
const DefaultPerCheckTimeout = 5 * time.Second

// Options configures the diagnostic run.
type Options struct {
	Mode       CheckMode
	ConfigPath string
	LogDir     string
	LockDir    string // directory holding daemon lock files (default /var/run/lyrebird)

	// Path overrides for testability. Defaults use the real system paths.
	ProcFS          string // /proc mount point (default "/proc")
	DevSndDir       string // sound device directory (default "/dev/snd")
	UdevRulesDir    string // udev rules directory (default "/etc/udev/rules.d")
	MediaMTXAPIAddr string // MediaMTX API host:port (default "localhost:9997")

	// PerCheckTimeout bounds each individual check. A value <= 0 falls back to
	// DefaultPerCheckTimeout. This prevents one wedged check (e.g. a subprocess
	// blocked on a stuck D-Bus/journald) from stalling the entire run.
	PerCheckTimeout time.Duration

	Output  io.Writer
	Verbose bool
}

// DefaultOptions returns default diagnostic options.
func DefaultOptions() Options {
	return Options{
		Mode:            ModeFull,
		ConfigPath:      "/etc/lyrebird/config.yaml",
		LogDir:          "/var/log/lyrebird",
		LockDir:         "/var/run/lyrebird",
		ProcFS:          "/proc",
		DevSndDir:       "/dev/snd",
		UdevRulesDir:    "/etc/udev/rules.d",
		MediaMTXAPIAddr: "localhost:9997",
		PerCheckTimeout: DefaultPerCheckTimeout,
		Output:          os.Stdout,
		Verbose:         false,
	}
}

// Runner executes diagnostic checks.
type Runner struct {
	opts Options
}

// NewRunner creates a new diagnostic runner.
func NewRunner(opts Options) *Runner {
	return &Runner{opts: opts}
}

// Run executes all diagnostic checks and returns a report.
func (r *Runner) Run(ctx context.Context) (*DiagnosticReport, error) {
	start := time.Now()

	report := &DiagnosticReport{
		Timestamp:  start,
		SystemInfo: r.collectSystemInfo(),
		Summary:    &Summary{},
	}

	// Define checks based on mode
	checks := r.getChecks()

	// Run each check
	for _, check := range checks {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		default:
			result := r.runCheck(ctx, check)
			report.Checks = append(report.Checks, result)

			// Update summary
			report.Summary.Total++
			switch result.Status {
			case StatusOK:
				report.Summary.OK++
			case StatusWarning:
				report.Summary.Warning++
			case StatusCritical:
				report.Summary.Critical++
			case StatusSkipped:
				report.Summary.Skipped++
			case StatusError:
				report.Summary.Error++
			}
		}
	}

	report.Duration = time.Since(start)
	report.Healthy = report.Summary.Critical == 0 && report.Summary.Error == 0

	return report, nil
}

// namedCheck pairs a diagnostic check with a stable, human-readable name.
//
// The name mirrors the CheckResult.Name each check sets internally, but it is
// needed up front: when a check exceeds its per-check timeout it never returns
// a CheckResult, so runCheck must synthesize one and needs a label for it.
// Keeping the name here lets a wedged check still appear in the report with a
// meaningful identity instead of a blank entry.
type namedCheck struct {
	name string
	fn   func(context.Context) CheckResult
}

// perCheckTimeout returns the maximum time an individual check may run, falling
// back to DefaultPerCheckTimeout when the option is unset or non-positive.
func (r *Runner) perCheckTimeout() time.Duration {
	if r.opts.PerCheckTimeout > 0 {
		return r.opts.PerCheckTimeout
	}
	return DefaultPerCheckTimeout
}

// runCheck executes a single diagnostic check under a bounded per-check timeout.
//
// The check receives its own context derived from ctx with the per-check
// deadline, so checks that shell out via exec.CommandContext have their
// subprocess killed at the deadline instead of hanging the whole run.
//
// The check runs in a separate goroutine and runCheck selects on its completion
// versus the deadline. This guarantees the runner returns within the timeout
// even for a check that ignores its context entirely — e.g. one blocked in an
// uninterruptible syscall or awaiting a subprocess stuck in D state that will
// not die on SIGKILL. In that case the goroutine is abandoned (the buffered
// channel lets it deliver its result and exit later without leaking) and
// runCheck synthesizes a degraded ERROR result so the check is still reported.
func (r *Runner) runCheck(ctx context.Context, c namedCheck) CheckResult {
	timeout := r.perCheckTimeout()
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	done := make(chan CheckResult, 1)
	go func() {
		done <- c.fn(checkCtx)
	}()

	select {
	case res := <-done:
		return res
	case <-checkCtx.Done():
		msg := fmt.Sprintf("check timed out after %s", timeout)
		// Distinguish a per-check timeout from parent cancellation so the
		// report does not mislabel an interrupted run as a wedged check.
		if ctx.Err() != nil {
			msg = "check cancelled: " + ctx.Err().Error()
		}
		return CheckResult{
			Name:     c.name,
			Category: "System",
			Status:   StatusError,
			Message:  msg,
			Duration: time.Since(start),
		}
	}
}

// getChecks returns the checks to run based on mode.
func (r *Runner) getChecks() []namedCheck {
	// Quick mode: essential checks only
	quickChecks := []namedCheck{
		{"FFmpeg", r.checkFFmpeg},
		{"ALSA", r.checkALSA},
		{"USB Audio", r.checkUSBAudio},
		{"MediaMTX Service", r.checkMediaMTXService},
		{"Configuration", r.checkConfig},
	}

	if r.opts.Mode == ModeQuick {
		return quickChecks
	}

	// Full mode: all checks. Names must match each check's internal
	// CheckResult.Name so a timed-out check is labeled consistently.
	return []namedCheck{
		{"Prerequisites", r.checkPrerequisites},
		{"Versions", r.checkVersions},
		{"System Info", r.checkSystemInfo},
		{"USB Audio", r.checkUSBAudio},
		{"Audio Capabilities", r.checkAudioCapabilities},
		{"FFmpeg", r.checkFFmpeg},
		{"ALSA", r.checkALSA},
		{"MediaMTX Service", r.checkMediaMTXService},
		{"MediaMTX API", r.checkMediaMTXAPI},
		{"MediaMTX Config", r.checkMediaMTXConfig},
		{"Configuration", r.checkConfig},
		{"udev Rules", r.checkUdevRules},
		{"Lock Directory", r.checkLockDir},
		{"Log Files", r.checkLogFiles},
		{"Disk Space", r.checkDiskSpace},
		{"File Descriptors", r.checkFileDescriptors},
		{"Memory", r.checkMemory},
		{"Network Ports", r.checkNetworkPorts},
		{"Time Sync", r.checkTimeSynchronization},
		{"Systemd Services", r.checkSystemdServices},
		{"Process Stability", r.checkProcessStability},
		{"Audio Conflicts", r.checkAudioConflicts},
		{"inotify Limits", r.checkInotifyLimits},
		{"TCP Resources", r.checkTCPResources},
		{"Entropy", r.checkEntropy},
		{"Kernel Modules", r.checkKernelModules},
		{"Device Permissions", r.checkDevicePermissions},
		{"FFmpeg Codecs", r.checkFFmpegCodecs},
		{"USB Stability", r.checkUSBStability},
		{"Lock File Permissions", r.checkLockFilePermissions},
		{"Resource Limits", r.checkUlimits},
	}
}

// collectSystemInfo gathers basic system information.
func (r *Runner) collectSystemInfo() *SystemInfo {
	info := &SystemInfo{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		CPUs:         runtime.NumCPU(),
		GoVersion:    runtime.Version(),
	}

	// Hostname
	if h, err := os.Hostname(); err == nil {
		info.Hostname = h
	}

	// Kernel version
	if data, err := os.ReadFile(r.opts.ProcFS + "/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			info.Kernel = parts[2]
		}
	}

	// Memory
	if data, err := os.ReadFile(r.opts.ProcFS + "/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						info.Memory = kb * 1024
					}
				}
				break
			}
		}
	}

	// Uptime
	if data, err := os.ReadFile(r.opts.ProcFS + "/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			if secs, err := strconv.ParseFloat(fields[0], 64); err == nil {
				d := time.Duration(secs) * time.Second
				info.Uptime = formatDuration(d)
			}
		}
	}

	return info
}
