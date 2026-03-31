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

	// Check for USB audio devices in procFS/asound
	pattern := r.opts.ProcFS + "/asound/card*/usbid"
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

	// #nosec G204 -- path is from exec.LookPath, not user input
	codecOut, _ := exec.CommandContext(ctx, path, "-encoders").Output()

	result.Status, result.Message, result.Details = evaluateFFmpegOutput(
		string(out), string(codecOut),
	)
	if result.Status == StatusWarning {
		result.Suggestions = append(result.Suggestions, "Install ffmpeg with opus support")
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

	// Check procFS/asound exists
	if _, err := os.Stat(r.opts.ProcFS + "/asound"); os.IsNotExist(err) {
		result.Status = StatusCritical
		result.Message = "ALSA not available (/proc/asound missing)"
		result.Suggestions = append(result.Suggestions, "Load ALSA kernel modules")
		result.Duration = time.Since(start)
		return result
	}

	// Check for audio cards
	cards, _ := filepath.Glob(r.opts.ProcFS + "/asound/card*")
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

	result.Status, result.Message, result.Suggestions = evaluateNetworkPorts(rtspOpen, apiOpen)

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

	result.Status, result.Message, result.Suggestions = evaluateDiskUsage(usedPercent, available)

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkFileDescriptors(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "File Descriptors",
		Category: "Resources",
	}

	data, err := os.ReadFile(r.opts.ProcFS + "/sys/fs/file-nr")
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to read file descriptor info"
		result.Duration = time.Since(start)
		return result
	}

	result.Status, result.Message = evaluateFDUsage(string(data))

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkMemory(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Memory",
		Category: "Resources",
	}

	data, err := os.ReadFile(r.opts.ProcFS + "/meminfo")
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to read memory info"
		result.Duration = time.Since(start)
		return result
	}

	result.Status, result.Message = evaluateMemoryUsage(string(data))

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

	result.Status, result.Message = evaluateTCPResources(string(out))

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

	result.Status, result.Message, result.Suggestions = evaluateAudioConflicts(pulseRunning == nil, pulseActive)

	result.Duration = time.Since(start)
	return result
}
