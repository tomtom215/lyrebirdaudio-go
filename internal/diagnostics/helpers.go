// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"fmt"
	"strconv"
	"strings"
)

// evaluateFDUsage determines the status of file descriptor usage from /proc/sys/fs/file-nr data.
func evaluateFDUsage(data string) (CheckStatus, string) {
	fields := strings.Fields(data)
	if len(fields) < 3 {
		return StatusError, "Invalid file-nr format"
	}

	used, _ := strconv.ParseInt(fields[0], 10, 64)
	max, _ := strconv.ParseInt(fields[2], 10, 64)
	if max == 0 {
		return StatusError, "Invalid max file descriptors (0)"
	}
	usedPercent := float64(used) / float64(max) * 100

	if usedPercent > FDUsageCriticalPercent {
		return StatusCritical, fmt.Sprintf("FD usage critical: %.1f%% (%d/%d)", usedPercent, used, max)
	} else if usedPercent > FDUsageWarningPercent {
		return StatusWarning, fmt.Sprintf("FD usage elevated: %.1f%% (%d/%d)", usedPercent, used, max)
	}
	return StatusOK, fmt.Sprintf("FD usage normal: %.1f%% (%d/%d)", usedPercent, used, max)
}

// evaluateMemoryUsage determines the status of memory usage from /proc/meminfo data.
func evaluateMemoryUsage(data string) (CheckStatus, string) {
	var total, available int64
	for _, line := range strings.Split(data, "\n") {
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

	if total == 0 {
		return StatusError, "Could not determine total memory"
	}

	usedPercent := 100.0 - (float64(available)/float64(total))*100.0

	if usedPercent > MemoryUsageCriticalPercent {
		return StatusCritical, fmt.Sprintf("Memory usage critical: %.1f%%", usedPercent)
	} else if usedPercent > MemoryUsageWarningPercent {
		return StatusWarning, fmt.Sprintf("Memory usage elevated: %.1f%%", usedPercent)
	}
	return StatusOK, fmt.Sprintf("Memory usage: %.1f%% (%s available)", usedPercent, formatBytes(available))
}

// evaluateEntropy determines the status of system entropy from /proc/sys/kernel/random/entropy_avail data.
func evaluateEntropy(data string) (CheckStatus, string, []string) {
	entropy, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64)
	if err != nil {
		return StatusError, "Could not parse entropy value", nil
	}
	if entropy < MinEntropyBytes {
		return StatusWarning, fmt.Sprintf("Entropy pool low: %d", entropy),
			[]string{"Install haveged or rng-tools"}
	}
	return StatusOK, fmt.Sprintf("Entropy pool: %d", entropy), nil
}

// evaluateInotifyLimits determines the status of inotify watches from /proc/sys/fs/inotify/max_user_watches data.
func evaluateInotifyLimits(data string) (CheckStatus, string, []string) {
	maxWatches, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64)
	if err != nil {
		return StatusError, "Could not parse inotify value", nil
	}
	if maxWatches < MinInotifyWatches {
		return StatusWarning, fmt.Sprintf("inotify max_user_watches low: %d", maxWatches),
			[]string{"Increase with: sysctl fs.inotify.max_user_watches=65536"}
	}
	return StatusOK, fmt.Sprintf("inotify max_user_watches: %d", maxWatches), nil
}

// evaluateDiskUsage determines disk usage status from used percentage and available bytes.
func evaluateDiskUsage(usedPercent float64, available uint64) (CheckStatus, string, []string) {
	if usedPercent > DiskUsageCriticalPercent {
		return StatusCritical, fmt.Sprintf("Disk usage critical: %.1f%%", usedPercent),
			[]string{"Free up disk space"}
	} else if usedPercent > DiskUsageWarningPercent {
		return StatusWarning, fmt.Sprintf("Disk usage high: %.1f%%", usedPercent), nil
	}
	return StatusOK, fmt.Sprintf("Disk usage: %.1f%% (%.1f GB available)",
		usedPercent, float64(available)/(1024*1024*1024)), nil
}

// evaluateNetworkPorts determines the status of RTSP and API ports.
func evaluateNetworkPorts(rtspOpen, apiOpen bool) (CheckStatus, string, []string) {
	if rtspOpen && apiOpen {
		return StatusOK, fmt.Sprintf("RTSP (%d) and API (%d) ports accessible",
			DefaultRTSPPort, DefaultAPIPort), nil
	} else if !rtspOpen && !apiOpen {
		return StatusWarning, "RTSP and API ports not accessible",
			[]string{"Start MediaMTX service"}
	}

	var ports []string
	if !rtspOpen {
		ports = append(ports, fmt.Sprintf("RTSP (%d)", DefaultRTSPPort))
	}
	if !apiOpen {
		ports = append(ports, fmt.Sprintf("API (%d)", DefaultAPIPort))
	}
	return StatusWarning, "Some ports not accessible: " + strings.Join(ports, ", "), nil
}

// evaluateAudioConflicts determines the status of audio conflicts based on PulseAudio state.
func evaluateAudioConflicts(pulseInstalled, pulseActive bool) (CheckStatus, string, []string) {
	if pulseActive {
		return StatusWarning, "PulseAudio running (may conflict with ALSA)",
			[]string{"Consider stopping PulseAudio for dedicated audio streaming"}
	} else if pulseInstalled {
		return StatusOK, "PulseAudio installed but not running", nil
	}
	return StatusOK, "No audio conflicts detected", nil
}

// evaluateTCPResources determines the status of TCP TIME_WAIT connections.
func evaluateTCPResources(ssOutput string) (CheckStatus, string) {
	timeWaitCount := strings.Count(ssOutput, "\n") - 1
	if timeWaitCount < 0 {
		timeWaitCount = 0
	}

	if timeWaitCount > TimeWaitWarningThreshold {
		return StatusWarning, fmt.Sprintf("High TIME_WAIT connections: %d", timeWaitCount)
	}
	return StatusOK, fmt.Sprintf("TIME_WAIT connections: %d", timeWaitCount)
}
