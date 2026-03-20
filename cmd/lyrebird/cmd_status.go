// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

// StatusOutput represents the JSON output format for status command.
type StatusOutput struct {
	ServiceStatus string         `json:"service_status"`
	DeviceCount   int            `json:"device_count"`
	ActiveStreams []StreamStatus `json:"active_streams"`
	AvailableURLs []StreamURL    `json:"available_urls"`
	Error         string         `json:"error,omitempty"`
}

// StreamStatus represents the status of an individual stream.
type StreamStatus struct {
	DeviceName string `json:"device_name"`
	Status     string `json:"status"`
	PID        int    `json:"pid,omitempty"`
}

// StreamURL represents an available RTSP URL.
type StreamURL struct {
	DeviceName string `json:"device_name"`
	URL        string `json:"url"`
}

// runStatus shows stream status.
func runStatus(args []string) error {
	// Parse flags
	lockDir := "/var/run/lyrebird"
	configPath := defaultConfigPath
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--lock-dir="):
			lockDir = strings.TrimPrefix(args[i], "--lock-dir=")
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--json" || args[i] == "-j":
			jsonOutput = true
		}
	}

	// Collect status data
	status := StatusOutput{}

	// Check systemd service status
	status.ServiceStatus = getServiceStatus("lyrebird-stream")

	// Load config for MediaMTX settings
	cfg, _ := config.LoadConfig(configPath)
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	// Detect current devices
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil {
		status.Error = fmt.Sprintf("device detection error: %v", err)
	} else {
		status.DeviceCount = len(devices)
	}

	// Check lock files for active streams
	status.ActiveStreams = []StreamStatus{}
	locks, _ := filepath.Glob(filepath.Join(lockDir, "*.lock"))
	for _, lockFile := range locks {
		deviceName := strings.TrimSuffix(filepath.Base(lockFile), ".lock")
		pid, err := readLockPID(lockFile)
		if err != nil {
			status.ActiveStreams = append(status.ActiveStreams, StreamStatus{
				DeviceName: deviceName,
				Status:     "unknown",
			})
			continue
		}

		if pid > 0 && processExists(pid) {
			status.ActiveStreams = append(status.ActiveStreams, StreamStatus{
				DeviceName: deviceName,
				Status:     "running",
				PID:        pid,
			})
		} else {
			status.ActiveStreams = append(status.ActiveStreams, StreamStatus{
				DeviceName: deviceName,
				Status:     "stale",
				PID:        pid,
			})
		}
	}

	// Collect RTSP URLs
	status.AvailableURLs = []StreamURL{}
	for _, dev := range devices {
		devName := audio.SanitizeDeviceName(dev.Name)
		url := fmt.Sprintf("%s/%s", cfg.MediaMTX.RTSPURL, devName)
		status.AvailableURLs = append(status.AvailableURLs, StreamURL{
			DeviceName: devName,
			URL:        url,
		})
	}

	// Output based on format
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	// Text output (original format)
	fmt.Println("LyreBird Stream Status")
	fmt.Println("======================")
	fmt.Println()

	fmt.Printf("Service: %s\n", status.ServiceStatus)
	fmt.Println()

	if status.Error != "" {
		fmt.Printf("Device detection: error - %s\n", status.Error)
	} else {
		fmt.Printf("Detected Devices: %d USB audio device(s)\n", status.DeviceCount)
	}
	fmt.Println()

	fmt.Println("Active Streams:")
	fmt.Println("---------------")

	if len(status.ActiveStreams) == 0 {
		fmt.Println("  (no active streams)")
	} else {
		for _, s := range status.ActiveStreams {
			switch s.Status {
			case "running":
				fmt.Printf("  %s: running (PID %d)\n", s.DeviceName, s.PID)
			case "stale":
				fmt.Printf("  %s: stale lock (PID %d not running)\n", s.DeviceName, s.PID)
			default:
				fmt.Printf("  %s: unknown (lock file error)\n", s.DeviceName)
			}
		}
	}
	fmt.Println()

	fmt.Println("Stream URLs:")
	fmt.Println("------------")
	if len(status.AvailableURLs) == 0 {
		fmt.Println("  (no devices to stream)")
	} else {
		for _, u := range status.AvailableURLs {
			fmt.Printf("  %s: %s\n", u.DeviceName, u.URL)
		}
	}

	return nil
}

// getServiceStatus checks systemd service status.
func getServiceStatus(serviceName string) string {
	// Try to run systemctl is-active
	cmd := exec.Command("systemctl", "is-active", serviceName) // #nosec G204 G702 -- serviceName is a controlled constant, not user input
	output, err := cmd.Output()
	if err != nil {
		return "not running (or systemd unavailable)"
	}

	status := strings.TrimSpace(string(output))
	switch status {
	case "active":
		return "active (running)"
	case "inactive":
		return "inactive (stopped)"
	case "failed":
		return "failed"
	default:
		return status
	}
}

// readLockPID reads the PID from a lock file.
func readLockPID(lockFile string) (int, error) {
	data, err := os.ReadFile(lockFile) // #nosec G304 G703 -- lock files are in controlled directory
	if err != nil {
		return 0, err
	}

	var pid int
	_, err = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
	return pid, err
}

// processExists checks if a process with the given PID exists.
func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if process exists.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
