// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/udev"
)

// runSetup runs interactive setup wizard (stub for now).
func runSetup(args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("setup requires root privileges (run with sudo)")
	}

	// Parse flags
	autoMode := false
	for _, arg := range args {
		if arg == "--auto" || arg == "-y" {
			autoMode = true
		}
	}

	fmt.Println("LyreBirdAudio Setup Wizard")
	fmt.Println("==========================")
	fmt.Println()

	// Step 1: Check prerequisites
	fmt.Println("Step 1: Checking prerequisites...")
	prereqsOK := true

	// Check FFmpeg
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Println("  [!] FFmpeg not found - required for audio encoding")
		fmt.Println("      Install with: sudo apt-get install ffmpeg")
		prereqsOK = false
	} else {
		fmt.Println("  [✓] FFmpeg installed")
	}

	// Check ALSA
	if _, err := os.Stat("/proc/asound"); os.IsNotExist(err) {
		fmt.Println("  [!] ALSA not available - required for audio capture")
		prereqsOK = false
	} else {
		fmt.Println("  [✓] ALSA available")
	}

	if !prereqsOK && !autoMode {
		fmt.Println()
		fmt.Println("Some prerequisites are missing. Continue anyway? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			return fmt.Errorf("setup cancelled - install missing prerequisites first")
		}
	}
	fmt.Println()

	// Step 2: Install MediaMTX
	fmt.Println("Step 2: MediaMTX RTSP Server")
	if _, err := exec.LookPath("mediamtx"); err == nil {
		fmt.Println("  [✓] MediaMTX already installed")
	} else {
		if autoMode || promptYesNo("  Install MediaMTX?") {
			fmt.Println("  Installing MediaMTX...")
			if err := runInstallMediaMTX([]string{}); err != nil {
				fmt.Printf("  [!] MediaMTX installation failed: %v\n", err)
				if !autoMode {
					fmt.Println("  Continue anyway? [y/N]: ")
					var response string
					_, _ = fmt.Scanln(&response)
					if strings.ToLower(response) != "y" {
						return err
					}
				}
			} else {
				fmt.Println("  [✓] MediaMTX installed")
			}
		} else {
			fmt.Println("  [!] Skipping MediaMTX installation")
		}
	}
	fmt.Println()

	// Step 3: Detect USB audio devices
	fmt.Println("Step 3: Detecting USB Audio Devices")
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil {
		fmt.Printf("  [!] Device detection failed: %v\n", err)
	} else if len(devices) == 0 {
		fmt.Println("  [!] No USB audio devices found")
		fmt.Println("      Connect USB microphones and try again")
	} else {
		fmt.Printf("  [✓] Found %d USB audio device(s):\n", len(devices))
		for _, dev := range devices {
			fmt.Printf("      - Card %d: %s\n", dev.CardNumber, dev.Name)
		}
	}
	fmt.Println()

	// Step 4: Create udev rules
	fmt.Println("Step 4: USB Device Mapping (udev rules)")
	if _, err := os.Stat(udev.RulesFilePath); err == nil {
		fmt.Printf("  [✓] udev rules already exist (%s)\n", udev.RulesFilePath)
	} else if len(devices) > 0 {
		if autoMode || promptYesNo("  Create udev rules for persistent device mapping?") {
			fmt.Println("  Creating udev rules...")
			if err := runUSBMapWithPath("/proc/asound", []string{"--no-reload"}); err != nil {
				fmt.Printf("  [!] udev rule creation failed: %v\n", err)
			} else {
				fmt.Println("  [✓] udev rules created")
			}
		} else {
			fmt.Println("  [!] Skipping udev rules")
		}
	} else {
		fmt.Println("  [!] Skipping - no devices to map")
	}
	fmt.Println()

	// Step 5: Create default config if needed
	fmt.Println("Step 5: Configuration")
	if _, err := os.Stat(defaultConfigPath); err == nil {
		fmt.Printf("  [✓] Configuration exists (%s)\n", defaultConfigPath)
	} else {
		if autoMode || promptYesNo("  Create default configuration?") {
			fmt.Println("  Creating default configuration...")
			cfg := config.DefaultConfig()

			// Add detected devices to config
			for _, dev := range devices {
				devName := audio.SanitizeDeviceName(dev.Name)
				cfg.Devices[devName] = config.DeviceConfig{
					SampleRate: 48000,
					Channels:   2,
					Bitrate:    "128k",
					Codec:      "opus",
				}
			}

			// Ensure directory exists
			if err := os.MkdirAll(filepath.Dir(defaultConfigPath), 0750); err != nil { // #nosec G301 -- config dir needs to be readable
				fmt.Printf("  [!] Failed to create config directory: %v\n", err)
			} else {
				// P-5 fix: Backup existing config before overwriting.
				setupBackupDir := config.GetBackupDir(defaultConfigPath)
				if _, statErr := os.Stat(defaultConfigPath); statErr == nil {
					if bkPath, bkErr := config.BackupConfig(defaultConfigPath, setupBackupDir); bkErr != nil {
						fmt.Printf("  [!] Warning: failed to backup existing config: %v\n", bkErr)
					} else {
						fmt.Printf("  [✓] Backed up existing config to %s\n", bkPath)
					}
				}
				if err := cfg.Save(defaultConfigPath); err != nil {
					fmt.Printf("  [!] Failed to save configuration: %v\n", err)
				} else {
					fmt.Printf("  [✓] Configuration saved to %s\n", defaultConfigPath)
				}
			}
		} else {
			fmt.Println("  [!] Skipping configuration creation")
		}
	}
	fmt.Println()

	// Step 6: Install systemd service
	fmt.Println("Step 6: Systemd Service")
	servicePath := "/etc/systemd/system/lyrebird-stream.service"
	if _, err := os.Stat(servicePath); err == nil {
		fmt.Println("  [✓] Service already installed")
	} else {
		if autoMode || promptYesNo("  Install lyrebird-stream service?") {
			fmt.Println("  Installing systemd service...")
			if err := installLyreBirdService(); err != nil {
				fmt.Printf("  [!] Service installation failed: %v\n", err)
			} else {
				fmt.Println("  [✓] Service installed")
			}
		} else {
			fmt.Println("  [!] Skipping service installation")
		}
	}
	fmt.Println()

	// Summary
	fmt.Println("Setup Complete!")
	fmt.Println("===============")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Start MediaMTX:         sudo systemctl start mediamtx")
	fmt.Println("  2. Start streaming:        sudo systemctl start lyrebird-stream")
	fmt.Println("  3. Enable on boot:         sudo systemctl enable mediamtx lyrebird-stream")
	fmt.Println("  4. Check status:           lyrebird status")
	fmt.Println()
	if len(devices) > 0 {
		fmt.Println("Stream URLs:")
		for _, dev := range devices {
			devName := audio.SanitizeDeviceName(dev.Name)
			fmt.Printf("  rtsp://localhost:8554/%s\n", devName)
		}
	}

	return nil
}

// promptYesNo displays a yes/no prompt and returns true for yes.
func promptYesNo(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	var response string
	_, _ = fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}

// lyrebirdServiceContent is the full systemd service file content.
//
// This MUST be kept byte-for-byte identical to systemd/lyrebird-stream.service
// at the repository root. TestInstallLyreBirdServiceMatchesSystemdFile in
// main_test.go asserts both are identical whenever the test can locate the file.
var lyrebirdServiceContent = `# LyreBirdAudio Stream Manager Service
#
# This service manages audio streaming from USB devices to RTSP via MediaMTX.
# It automatically detects USB audio devices and maintains persistent streams
# with automatic restart on failure.
#
# Installation:
#   sudo cp lyrebird-stream.service /etc/systemd/system/
#   sudo systemctl daemon-reload
#   sudo systemctl enable lyrebird-stream
#   sudo systemctl start lyrebird-stream
#
# Configuration:
#   Primary: /etc/lyrebird/config.yaml
#   Overrides: /etc/lyrebird/environment (optional)
#
# Logs: journalctl -u lyrebird-stream -f
#
# Hot-reload configuration (no restart required):
#   sudo systemctl reload lyrebird-stream
#
# User Configuration:
#   The service runs as root by default for reliable ALSA device access.
#   To run as a dedicated user (recommended for production):
#   1. Create user: sudo useradd -r -s /usr/sbin/nologin -G audio lyrebird
#   2. Create directories: sudo mkdir -p /var/run/lyrebird /etc/lyrebird
#   3. Set ownership: sudo chown lyrebird:audio /var/run/lyrebird
#   4. Change User=root to User=lyrebird in this file
#   5. Reload: sudo systemctl daemon-reload && sudo systemctl restart lyrebird-stream

[Unit]
Description=LyreBirdAudio Stream Manager
Documentation=https://github.com/tomtom215/lyrebirdaudio-go
After=network.target sound.target mediamtx.service
Wants=mediamtx.service
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=simple
User=root
Group=audio

# Optional environment file for configuration overrides
# Create /etc/lyrebird/environment with:
#   LYREBIRD_CONFIG=/etc/lyrebird/config.yaml
#   LYREBIRD_LOG_LEVEL=info
EnvironmentFile=-/etc/lyrebird/environment

# Default configuration (can be overridden via environment file)
Environment=LYREBIRD_CONFIG=/etc/lyrebird/config.yaml
Environment=LYREBIRD_LOG_LEVEL=info

# Main executable
ExecStart=/usr/local/bin/lyrebird-stream --config=${LYREBIRD_CONFIG} --log-level=${LYREBIRD_LOG_LEVEL}

# Hot-reload configuration without stopping streams
ExecReload=/bin/kill -HUP $MAINPID

# Graceful shutdown
ExecStop=/bin/kill -SIGTERM $MAINPID
TimeoutStopSec=30

# Restart policy
Restart=always
RestartSec=10

# GAP-2 / A-5: systemd watchdog integration.
# The daemon calls sd_notify("WATCHDOG=1") every ~45s via the NOTIFY_SOCKET.
# If the Go runtime deadlocks (goroutine holding a mutex, channel with no receiver,
# etc.), the heartbeat stops and systemd restarts the unit after WatchdogSec.
# NotifyAccess=main is required for Type=simple to receive notifications.
WatchdogSec=90s
NotifyAccess=main

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=yes
RestrictNamespaces=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
RestrictRealtime=yes
SystemCallFilter=@system-service
SystemCallArchitectures=native

# Allow access to audio devices
SupplementaryGroups=audio
DeviceAllow=/dev/snd/* rw
DevicePolicy=closed

# Allow access to required paths
ReadWritePaths=/var/run/lyrebird /var/lib/lyrebird
ReadOnlyPaths=/etc/lyrebird /proc/asound

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

# P-10 fix: Memory limits to prevent OOM on resource-constrained devices
# (e.g., Raspberry Pi with 1-4 GB RAM and multiple USB microphones).
# MemoryHigh triggers memory pressure reclaim; MemoryMax is a hard kill limit.
# 512M is conservative for audio-only FFmpeg streams (typically 20-50 MB each).
MemoryHigh=384M
MemoryMax=512M

# Environment
Environment=HOME=/var/run/lyrebird

[Install]
WantedBy=multi-user.target
`

// installLyreBirdService installs the lyrebird-stream systemd service.
func installLyreBirdService() error {
	return installLyreBirdServiceToPath("/etc/systemd/system/lyrebird-stream.service")
}

// installLyreBirdServiceToPath writes the lyrebird-stream service file to path
// and reloads systemd. Separated for testability.
func installLyreBirdServiceToPath(servicePath string) error {
	// #nosec G306 - systemd service files should be world-readable
	if err := os.WriteFile(servicePath, []byte(lyrebirdServiceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	reloadCmd := exec.Command("systemctl", "daemon-reload") // #nosec G204 -- "systemctl" is a literal
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w: %s", err, string(output))
	}

	return nil
}
