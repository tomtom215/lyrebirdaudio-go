// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// evaluateKernelModules checks which required/optional modules are loaded from /proc/modules content.
func evaluateKernelModules(modulesData string, required, optional []string) (CheckStatus, string) {
	loadedModules := make(map[string]bool)
	for _, line := range strings.Split(modulesData, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			loadedModules[fields[0]] = true
		}
	}

	var missingRequired []string
	var missingOptional []string

	for _, mod := range required {
		if !loadedModules[mod] {
			missingRequired = append(missingRequired, mod)
		}
	}
	for _, mod := range optional {
		if !loadedModules[mod] {
			missingOptional = append(missingOptional, mod)
		}
	}

	if len(missingRequired) > 0 {
		return StatusCritical, fmt.Sprintf("Missing required kernel modules: %s", strings.Join(missingRequired, ", "))
	}
	if len(missingOptional) > 0 {
		return StatusWarning, fmt.Sprintf("All required modules loaded; missing optional: %s", strings.Join(missingOptional, ", "))
	}
	return StatusOK, "All audio kernel modules loaded"
}

// evaluateUSBStability analyzes dmesg output for USB errors and disconnects.
func evaluateUSBStability(dmesgOutput string) (CheckStatus, string, string) {
	lines := strings.Split(dmesgOutput, "\n")
	usbErrors := 0
	usbDisconnects := 0
	var recentErrors []string

	for _, line := range lines {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "usb") {
			continue
		}
		if strings.Contains(lower, "disconnect") {
			usbDisconnects++
			if len(recentErrors) < 5 {
				recentErrors = append(recentErrors, strings.TrimSpace(line))
			}
		} else if strings.Contains(lower, "error") || strings.Contains(lower, "cannot") || strings.Contains(lower, "timeout") {
			usbErrors++
			if len(recentErrors) < 5 {
				recentErrors = append(recentErrors, strings.TrimSpace(line))
			}
		}
	}

	details := strings.Join(recentErrors, "\n")

	if usbErrors > 10 || usbDisconnects > 5 {
		return StatusWarning, fmt.Sprintf("%d USB errors, %d disconnects in dmesg", usbErrors, usbDisconnects), details
	}
	if usbErrors > 0 || usbDisconnects > 0 {
		return StatusOK, fmt.Sprintf("%d USB errors, %d disconnects (within normal range)", usbErrors, usbDisconnects), ""
	}
	return StatusOK, "No USB errors in dmesg", ""
}

// evaluateResourceLimits analyzes /proc/self/limits content for low limits.
func evaluateResourceLimits(limitsData string) (CheckStatus, string) {
	var issues []string

	for _, line := range strings.Split(limitsData, "\n") {
		softLimit := parseLimitLine(line)
		if strings.HasPrefix(line, "Max open files") {
			if softLimit > 0 && softLimit < 1024 {
				issues = append(issues, fmt.Sprintf("low open files limit: %d (recommend >= 1024)", softLimit))
			}
		}
		if strings.HasPrefix(line, "Max processes") {
			if softLimit > 0 && softLimit < 512 {
				issues = append(issues, fmt.Sprintf("low process limit: %d (recommend >= 512)", softLimit))
			}
		}
	}

	if len(issues) > 0 {
		return StatusWarning, strings.Join(issues, "; ")
	}
	return StatusOK, "Resource limits adequate"
}

// parseLimitLine extracts the soft limit (first numeric field) from a /proc/self/limits line.
func parseLimitLine(line string) int64 {
	fields := strings.Fields(line)
	for _, f := range fields {
		if v, err := strconv.ParseInt(f, 10, 64); err == nil {
			return v
		}
	}
	return 0
}

// evaluateCodecsOutput determines FFmpeg codec availability from encoder/decoder output.
// encoderOutput is from `ffmpeg -encoders`, decoderOutput is from `ffmpeg -decoders`.
func evaluateCodecsOutput(
	encoderOutput, decoderOutput string,
	requiredEncoders, requiredDecoders map[string]string,
) (status CheckStatus, message, details string) {
	var missing []string
	var found []string

	for codec, desc := range requiredEncoders {
		if strings.Contains(encoderOutput, codec) {
			found = append(found, "encoder "+codec+": OK")
		} else {
			missing = append(missing, codec+" ("+desc+")")
		}
	}
	for codec, desc := range requiredDecoders {
		if strings.Contains(decoderOutput, codec) {
			found = append(found, "decoder "+codec+": OK")
		} else {
			missing = append(missing, codec+" ("+desc+")")
		}
	}

	// Sort for deterministic output in tests.
	sort.Strings(found)
	sort.Strings(missing)

	if len(missing) > 0 {
		return StatusCritical,
			fmt.Sprintf("Missing codecs: %s", strings.Join(missing, "; ")),
			""
	}
	return StatusOK, "All required codecs available", strings.Join(found, "; ")
}

// evaluateFFmpegOutput determines FFmpeg status from version and codec output.
// Returns status, message, and details (first line of version output).
func evaluateFFmpegOutput(versionOut, codecOut string) (CheckStatus, string, string) {
	lines := strings.SplitN(versionOut, "\n", 2)
	details := ""
	if len(lines) > 0 {
		details = strings.TrimSpace(lines[0])
	}

	hasOpus := strings.Contains(codecOut, "libopus")
	hasAAC := strings.Contains(codecOut, "aac")
	if !hasOpus && !hasAAC {
		return StatusWarning, "FFmpeg missing recommended audio codecs", details
	}
	return StatusOK, "FFmpeg available with audio codecs", details
}

// evaluateTimeSyncOutput parses `timedatectl status` output to determine sync status.
func evaluateTimeSyncOutput(output string) (CheckStatus, string) {
	if strings.Contains(output, "synchronized: yes") || strings.Contains(output, "System clock synchronized: yes") {
		return StatusOK, "System time synchronized"
	}
	return StatusWarning, "System time may not be synchronized"
}

// evaluateSystemdServicesOutput evaluates a map of service → is-active status strings.
// services is the ordered list of service names for stable messaging.
func evaluateSystemdServicesOutput(services []string, statuses map[string]string) (CheckStatus, string) {
	var running, stopped []string
	for _, svc := range services {
		if statuses[svc] == "active" {
			running = append(running, svc)
		} else {
			stopped = append(stopped, svc)
		}
	}

	switch {
	case len(running) == len(services):
		return StatusOK, "All services running"
	case len(running) > 0:
		return StatusWarning, fmt.Sprintf("Some services stopped: %s", strings.Join(stopped, ", "))
	default:
		return StatusWarning, "No LyreBird services running"
	}
}

// evaluateProcessRestarts counts "Started" occurrences in journalctl output
// and returns a stability verdict.
func evaluateProcessRestarts(journalOutput string) (CheckStatus, string) {
	restarts := strings.Count(journalOutput, "Started")
	if restarts > 3 {
		return StatusWarning, fmt.Sprintf("MediaMTX restarted %d times in last hour", restarts)
	}
	return StatusOK, "Services stable"
}
