// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// checkKernelModules verifies that required audio kernel modules are loaded.
func (r *Runner) checkKernelModules(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Kernel Modules",
		Category: "System",
	}

	data, err := os.ReadFile(r.opts.ProcFS + "/modules")
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("failed to read /proc/modules: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	requiredModules := []string{"snd_usb_audio"}
	optionalModules := []string{"snd_pcm", "snd_hwdep", "snd_usbmidi_lib"}

	result.Status, result.Message = evaluateKernelModules(string(data), requiredModules, optionalModules)
	if result.Status == StatusCritical {
		result.Suggestions = append(result.Suggestions, "Load module: sudo modprobe snd_usb_audio")
	}

	result.Duration = time.Since(start)
	return result
}

// checkDevicePermissions verifies /dev/snd/* permissions for non-root users.
func (r *Runner) checkDevicePermissions(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Device Permissions",
		Category: "Audio",
	}

	matches, err := filepath.Glob(r.opts.DevSndDir + "/*")
	if err != nil || len(matches) == 0 {
		result.Status = StatusWarning
		result.Message = "No /dev/snd devices found"
		result.Suggestions = append(result.Suggestions, "Ensure ALSA is installed and a sound card is present")
		result.Duration = time.Since(start)
		return result
	}

	var unreadable []string
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		mode := info.Mode()
		if mode&0060 == 0 {
			unreadable = append(unreadable, filepath.Base(path))
		}
	}

	if len(unreadable) > 0 {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("%d device(s) not group-accessible: %s", len(unreadable), strings.Join(unreadable, ", "))
		result.Suggestions = append(result.Suggestions,
			"Ensure user is in 'audio' group: sudo usermod -a -G audio $USER")
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("All %d /dev/snd devices have group access", len(matches))
	}

	result.Duration = time.Since(start)
	return result
}

// checkFFmpegCodecs verifies that required audio codecs are available in FFmpeg.
func (r *Runner) checkFFmpegCodecs(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "FFmpeg Codecs",
		Category: "Audio",
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		result.Status = StatusSkipped
		result.Message = "FFmpeg not found, cannot check codecs"
		result.Duration = time.Since(start)
		return result
	}

	requiredEncoders := map[string]string{
		"libopus": "Opus encoder (primary codec)",
		"aac":     "AAC encoder (fallback codec)",
	}
	requiredDecoders := map[string]string{
		"pcm_s16le": "PCM S16 LE decoder (ALSA input)",
	}

	// #nosec G204 -- ffmpegPath from exec.LookPath
	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("failed to query FFmpeg encoders: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// #nosec G204 -- ffmpegPath from exec.LookPath
	cmd2 := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-decoders")
	output2, err := cmd2.Output()
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("failed to query FFmpeg decoders: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Status, result.Message, result.Details = evaluateCodecsOutput(
		string(output), string(output2), requiredEncoders, requiredDecoders,
	)
	if result.Status == StatusCritical {
		result.Suggestions = append(result.Suggestions,
			"Reinstall FFmpeg with full codec support: apt-get install ffmpeg")
	}

	result.Duration = time.Since(start)
	return result
}

// checkUSBStability checks kernel dmesg for recent USB disconnect/error events.
func (r *Runner) checkUSBStability(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "USB Stability",
		Category: "Audio",
	}

	// #nosec G204 -- fixed command arguments
	cmd := exec.CommandContext(ctx, "dmesg", "--time-format=iso", "-l", "err,warn")
	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusSkipped
		result.Message = "Cannot read dmesg (may require root)"
		result.Duration = time.Since(start)
		return result
	}

	result.Status, result.Message, result.Details = evaluateUSBStability(string(output))
	if result.Status == StatusWarning {
		result.Suggestions = append(result.Suggestions,
			"Check USB cable and hub connections",
			"Try a powered USB hub for bus-powered devices")
	}

	result.Duration = time.Since(start)
	return result
}

// checkLockFilePermissions validates lock directory ownership and permissions.
func (r *Runner) checkLockFilePermissions(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Lock File Permissions",
		Category: "System",
	}

	lockDir := r.opts.LockDir
	info, err := os.Stat(lockDir)
	if err != nil {
		if os.IsNotExist(err) {
			result.Status = StatusOK
			result.Message = "Lock directory not yet created (will be created on daemon start)"
		} else {
			result.Status = StatusError
			result.Message = fmt.Sprintf("Cannot stat lock directory: %v", err)
		}
		result.Duration = time.Since(start)
		return result
	}

	mode := info.Mode().Perm()
	var issues []string

	if mode&0007 != 0 {
		issues = append(issues, fmt.Sprintf("world-accessible (mode %04o, expected 0750)", mode))
	}
	if !info.IsDir() {
		issues = append(issues, "not a directory")
	}

	// Check for stale lock files
	entries, _ := os.ReadDir(lockDir)
	staleCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".lock") {
			lockPath := filepath.Join(lockDir, e.Name())
			data, err := os.ReadFile(lockPath) //#nosec G304 -- lockDir is a constant
			if err != nil {
				continue
			}
			pidStr := strings.TrimSpace(string(data))
			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				staleCount++
				continue
			}
			if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); os.IsNotExist(err) {
				staleCount++
			}
		}
	}

	if staleCount > 0 {
		issues = append(issues, fmt.Sprintf("%d stale lock file(s) found", staleCount))
	}

	if len(issues) > 0 {
		result.Status = StatusWarning
		result.Message = strings.Join(issues, "; ")
		if mode&0007 != 0 {
			result.Suggestions = append(result.Suggestions,
				fmt.Sprintf("Fix permissions: sudo chmod 0750 %s", lockDir))
		}
		if staleCount > 0 {
			result.Suggestions = append(result.Suggestions,
				fmt.Sprintf("Remove stale locks: sudo rm %s/*.lock", lockDir))
		}
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Lock directory OK (mode %04o, %d lock files)", mode, len(entries))
	}

	result.Duration = time.Since(start)
	return result
}

// checkUlimits verifies that system resource limits are sufficient.
func (r *Runner) checkUlimits(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:     "Resource Limits",
		Category: "System",
	}

	softData, err := os.ReadFile(r.opts.ProcFS + "/self/limits")
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("failed to read /proc/self/limits: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Status, result.Message = evaluateResourceLimits(string(softData))
	if result.Status == StatusWarning {
		result.Suggestions = append(result.Suggestions,
			"Increase limits in /etc/security/limits.conf or systemd service LimitNOFILE=")
	}

	result.Duration = time.Since(start)
	return result
}
