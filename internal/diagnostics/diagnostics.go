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

// Options configures the diagnostic run.
type Options struct {
	Mode       CheckMode
	ConfigPath string
	LogDir     string
	LockDir    string // directory holding daemon lock files (default /var/run/lyrebird)

	// Path overrides for testability. Defaults use the real system paths.
	ProcFS       string // /proc mount point (default "/proc")
	DevSndDir    string // sound device directory (default "/dev/snd")
	UdevRulesDir string // udev rules directory (default "/etc/udev/rules.d")

	Output  io.Writer
	Verbose bool
}

// DefaultOptions returns default diagnostic options.
func DefaultOptions() Options {
	return Options{
		Mode:         ModeFull,
		ConfigPath:   "/etc/lyrebird/config.yaml",
		LogDir:       "/var/log/lyrebird",
		LockDir:      "/var/run/lyrebird",
		ProcFS:       "/proc",
		DevSndDir:    "/dev/snd",
		UdevRulesDir: "/etc/udev/rules.d",
		Output:       os.Stdout,
		Verbose:      false,
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
			result := check(ctx)
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

// getChecks returns the checks to run based on mode.
func (r *Runner) getChecks() []func(context.Context) CheckResult {
	// Quick mode: essential checks only
	quickChecks := []func(context.Context) CheckResult{
		r.checkFFmpeg,
		r.checkALSA,
		r.checkUSBAudio,
		r.checkMediaMTXService,
		r.checkConfig,
	}

	if r.opts.Mode == ModeQuick {
		return quickChecks
	}

	// Full mode: all 24 checks
	return []func(context.Context) CheckResult{
		// 1. Prerequisites & Dependencies
		r.checkPrerequisites,
		// 2. Project Info & Versions
		r.checkVersions,
		// 3. System Information
		r.checkSystemInfo,
		// 4. USB Audio Devices
		r.checkUSBAudio,
		// 5. Audio Capabilities
		r.checkAudioCapabilities,
		// 6. FFmpeg
		r.checkFFmpeg,
		// 7. ALSA
		r.checkALSA,
		// 8. MediaMTX Service
		r.checkMediaMTXService,
		// 9. MediaMTX API
		r.checkMediaMTXAPI,
		// 10. Configuration
		r.checkConfig,
		// 11. udev Rules
		r.checkUdevRules,
		// 12. Lock Directory
		r.checkLockDir,
		// 13. Log Files
		r.checkLogFiles,
		// 14. Disk Space
		r.checkDiskSpace,
		// 15. File Descriptors
		r.checkFileDescriptors,
		// 16. Memory
		r.checkMemory,
		// 17. Network Ports
		r.checkNetworkPorts,
		// 18. Time Synchronization
		r.checkTimeSynchronization,
		// 19. Systemd Services
		r.checkSystemdServices,
		// 20. Process Stability
		r.checkProcessStability,
		// 21. Audio Conflicts
		r.checkAudioConflicts,
		// 22. inotify Limits
		r.checkInotifyLimits,
		// 23. TCP Resources
		r.checkTCPResources,
		// 24. Entropy
		r.checkEntropy,
		// 25. Kernel Modules
		r.checkKernelModules,
		// 26. Device Permissions
		r.checkDevicePermissions,
		// 27. FFmpeg Codecs
		r.checkFFmpegCodecs,
		// 28. USB Stability
		r.checkUSBStability,
		// 29. Lock File Permissions
		r.checkLockFilePermissions,
		// 30. Resource Limits
		r.checkUlimits,
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
