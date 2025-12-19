// Package diagnostics provides comprehensive system health checks for LyreBirdAudio.
//
// This implements the 24 diagnostic checks from the bash implementation,
// covering system resources, audio subsystem, services, and networking.
//
// Reference: lyrebird-diagnostics.sh
package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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
	Output     io.Writer
	Verbose    bool
}

// DefaultOptions returns default diagnostic options.
func DefaultOptions() Options {
	return Options{
		Mode:       ModeFull,
		ConfigPath: "/etc/lyrebird/config.yaml",
		LogDir:     "/var/log/lyrebird",
		Output:     os.Stdout,
		Verbose:    false,
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
	if data, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			info.Kernel = parts[2]
		}
	}

	// Memory
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
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
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
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

// Individual check implementations

func (r *Runner) checkPrerequisites(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Prerequisites",
		Category: "System",
	}

	required := []string{"ffmpeg"}
	optional := []string{"arecord", "aplay", "udevadm", "systemctl"}

	var missing []string
	var warnings []string

	for _, cmd := range required {
		if _, err := exec.LookPath(cmd); err != nil {
			missing = append(missing, cmd)
		}
	}

	for _, cmd := range optional {
		if _, err := exec.LookPath(cmd); err != nil {
			warnings = append(warnings, cmd)
		}
	}

	if len(missing) > 0 {
		result.Status = StatusCritical
		result.Message = fmt.Sprintf("Missing required tools: %s", strings.Join(missing, ", "))
		result.Suggestions = append(result.Suggestions, "Install missing tools with: apt-get install "+strings.Join(missing, " "))
	} else if len(warnings) > 0 {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Missing optional tools: %s", strings.Join(warnings, ", "))
	} else {
		result.Status = StatusOK
		result.Message = "All required tools available"
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkVersions(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Versions",
		Category: "System",
	}

	var versions []string

	// FFmpeg version
	if out, err := exec.CommandContext(ctx, "ffmpeg", "-version").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			versions = append(versions, "FFmpeg: "+strings.TrimPrefix(lines[0], "ffmpeg version "))
		}
	}

	// MediaMTX version (if available)
	if out, err := exec.CommandContext(ctx, "mediamtx", "--version").Output(); err == nil {
		versions = append(versions, "MediaMTX: "+strings.TrimSpace(string(out)))
	}

	result.Status = StatusOK
	result.Message = "Version information collected"
	result.Details = strings.Join(versions, "\n")
	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkSystemInfo(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "System Info",
		Category: "System",
		Status:   StatusOK,
		Message:  "System information collected",
	}
	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkUSBAudio(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "USB Audio",
		Category: "Audio",
	}

	// Check for USB audio devices in /proc/asound
	pattern := "/proc/asound/card*/usbid"
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		result.Status = StatusWarning
		result.Message = "No USB audio devices detected"
		result.Suggestions = append(result.Suggestions, "Connect a USB audio device")
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Found %d USB audio device(s)", len(matches))

		// Get device names
		var devices []string
		for _, m := range matches {
			cardDir := filepath.Dir(m)
			// #nosec G304 -- reading from /proc/asound, controlled path
			if id, err := os.ReadFile(filepath.Join(cardDir, "id")); err == nil {
				devices = append(devices, strings.TrimSpace(string(id)))
			}
		}
		result.Details = strings.Join(devices, ", ")
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkAudioCapabilities(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Audio Capabilities",
		Category: "Audio",
	}

	// Check for ALSA mixer
	if _, err := exec.LookPath("amixer"); err != nil {
		result.Status = StatusWarning
		result.Message = "amixer not available"
	} else if out, err := exec.CommandContext(ctx, "amixer", "info").Output(); err == nil {
		result.Status = StatusOK
		result.Message = "ALSA mixer available"
		result.Details = string(out)
	} else {
		result.Status = StatusWarning
		result.Message = "ALSA mixer check failed"
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkFFmpeg(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "FFmpeg",
		Category: "Tools",
	}

	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		result.Status = StatusCritical
		result.Message = "FFmpeg not found"
		result.Suggestions = append(result.Suggestions, "Install FFmpeg: apt-get install ffmpeg")
		result.Duration = time.Since(start)
		return result
	}

	// Check version and codecs
	// #nosec G204 -- path is from exec.LookPath, not user input
	out, err := exec.CommandContext(ctx, path, "-version").Output()
	if err != nil {
		result.Status = StatusWarning
		result.Message = "FFmpeg found but version check failed"
		result.Duration = time.Since(start)
		return result
	}

	// Check for opus codec
	// #nosec G204 -- path is from exec.LookPath, not user input
	codecOut, _ := exec.CommandContext(ctx, path, "-encoders").Output()
	hasOpus := strings.Contains(string(codecOut), "libopus")
	hasAAC := strings.Contains(string(codecOut), "aac")

	if !hasOpus && !hasAAC {
		result.Status = StatusWarning
		result.Message = "FFmpeg missing recommended audio codecs"
		result.Suggestions = append(result.Suggestions, "Install ffmpeg with opus support")
	} else {
		result.Status = StatusOK
		result.Message = "FFmpeg available with audio codecs"
	}

	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		result.Details = lines[0]
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkALSA(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "ALSA",
		Category: "Audio",
	}

	// Check /proc/asound exists
	if _, err := os.Stat("/proc/asound"); os.IsNotExist(err) {
		result.Status = StatusCritical
		result.Message = "ALSA not available (/proc/asound missing)"
		result.Suggestions = append(result.Suggestions, "Load ALSA kernel modules")
		result.Duration = time.Since(start)
		return result
	}

	// Check for audio cards
	cards, _ := filepath.Glob("/proc/asound/card*")
	if len(cards) == 0 {
		result.Status = StatusWarning
		result.Message = "No ALSA audio cards found"
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("ALSA available with %d card(s)", len(cards))
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkMediaMTXService(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "MediaMTX Service",
		Category: "Services",
	}

	// Check if mediamtx binary exists
	if _, err := exec.LookPath("mediamtx"); err != nil {
		result.Status = StatusWarning
		result.Message = "MediaMTX not installed"
		result.Suggestions = append(result.Suggestions, "Run: lyrebird install-mediamtx")
		result.Duration = time.Since(start)
		return result
	}

	// Check systemd service status
	out, err := exec.CommandContext(ctx, "systemctl", "is-active", "mediamtx").Output()
	if err != nil {
		result.Status = StatusWarning
		result.Message = "MediaMTX service not running"
		result.Suggestions = append(result.Suggestions, "Start service: systemctl start mediamtx")
	} else if strings.TrimSpace(string(out)) == "active" {
		result.Status = StatusOK
		result.Message = "MediaMTX service running"
	} else {
		result.Status = StatusWarning
		result.Message = "MediaMTX service state: " + strings.TrimSpace(string(out))
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkMediaMTXAPI(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "MediaMTX API",
		Category: "Services",
	}

	// Try to connect to API
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:9997/v3/paths/list")
	if err != nil {
		result.Status = StatusWarning
		result.Message = "MediaMTX API not reachable"
		result.Details = err.Error()
		result.Duration = time.Since(start)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 {
		result.Status = StatusOK
		result.Message = "MediaMTX API reachable"
	} else {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("MediaMTX API returned status %d", resp.StatusCode)
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkConfig(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Configuration",
		Category: "Config",
	}

	if _, err := os.Stat(r.opts.ConfigPath); os.IsNotExist(err) {
		result.Status = StatusWarning
		result.Message = "Configuration file not found"
		result.Details = r.opts.ConfigPath
		result.Suggestions = append(result.Suggestions, "Run: lyrebird setup")
	} else {
		result.Status = StatusOK
		result.Message = "Configuration file exists"
		result.Details = r.opts.ConfigPath
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkUdevRules(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "udev Rules",
		Category: "Config",
	}

	rulesPath := "/etc/udev/rules.d/99-usb-soundcards.rules"
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		result.Status = StatusWarning
		result.Message = "udev rules not configured"
		result.Suggestions = append(result.Suggestions, "Run: lyrebird usb-map")
	} else {
		result.Status = StatusOK
		result.Message = "udev rules configured"
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkLockDir(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Lock Directory",
		Category: "System",
	}

	lockDir := "/var/run/lyrebird"
	if info, err := os.Stat(lockDir); os.IsNotExist(err) {
		result.Status = StatusOK
		result.Message = "Lock directory will be created on first run"
	} else if !info.IsDir() {
		result.Status = StatusCritical
		result.Message = "Lock path exists but is not a directory"
	} else {
		result.Status = StatusOK
		result.Message = "Lock directory exists"

		// Count lock files
		entries, _ := os.ReadDir(lockDir)
		locks := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".lock") {
				locks++
			}
		}
		if locks > 0 {
			result.Details = fmt.Sprintf("%d active lock(s)", locks)
		}
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkLogFiles(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Log Files",
		Category: "System",
	}

	// Check log directory
	if _, err := os.Stat(r.opts.LogDir); os.IsNotExist(err) {
		result.Status = StatusOK
		result.Message = "Log directory will be created on first run"
		result.Duration = time.Since(start)
		return result
	}

	// Calculate total log size
	var totalSize int64
	_ = filepath.Walk(r.opts.LogDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if totalSize > LogSizeWarningBytes {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Log directory size: %s", formatBytes(totalSize))
		result.Suggestions = append(result.Suggestions, "Consider cleaning old logs")
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Log directory size: %s", formatBytes(totalSize))
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkDiskSpace(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Disk Space",
		Category: "Resources",
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		result.Status = StatusError
		result.Message = "Failed to check disk space"
		result.Duration = time.Since(start)
		return result
	}

	// #nosec G115 -- Bsize is always positive on Linux filesystems
	available := stat.Bavail * uint64(stat.Bsize)
	// #nosec G115 -- Bsize is always positive on Linux filesystems
	total := stat.Blocks * uint64(stat.Bsize)
	usedPercent := 100.0 - (float64(available)/float64(total))*100.0

	if usedPercent > DiskUsageCriticalPercent {
		result.Status = StatusCritical
		result.Message = fmt.Sprintf("Disk usage critical: %.1f%%", usedPercent)
		result.Suggestions = append(result.Suggestions, "Free up disk space")
	} else if usedPercent > DiskUsageWarningPercent {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Disk usage high: %.1f%%", usedPercent)
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Disk usage: %.1f%% (%.1f GB available)", usedPercent, float64(available)/(1024*1024*1024))
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkFileDescriptors(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "File Descriptors",
		Category: "Resources",
	}

	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to read file descriptor info"
		result.Duration = time.Since(start)
		return result
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		result.Status = StatusError
		result.Message = "Invalid file-nr format"
		result.Duration = time.Since(start)
		return result
	}

	used, _ := strconv.ParseInt(fields[0], 10, 64)
	max, _ := strconv.ParseInt(fields[2], 10, 64)
	usedPercent := float64(used) / float64(max) * 100

	if usedPercent > FDUsageCriticalPercent {
		result.Status = StatusCritical
		result.Message = fmt.Sprintf("FD usage critical: %.1f%% (%d/%d)", usedPercent, used, max)
	} else if usedPercent > FDUsageWarningPercent {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("FD usage elevated: %.1f%% (%d/%d)", usedPercent, used, max)
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("FD usage normal: %.1f%% (%d/%d)", usedPercent, used, max)
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkMemory(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Memory",
		Category: "Resources",
	}

	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to read memory info"
		result.Duration = time.Since(start)
		return result
	}

	var total, available int64
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				total, _ = strconv.ParseInt(fields[1], 10, 64)
				total *= 1024
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				available, _ = strconv.ParseInt(fields[1], 10, 64)
				available *= 1024
			}
		}
	}

	usedPercent := 100.0 - (float64(available)/float64(total))*100.0

	if usedPercent > MemoryUsageCriticalPercent {
		result.Status = StatusCritical
		result.Message = fmt.Sprintf("Memory usage critical: %.1f%%", usedPercent)
	} else if usedPercent > MemoryUsageWarningPercent {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Memory usage elevated: %.1f%%", usedPercent)
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Memory usage: %.1f%% (%s available)", usedPercent, formatBytes(available))
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkNetworkPorts(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Network Ports",
		Category: "Network",
	}

	// Check RTSP and API ports
	rtspAddr := fmt.Sprintf("localhost:%d", DefaultRTSPPort)
	apiAddr := fmt.Sprintf("localhost:%d", DefaultAPIPort)
	rtspOpen := isPortOpen(rtspAddr)
	apiOpen := isPortOpen(apiAddr)

	if rtspOpen && apiOpen {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("RTSP (%d) and API (%d) ports accessible", DefaultRTSPPort, DefaultAPIPort)
	} else if !rtspOpen && !apiOpen {
		result.Status = StatusWarning
		result.Message = "RTSP and API ports not accessible"
		result.Suggestions = append(result.Suggestions, "Start MediaMTX service")
	} else {
		result.Status = StatusWarning
		var ports []string
		if !rtspOpen {
			ports = append(ports, fmt.Sprintf("RTSP (%d)", DefaultRTSPPort))
		}
		if !apiOpen {
			ports = append(ports, fmt.Sprintf("API (%d)", DefaultAPIPort))
		}
		result.Message = "Some ports not accessible: " + strings.Join(ports, ", ")
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkTimeSynchronization(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Time Sync",
		Category: "System",
	}

	// Check timedatectl status
	out, err := exec.CommandContext(ctx, "timedatectl", "status").Output()
	if err != nil {
		result.Status = StatusOK
		result.Message = "Time sync check skipped (timedatectl not available)"
		result.Duration = time.Since(start)
		return result
	}

	if strings.Contains(string(out), "synchronized: yes") {
		result.Status = StatusOK
		result.Message = "System time synchronized"
	} else {
		result.Status = StatusWarning
		result.Message = "System time may not be synchronized"
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkSystemdServices(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Systemd Services",
		Category: "Services",
	}

	services := []string{"mediamtx", "lyrebird-stream"}
	var running, stopped []string

	for _, svc := range services {
		// #nosec G204 -- svc is from hardcoded list, not user input
		out, _ := exec.CommandContext(ctx, "systemctl", "is-active", svc).Output()
		status := strings.TrimSpace(string(out))
		if status == "active" {
			running = append(running, svc)
		} else {
			stopped = append(stopped, svc)
		}
	}

	if len(running) == len(services) {
		result.Status = StatusOK
		result.Message = "All services running"
	} else if len(running) > 0 {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Some services stopped: %s", strings.Join(stopped, ", "))
	} else {
		result.Status = StatusWarning
		result.Message = "No LyreBird services running"
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkProcessStability(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Process Stability",
		Category: "Services",
	}

	// Check for recent service restarts
	out, err := exec.CommandContext(ctx, "journalctl", "-u", "mediamtx", "--since", "1 hour ago", "-q").Output()
	if err != nil {
		result.Status = StatusOK
		result.Message = "Process stability check skipped"
		result.Duration = time.Since(start)
		return result
	}

	restarts := strings.Count(string(out), "Started")
	if restarts > 3 {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("MediaMTX restarted %d times in last hour", restarts)
	} else {
		result.Status = StatusOK
		result.Message = "Services stable"
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkAudioConflicts(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Audio Conflicts",
		Category: "Audio",
	}

	// Check for PulseAudio
	_, pulseRunning := exec.LookPath("pulseaudio")
	out, _ := exec.CommandContext(ctx, "pgrep", "pulseaudio").Output()
	pulseActive := len(out) > 0

	if pulseActive {
		result.Status = StatusWarning
		result.Message = "PulseAudio running (may conflict with ALSA)"
		result.Suggestions = append(result.Suggestions, "Consider stopping PulseAudio for dedicated audio streaming")
	} else if pulseRunning == nil {
		result.Status = StatusOK
		result.Message = "PulseAudio installed but not running"
	} else {
		result.Status = StatusOK
		result.Message = "No audio conflicts detected"
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkInotifyLimits(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "inotify Limits",
		Category: "Resources",
	}

	data, err := os.ReadFile("/proc/sys/fs/inotify/max_user_watches")
	if err != nil {
		result.Status = StatusOK
		result.Message = "inotify check skipped"
		result.Duration = time.Since(start)
		return result
	}

	maxWatches, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)

	if maxWatches < MinInotifyWatches {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("inotify max_user_watches low: %d", maxWatches)
		result.Suggestions = append(result.Suggestions, "Increase with: sysctl fs.inotify.max_user_watches=65536")
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("inotify max_user_watches: %d", maxWatches)
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkTCPResources(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "TCP Resources",
		Category: "Network",
	}

	// Count TIME_WAIT connections
	out, err := exec.CommandContext(ctx, "ss", "-tan", "state", "time-wait").Output()
	if err != nil {
		result.Status = StatusOK
		result.Message = "TCP check skipped"
		result.Duration = time.Since(start)
		return result
	}

	timeWaitCount := strings.Count(string(out), "\n") - 1
	if timeWaitCount < 0 {
		timeWaitCount = 0
	}

	if timeWaitCount > TimeWaitWarningThreshold {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("High TIME_WAIT connections: %d", timeWaitCount)
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("TIME_WAIT connections: %d", timeWaitCount)
	}

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkEntropy(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Entropy",
		Category: "System",
	}

	data, err := os.ReadFile("/proc/sys/kernel/random/entropy_avail")
	if err != nil {
		result.Status = StatusOK
		result.Message = "Entropy check skipped"
		result.Duration = time.Since(start)
		return result
	}

	entropy, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)

	if entropy < MinEntropyBytes {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Entropy pool low: %d", entropy)
		result.Suggestions = append(result.Suggestions, "Install haveged or rng-tools")
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Entropy pool: %d", entropy)
	}

	result.Duration = time.Since(start)
	return result
}

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
