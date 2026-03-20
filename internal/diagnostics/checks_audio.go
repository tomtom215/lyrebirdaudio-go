// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:9997/v3/paths/list", nil)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to create request"
		result.Duration = time.Since(start)
		return result
	}
	resp, err := client.Do(req)
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
