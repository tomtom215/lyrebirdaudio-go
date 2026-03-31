// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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

	rulesPath := filepath.Join(r.opts.UdevRulesDir, "99-usb-soundcards.rules")
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

	lockDir := r.opts.LockDir
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

	result.Status, result.Message = evaluateTimeSyncOutput(string(out))

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
	statuses := make(map[string]string, len(services))
	for _, svc := range services {
		// #nosec G204 -- svc is from hardcoded list, not user input
		out, _ := exec.CommandContext(ctx, "systemctl", "is-active", svc).Output()
		statuses[svc] = strings.TrimSpace(string(out))
	}
	result.Status, result.Message = evaluateSystemdServicesOutput(services, statuses)

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

	result.Status, result.Message = evaluateProcessRestarts(string(out))

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkEntropy(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Entropy",
		Category: "System",
	}

	data, err := os.ReadFile(r.opts.ProcFS + "/sys/kernel/random/entropy_avail")
	if err != nil {
		result.Status = StatusOK
		result.Message = "Entropy check skipped"
		result.Duration = time.Since(start)
		return result
	}

	result.Status, result.Message, result.Suggestions = evaluateEntropy(string(data))

	result.Duration = time.Since(start)
	return result
}

func (r *Runner) checkInotifyLimits(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "inotify Limits",
		Category: "Resources",
	}

	data, err := os.ReadFile(r.opts.ProcFS + "/sys/fs/inotify/max_user_watches")
	if err != nil {
		result.Status = StatusOK
		result.Message = "inotify check skipped"
		result.Duration = time.Since(start)
		return result
	}

	result.Status, result.Message, result.Suggestions = evaluateInotifyLimits(string(data))

	result.Duration = time.Since(start)
	return result
}
