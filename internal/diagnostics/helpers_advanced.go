// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"fmt"
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
