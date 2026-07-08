// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/mediamtx"
)

// statusSessionQueryTimeout caps how long `lyrebird status` will wait for the
// MediaMTX API when listing active RTSP sessions. Status is an interactive
// command and must not hang when MediaMTX is down or unreachable.
const statusSessionQueryTimeout = 2 * time.Second

// StatusOutput represents the JSON output format for status command.
type StatusOutput struct {
	ServiceStatus  string         `json:"service_status"`
	DeviceCount    int            `json:"device_count"`
	ActiveStreams  []StreamStatus `json:"active_streams"`
	AvailableURLs  []StreamURL    `json:"available_urls"`
	ActiveSessions []SessionInfo  `json:"active_sessions"`
	Error          string         `json:"error,omitempty"`
}

// SessionInfo summarises one active RTSP session reported by MediaMTX. The
// field set intentionally mirrors the subset of mediamtx.RTSPSession that is
// useful to a human operator running `lyrebird status`: who is connected,
// from where, to which path, and how much traffic has flowed.
type SessionInfo struct {
	ID            string `json:"id"`
	RemoteAddr    string `json:"remote_addr"`
	State         string `json:"state"`
	Path          string `json:"path"`
	Transport     string `json:"transport,omitempty"`
	Created       string `json:"created,omitempty"`
	InboundBytes  uint64 `json:"inbound_bytes"`
	OutboundBytes uint64 `json:"outbound_bytes"`
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

	// Collect active RTSP sessions from MediaMTX. Fail-soft: the status
	// command must work when MediaMTX is down, so any error here just
	// leaves the field as an empty (but non-nil) slice.
	status.ActiveSessions = fetchActiveSessions(cfg.MediaMTX.APIURL)

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
	fmt.Println()

	fmt.Println("Active Readers:")
	fmt.Println("---------------")
	if len(status.ActiveSessions) == 0 {
		fmt.Println("  (no active RTSP readers, or MediaMTX API unreachable)")
	} else {
		for _, s := range status.ActiveSessions {
			// Only readers are interesting here (sessions in "read" state);
			// "publish" is ffmpeg itself and would just add noise.
			if s.State != "read" {
				continue
			}
			fmt.Printf("  %s <- %s (%s, %s out)\n",
				s.Path, s.RemoteAddr, s.Transport, formatBytes(s.OutboundBytes))
		}
	}

	return nil
}

// fetchActiveSessions queries the MediaMTX API for the list of active RTSP
// sessions. It is a fail-soft helper: any error (including connection refused,
// non-2xx status, or decode failure) results in an empty (non-nil) slice. The
// function is overridable via fetchActiveSessionsFn so tests can inject a
// deterministic fake without standing up an HTTP server.
var fetchActiveSessionsFn = defaultFetchActiveSessions

func fetchActiveSessions(apiURL string) []SessionInfo {
	return fetchActiveSessionsFn(apiURL)
}

func defaultFetchActiveSessions(apiURL string) []SessionInfo {
	if apiURL == "" {
		return []SessionInfo{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusSessionQueryTimeout)
	defer cancel()

	client := mediamtx.NewClient(apiURL, mediamtx.WithTimeout(statusSessionQueryTimeout))
	sessions, err := client.ListRTSPSessions(ctx)
	if err != nil {
		return []SessionInfo{}
	}

	out := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, SessionInfo{
			ID:            s.ID,
			RemoteAddr:    s.RemoteAddr,
			State:         s.State,
			Path:          s.Path,
			Transport:     s.Transport,
			Created:       s.Created,
			InboundBytes:  s.InboundBytes,
			OutboundBytes: s.OutboundBytes,
		})
	}
	return out
}

// formatBytes formats a byte count as a short human-readable string. It is
// used only for the human-readable text output of `lyrebird status`; JSON
// output emits the raw integer.
func formatBytes(b uint64) string {
	const (
		kib = 1 << 10
		mib = 1 << 20
		gib = 1 << 30
	)
	switch {
	case b >= gib:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(gib))
	case b >= mib:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(mib))
	case b >= kib:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(kib))
	default:
		return fmt.Sprintf("%d B", b)
	}
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

	// On Unix, FindProcess always succeeds; send signal 0 to probe the process.
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true // alive and signalable by us
	}
	// EPERM means the process exists but is owned by another user — e.g. the
	// root-owned daemon when `lyrebird status` is run without sudo. Only ESRCH
	// means it is truly gone. Treating EPERM as "not running" would misreport a
	// live, healthy stream as a stale lock.
	return errors.Is(err, syscall.EPERM)
}
