package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/udev"
)

// Version information (set via ldflags during build).
var (
	Version   = "dev"
	GitCommit = "none"
	BuildDate = "unknown"
)

const (
	defaultConfigPath = "/etc/lyrebird/config.yaml"
	exitSuccess       = 0
	exitError         = 1
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitError)
	}
	os.Exit(exitSuccess)
}

// run is the main entry point, extracted for testability.
func run(args []string) error {
	if len(args) == 0 {
		return runHelp()
	}

	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "help", "--help", "-h":
		return runHelp()
	case "version", "--version", "-v":
		return runVersion()
	case "devices":
		return runDevices(commandArgs)
	case "detect":
		return runDetect(commandArgs)
	case "usb-map":
		return runUSBMap(commandArgs)
	case "migrate":
		return runMigrate(commandArgs)
	case "validate":
		return runValidate(commandArgs)
	case "status":
		return runStatus(commandArgs)
	case "setup":
		return runSetup(commandArgs)
	case "install-mediamtx":
		return runInstallMediaMTX(commandArgs)
	case "test":
		return runTest(commandArgs)
	case "diagnose":
		return runDiagnose(commandArgs)
	case "check-system":
		return runCheckSystem(commandArgs)
	default:
		return fmt.Errorf("unknown command: %s (run 'lyrebird help' for usage)", command)
	}
}

// runHelp displays usage information.
func runHelp() error {
	fmt.Printf(`LyreBirdAudio-Go v%s

USAGE:
    lyrebird [COMMAND] [OPTIONS]

COMMANDS:
    help              Show this help message
    version           Show version information
    devices           List detected USB audio devices
    detect            Detect device capabilities and optimal settings
    usb-map           Create udev rules for persistent device mapping
    migrate           Migrate configuration from bash to YAML
    validate          Validate configuration file
    status            Show stream status
    setup             Interactive setup wizard
    install-mediamtx  Install MediaMTX RTSP server
    test              Test configuration without modifying system
    diagnose          Run system diagnostics
    check-system      Check system compatibility

OPTIONS:
    --config PATH     Path to configuration file (default: %s)
    --help, -h        Show help for specific command

EXAMPLES:
    # Interactive setup
    sudo lyrebird setup

    # List detected devices
    lyrebird devices

    # Create USB device mappings
    sudo lyrebird usb-map

    # Migrate from bash configuration
    lyrebird migrate --from=/etc/mediamtx/audio-devices.conf --to=/etc/lyrebird/config.yaml

    # Validate configuration
    lyrebird validate --config=/etc/lyrebird/config.yaml

For more information, visit: https://github.com/tomtom215/lyrebirdaudio-go
`, Version, defaultConfigPath)
	return nil
}

// runVersion displays version information.
func runVersion() error {
	fmt.Printf("LyreBirdAudio-Go\n")
	fmt.Printf("  Version:    %s\n", Version)
	fmt.Printf("  Git Commit: %s\n", GitCommit)
	fmt.Printf("  Built:      %s\n", BuildDate)
	return nil
}

// runDevices lists detected USB audio devices.
func runDevices(args []string) error {
	return runDevicesWithPath("/proc/asound", args)
}

// runDevicesWithPath lists detected USB audio devices from the specified path.
// Extracted for testability.
func runDevicesWithPath(asoundPath string, args []string) error {
	// Scan for USB audio devices
	devices, err := audio.DetectDevices(asoundPath)
	if err != nil {
		return fmt.Errorf("failed to scan devices: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No USB audio devices detected")
		return nil
	}

	fmt.Printf("Found %d USB audio device(s):\n\n", len(devices))

	for _, dev := range devices {
		fmt.Printf("Device: %s\n", dev.Name)
		fmt.Printf("  ALSA ID:       hw:%d,0\n", dev.CardNumber)
		fmt.Printf("  Card Number:   %d\n", dev.CardNumber)
		fmt.Printf("  USB ID:        %s\n", dev.USBID)
		fmt.Printf("  Vendor ID:     %s\n", dev.VendorID)
		fmt.Printf("  Product ID:    %s\n", dev.ProductID)
		if dev.DeviceID != "" {
			fmt.Printf("  Device ID:     %s\n", dev.DeviceID)
		}
		fmt.Println()
	}

	return nil
}

// runDetect detects device capabilities and recommends settings.
func runDetect(args []string) error {
	return runDetectWithPath("/proc/asound", args)
}

// runDetectWithPath detects device capabilities from the specified path.
// Extracted for testability.
func runDetectWithPath(asoundPath string, args []string) error {
	// Scan for USB audio devices
	devices, err := audio.DetectDevices(asoundPath)
	if err != nil {
		return fmt.Errorf("failed to scan devices: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No USB audio devices detected")
		return nil
	}

	fmt.Printf("Detected %d device(s) with recommended settings:\n\n", len(devices))

	for _, dev := range devices {
		fmt.Printf("Device: %s\n", dev.Name)
		fmt.Printf("  ALSA ID:              hw:%d,0\n", dev.CardNumber)
		fmt.Printf("  Recommended settings:\n")
		fmt.Printf("    Sample rate:        48000 Hz\n")
		fmt.Printf("    Channels:           2 (stereo)\n")
		fmt.Printf("    Codec:              opus\n")
		fmt.Printf("    Bitrate:            128k\n")
		fmt.Println()
	}

	fmt.Println("Note: Configure per-device settings in /etc/lyrebird/config.yaml")
	return nil
}

// runUSBMap creates udev rules for persistent device mapping.
func runUSBMap(args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("usb-map requires root privileges (run with sudo)")
	}
	return runUSBMapWithPath("/proc/asound", args)
}

// runUSBMapWithPath creates udev rules from the specified path.
// Extracted for testability.
func runUSBMapWithPath(asoundPath string, args []string) error {
	// Parse flags
	dryRun := false
	reload := true
	outputPath := udev.RulesFilePath
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--dry-run":
			dryRun = true
		case args[i] == "--no-reload":
			reload = false
		case strings.HasPrefix(args[i], "--output="):
			outputPath = strings.TrimPrefix(args[i], "--output=")
		case args[i] == "--output" && i+1 < len(args):
			outputPath = args[i+1]
			i++
		}
	}

	// Detect USB audio devices
	devices, err := audio.DetectDevices(asoundPath)
	if err != nil {
		return fmt.Errorf("failed to detect devices: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No USB audio devices found to map")
		return nil
	}

	fmt.Printf("Found %d USB audio device(s):\n", len(devices))

	// Build DeviceInfo list with actual USB port paths
	var deviceInfos []*udev.DeviceInfo
	for _, dev := range devices {
		// Get USB bus/dev numbers from sysfs
		busNum, devNum, err := getUSBBusDevFromCard(dev.CardNumber)
		if err != nil {
			fmt.Printf("  - Card %d (%s): skipping - %v\n", dev.CardNumber, dev.Name, err)
			continue
		}

		// Get physical USB port path
		portPath, product, serial, err := udev.GetUSBPhysicalPort("/sys", busNum, devNum)
		if err != nil {
			fmt.Printf("  - Card %d (%s): skipping - %v\n", dev.CardNumber, dev.Name, err)
			continue
		}

		displayName := dev.Name
		if product != "" {
			displayName = product
		}

		fmt.Printf("  - Card %d (%s): port %s (bus %d, dev %d)\n",
			dev.CardNumber, displayName, portPath, busNum, devNum)

		info := &udev.DeviceInfo{
			PortPath: portPath,
			BusNum:   busNum,
			DevNum:   devNum,
			Product:  displayName,
			Serial:   serial,
		}
		deviceInfos = append(deviceInfos, info)
	}

	if len(deviceInfos) == 0 {
		fmt.Println("\nNo valid devices to map. Check that USB devices are properly connected.")
		return nil
	}

	fmt.Println()

	if dryRun {
		fmt.Printf("Dry run - would write to %s:\n\n", outputPath)
		// Generate and display content
		for _, info := range deviceInfos {
			fmt.Println(info.GenerateRule())
		}
		fmt.Println("\nTo apply these rules, run without --dry-run")
		return nil
	}

	// Write the rules file
	fmt.Printf("Writing udev rules to %s...\n", outputPath)
	if err := udev.WriteRulesFileToPath(deviceInfos, outputPath, reload); err != nil {
		return fmt.Errorf("failed to write rules file: %w", err)
	}

	fmt.Println("Rules written successfully!")
	if reload {
		fmt.Println("udev rules reloaded and triggered.")
	} else {
		fmt.Println("\nTo activate rules manually:")
		fmt.Println("  sudo udevadm control --reload-rules && sudo udevadm trigger")
	}
	fmt.Println("\nDevice symlinks will appear at /dev/snd/by-usb-port/")

	return nil
}

// getUSBBusDevFromCard extracts USB bus and device numbers for an ALSA card.
func getUSBBusDevFromCard(cardNum int) (busNum, devNum int, err error) {
	// The USB device info is available via sysfs
	// /sys/class/sound/card{N}/device -> links to the USB device
	cardPath := fmt.Sprintf("/sys/class/sound/card%d/device", cardNum)

	// Resolve the symlink to get the actual device path
	devicePath, err := filepath.EvalSymlinks(cardPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to resolve card device path: %w", err)
	}

	// Walk up to find the USB device (look for busnum/devnum files)
	for {
		busnumPath := filepath.Join(devicePath, "busnum")
		devnumPath := filepath.Join(devicePath, "devnum")

		if _, err := os.Stat(busnumPath); err == nil {
			// Found the USB device directory
			busnumData, err := os.ReadFile(busnumPath)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to read busnum: %w", err)
			}
			devnumData, err := os.ReadFile(devnumPath)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to read devnum: %w", err)
			}

			if _, err := fmt.Sscanf(strings.TrimSpace(string(busnumData)), "%d", &busNum); err != nil {
				continue // Try parent directory
			}
			if _, err := fmt.Sscanf(strings.TrimSpace(string(devnumData)), "%d", &devNum); err != nil {
				continue // Try parent directory
			}

			if busNum > 0 && devNum > 0 {
				return busNum, devNum, nil
			}
		}

		// Move up one directory
		parent := filepath.Dir(devicePath)
		if parent == devicePath || parent == "/" || parent == "/sys" {
			break
		}
		devicePath = parent
	}

	return 0, 0, fmt.Errorf("USB bus/dev numbers not found for card %d", cardNum)
}

// runMigrate migrates configuration from bash to YAML.
func runMigrate(args []string) error {
	// Parse flags
	fromPath := ""
	toPath := defaultConfigPath
	force := false

	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--from="):
			fromPath = strings.TrimPrefix(args[i], "--from=")
		case args[i] == "--from" && i+1 < len(args):
			fromPath = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--to="):
			toPath = strings.TrimPrefix(args[i], "--to=")
		case args[i] == "--to" && i+1 < len(args):
			toPath = args[i+1]
			i++
		case args[i] == "--force":
			force = true
		}
	}

	if fromPath == "" {
		return fmt.Errorf("--from path is required")
	}

	// Check if destination exists
	if _, err := os.Stat(toPath); err == nil && !force {
		return fmt.Errorf("destination file exists (use --force to overwrite): %s", toPath)
	}

	// Migrate
	fmt.Printf("Migrating configuration...\n")
	fmt.Printf("  From: %s\n", fromPath)
	fmt.Printf("  To:   %s\n\n", toPath)

	cfg, err := config.MigrateFromBash(fromPath)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Validate migrated config
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("migrated config is invalid: %w", err)
	}

	// Create directory if needed
	// #nosec G301 - Config directory needs 0755 for system access
	if err := os.MkdirAll(filepath.Dir(toPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Save
	if err := cfg.Save(toPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Migration complete\n")
	fmt.Printf("✓ Migrated %d device(s)\n", len(cfg.Devices))
	fmt.Println("\nRun 'lyrebird validate' to verify the configuration")

	return nil
}

// runValidate validates a configuration file.
func runValidate(args []string) error {
	configPath := defaultConfigPath

	// Parse flags
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--config" && i+1 < len(args):
			configPath = args[i+1]
			i++
		}
	}

	fmt.Printf("Validating configuration: %s\n\n", configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Println("✓ Configuration is valid")
	fmt.Printf("✓ Loaded %d device configuration(s)\n", len(cfg.Devices))

	// Show summary
	if len(cfg.Devices) > 0 {
		fmt.Println("\nConfigured devices:")
		for name := range cfg.Devices {
			fmt.Printf("  - %s\n", name)
		}
	}

	return nil
}

// runStatus shows stream status (stub for now).
func runStatus(args []string) error {
	// Parse flags
	lockDir := "/var/run/lyrebird"
	configPath := defaultConfigPath
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--lock-dir="):
			lockDir = strings.TrimPrefix(args[i], "--lock-dir=")
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		}
	}

	fmt.Println("LyreBird Stream Status")
	fmt.Println("======================")
	fmt.Println()

	// Check systemd service status
	fmt.Print("Service: ")
	serviceStatus := getServiceStatus("lyrebird-stream")
	fmt.Println(serviceStatus)
	fmt.Println()

	// Load config for MediaMTX settings
	cfg, _ := config.LoadConfig(configPath)
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	// Detect current devices
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil {
		fmt.Printf("Device detection: error - %v\n", err)
	} else {
		fmt.Printf("Detected Devices: %d USB audio device(s)\n", len(devices))
	}
	fmt.Println()

	// Check lock files for active streams
	fmt.Println("Active Streams:")
	fmt.Println("---------------")

	locks, err := filepath.Glob(filepath.Join(lockDir, "*.lock"))
	if err != nil || len(locks) == 0 {
		fmt.Println("  (no active streams)")
	} else {
		for _, lockFile := range locks {
			deviceName := strings.TrimSuffix(filepath.Base(lockFile), ".lock")
			pid, err := readLockPID(lockFile)
			if err != nil {
				fmt.Printf("  %s: unknown (lock file error)\n", deviceName)
				continue
			}

			if pid > 0 && processExists(pid) {
				fmt.Printf("  %s: running (PID %d)\n", deviceName, pid)
			} else {
				fmt.Printf("  %s: stale lock (PID %d not running)\n", deviceName, pid)
			}
		}
	}
	fmt.Println()

	// Show RTSP URLs
	fmt.Println("Stream URLs:")
	fmt.Println("------------")
	if len(devices) == 0 {
		fmt.Println("  (no devices to stream)")
	} else {
		for _, dev := range devices {
			devName := audio.SanitizeDeviceName(dev.Name)
			url := fmt.Sprintf("%s/%s", cfg.MediaMTX.RTSPURL, devName)
			fmt.Printf("  %s: %s\n", devName, url)
		}
	}

	return nil
}

// getServiceStatus checks systemd service status.
func getServiceStatus(serviceName string) string {
	// Try to run systemctl is-active
	cmd := exec.Command("systemctl", "is-active", serviceName)
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
	data, err := os.ReadFile(lockFile) // #nosec G304 -- lock files are in controlled directory
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
			} else if err := cfg.Save(defaultConfigPath); err != nil {
				fmt.Printf("  [!] Failed to save configuration: %v\n", err)
			} else {
				fmt.Printf("  [✓] Configuration saved to %s\n", defaultConfigPath)
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

// installLyreBirdService installs the lyrebird-stream systemd service.
func installLyreBirdService() error {
	serviceContent := `[Unit]
Description=LyreBirdAudio Stream Manager
Documentation=https://github.com/tomtom215/lyrebirdaudio-go
After=network.target sound.target mediamtx.service
Wants=mediamtx.service

[Service]
Type=simple
ExecStart=/usr/local/bin/lyrebird-stream --config=/etc/lyrebird/config.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`
	servicePath := "/etc/systemd/system/lyrebird-stream.service"
	// #nosec G306 - systemd service files should be world-readable
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	reloadCmd := exec.Command("systemctl", "daemon-reload")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w: %s", err, string(output))
	}

	return nil
}

// runInstallMediaMTX installs MediaMTX RTSP server.
func runInstallMediaMTX(args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("install-mediamtx requires root privileges (run with sudo)")
	}

	// Parse flags
	version := "v1.9.3" // Known stable version
	installService := true
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--version="):
			version = strings.TrimPrefix(arg, "--version=")
		case arg == "--no-service":
			installService = false
		}
	}

	fmt.Println("MediaMTX Installation")
	fmt.Println("=====================")
	fmt.Println()

	// Detect architecture
	arch := detectArch()
	fmt.Printf("Detected architecture: %s\n", arch)

	if arch == "" {
		return fmt.Errorf("unsupported architecture")
	}

	// Check if already installed
	if existingPath, err := exec.LookPath("mediamtx"); err == nil {
		fmt.Printf("MediaMTX already installed at: %s\n", existingPath)
		fmt.Print("Reinstall? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Installation cancelled.")
			return nil
		}
	}

	// Construct download URL
	downloadURL := fmt.Sprintf(
		"https://github.com/bluenviron/mediamtx/releases/download/%s/mediamtx_%s_linux_%s.tar.gz",
		version, version, arch,
	)

	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Download URL: %s\n", downloadURL)
	fmt.Println()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mediamtx-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "mediamtx.tar.gz")

	// Download using curl or wget
	fmt.Println("Downloading MediaMTX...")
	if err := downloadFile(downloadURL, tarPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	fmt.Println("Download complete.")

	// Extract
	fmt.Println("Extracting...")
	extractCmd := exec.Command("tar", "-xzf", tarPath, "-C", tmpDir) // #nosec G204 -- tarPath and tmpDir are controlled
	if output, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extraction failed: %w: %s", err, string(output))
	}

	// Install binary
	binaryPath := filepath.Join(tmpDir, "mediamtx")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("mediamtx binary not found in archive")
	}

	fmt.Println("Installing to /usr/local/bin/mediamtx...")
	installCmd := exec.Command("install", "-m", "755", binaryPath, "/usr/local/bin/mediamtx") // #nosec G204 -- binaryPath is from controlled tmpDir
	if output, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("installation failed: %w: %s", err, string(output))
	}

	// Install config if it doesn't exist
	configSrc := filepath.Join(tmpDir, "mediamtx.yml")
	configDst := "/etc/mediamtx/mediamtx.yml"
	if _, err := os.Stat(configDst); os.IsNotExist(err) {
		fmt.Printf("Installing default config to %s...\n", configDst)
		if err := os.MkdirAll("/etc/mediamtx", 0750); err != nil { // #nosec G301 -- config dir needs to be readable
			fmt.Printf("Warning: failed to create config directory: %v\n", err)
		} else if _, err := os.Stat(configSrc); err == nil {
			copyCmd := exec.Command("cp", configSrc, configDst) // #nosec G204 -- paths are from controlled tmpDir
			if output, err := copyCmd.CombinedOutput(); err != nil {
				fmt.Printf("Warning: failed to copy config: %v: %s\n", err, string(output))
			}
		}
	} else {
		fmt.Printf("Config already exists at %s, keeping existing.\n", configDst)
	}

	// Install systemd service
	if installService {
		fmt.Println("Installing systemd service...")
		if err := installMediaMTXService(); err != nil {
			fmt.Printf("Warning: failed to install systemd service: %v\n", err)
			fmt.Println("You can start MediaMTX manually with: mediamtx")
		} else {
			fmt.Println("Systemd service installed.")
			fmt.Println("Start with: sudo systemctl start mediamtx")
			fmt.Println("Enable on boot: sudo systemctl enable mediamtx")
		}
	}

	fmt.Println()
	fmt.Println("MediaMTX installation complete!")
	fmt.Println()
	fmt.Println("Default RTSP URL: rtsp://localhost:8554")
	fmt.Println("API URL: http://localhost:9997")

	return nil
}

// detectArch returns the MediaMTX architecture string for the current system.
func detectArch() string {
	cmd := exec.Command("uname", "-m")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	machine := strings.TrimSpace(string(output))
	switch machine {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "armhf":
		return "armv7"
	case "armv6l":
		return "armv6"
	default:
		return ""
	}
}

// downloadFile downloads a file from URL to destination path.
func downloadFile(url, dest string) error {
	// Try curl first
	if _, err := exec.LookPath("curl"); err == nil {
		cmd := exec.Command("curl", "-fsSL", "-o", dest, url)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("curl failed: %w: %s", err, string(output))
		}
		return nil
	}

	// Fall back to wget
	if _, err := exec.LookPath("wget"); err == nil {
		cmd := exec.Command("wget", "-q", "-O", dest, url)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wget failed: %w: %s", err, string(output))
		}
		return nil
	}

	return fmt.Errorf("neither curl nor wget found - install one of them first")
}

// installMediaMTXService installs the MediaMTX systemd service.
func installMediaMTXService() error {
	serviceContent := `[Unit]
Description=MediaMTX RTSP Server
Documentation=https://github.com/bluenviron/mediamtx
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mediamtx /etc/mediamtx/mediamtx.yml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	servicePath := "/etc/systemd/system/mediamtx.service"
	// #nosec G306 - systemd service files should be world-readable
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	reloadCmd := exec.Command("systemctl", "daemon-reload")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w: %s", err, string(output))
	}

	return nil
}

// runTest tests configuration without modifying system.
//
// Tests:
//   1. Config file syntax and validation
//   2. Device availability
//   3. FFmpeg command generation
//   4. MediaMTX connectivity
//   5. RTSP URL accessibility
func runTest(args []string) error {
	configPath := defaultConfigPath
	verbose := false

	// Parse flags
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--config" && i+1 < len(args):
			configPath = args[i+1]
			i++
		case args[i] == "-v" || args[i] == "--verbose":
			verbose = true
		}
	}

	fmt.Printf("Testing configuration: %s\n\n", configPath)

	allPassed := true

	// Test 1: Config syntax and validation
	fmt.Print("[1/5] Config syntax: ")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("FAILED\n      %v\n", err)
		allPassed = false
		// Can't continue without valid config
		return fmt.Errorf("config test failed: %w", err)
	}
	fmt.Println("OK")
	if verbose {
		fmt.Printf("      Default: %dHz, %dch, %s, %s\n",
			cfg.Default.SampleRate, cfg.Default.Channels, cfg.Default.Codec, cfg.Default.Bitrate)
		if len(cfg.Devices) > 0 {
			fmt.Printf("      Devices: %d configured\n", len(cfg.Devices))
		}
	}

	// Test 2: Device availability
	fmt.Print("[2/5] Device availability: ")
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil || len(devices) == 0 {
		fmt.Println("WARNING - No USB audio devices found")
		if verbose {
			fmt.Println("      Connect a USB audio device to stream")
		}
	} else {
		fmt.Printf("OK (%d device(s))\n", len(devices))
		if verbose {
			for _, d := range devices {
				devCfg := cfg.GetDeviceConfig(d.FriendlyName())
				fmt.Printf("      - %s (hw:%d,0) -> %dHz, %dch, %s\n",
					d.Name, d.CardNumber, devCfg.SampleRate, devCfg.Channels, devCfg.Codec)
			}
		}
	}

	// Test 3: FFmpeg command generation
	fmt.Print("[3/5] FFmpeg command: ")
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("FAILED - FFmpeg not found")
		allPassed = false
	} else {
		// Test that FFmpeg can at least parse a basic command
		testArgs := []string{
			"-hide_banner",
			"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
			"-t", "0.1",
			"-c:a", cfg.Default.Codec,
			"-b:a", cfg.Default.Bitrate,
			"-f", "null", "-",
		}
		cmd := exec.Command(ffmpegPath, testArgs...)
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Println("WARNING - FFmpeg test failed")
			if verbose {
				fmt.Printf("      %s\n", strings.TrimSpace(string(output)))
			}
		} else {
			fmt.Println("OK")
			if verbose {
				fmt.Printf("      Codec: %s, Bitrate: %s\n", cfg.Default.Codec, cfg.Default.Bitrate)
			}
		}
	}

	// Test 4: MediaMTX connectivity
	fmt.Print("[4/5] MediaMTX API: ")
	apiURL := cfg.MediaMTX.APIURL
	if apiURL == "" {
		apiURL = "http://localhost:9997"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(apiURL + "/v3/paths/list")
	if err != nil {
		fmt.Println("WARNING - Not reachable")
		if verbose {
			fmt.Printf("      URL: %s\n", apiURL)
			fmt.Printf("      Error: %v\n", err)
		}
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Println("OK")
		} else {
			fmt.Printf("WARNING - Status %d\n", resp.StatusCode)
		}
	}

	// Test 5: RTSP URL accessibility
	fmt.Print("[5/5] RTSP port: ")
	rtspURL := cfg.MediaMTX.RTSPURL
	if rtspURL == "" {
		rtspURL = "rtsp://localhost:8554"
	}
	// Extract host:port from RTSP URL
	rtspHost := strings.TrimPrefix(rtspURL, "rtsp://")
	if idx := strings.Index(rtspHost, "/"); idx != -1 {
		rtspHost = rtspHost[:idx]
	}
	conn, err := net.DialTimeout("tcp", rtspHost, 2*time.Second)
	if err != nil {
		fmt.Println("WARNING - Not accessible")
		if verbose {
			fmt.Printf("      Address: %s\n", rtspHost)
		}
	} else {
		conn.Close()
		fmt.Println("OK")
		if verbose {
			fmt.Printf("      RTSP URL: %s\n", rtspURL)
		}
	}

	fmt.Println()
	if allPassed {
		fmt.Println("All tests passed!")
	} else {
		fmt.Println("Some tests failed. Check the output above for details.")
	}

	return nil
}

// runDiagnose runs system diagnostics (stub for now).
func runDiagnose(args []string) error {
	fmt.Println("LyreBird System Diagnostics")
	fmt.Println("===========================")
	fmt.Println()

	issues := 0

	// 1. Check FFmpeg
	fmt.Print("FFmpeg: ")
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("NOT FOUND - audio encoding will fail")
		issues++
	} else {
		// Get version
		cmd := exec.Command(ffmpegPath, "-version") // #nosec G204 -- ffmpegPath is from exec.LookPath
		output, _ := cmd.Output()
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			fmt.Println(strings.TrimSpace(lines[0]))
		} else {
			fmt.Printf("found at %s\n", ffmpegPath)
		}
	}

	// 2. Check ALSA tools
	fmt.Print("ALSA (arecord): ")
	if _, err := exec.LookPath("arecord"); err != nil {
		fmt.Println("NOT FOUND - may affect device detection")
		issues++
	} else {
		fmt.Println("OK")
	}

	// 3. Check /proc/asound
	fmt.Print("/proc/asound: ")
	if _, err := os.Stat("/proc/asound"); os.IsNotExist(err) {
		fmt.Println("NOT FOUND - ALSA not available")
		issues++
	} else {
		fmt.Println("OK")
	}

	// 4. Check USB audio devices
	fmt.Print("USB Audio Devices: ")
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil {
		fmt.Printf("error - %v\n", err)
		issues++
	} else if len(devices) == 0 {
		fmt.Println("none detected")
	} else {
		fmt.Printf("%d device(s) found\n", len(devices))
		for _, dev := range devices {
			fmt.Printf("  - Card %d: %s\n", dev.CardNumber, dev.Name)
		}
	}

	// 5. Check udev rules
	fmt.Print("udev Rules: ")
	if _, err := os.Stat(udev.RulesFilePath); os.IsNotExist(err) {
		fmt.Printf("NOT CONFIGURED (%s not found)\n", udev.RulesFilePath)
		fmt.Println("  Run 'sudo lyrebird usb-map' to create persistent device mappings")
	} else {
		fmt.Printf("OK (%s exists)\n", udev.RulesFilePath)
	}

	// 6. Check config file
	fmt.Print("Configuration: ")
	if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
		fmt.Println("using defaults (no config file)")
	} else {
		cfg, err := config.LoadConfig(defaultConfigPath)
		if err != nil {
			fmt.Printf("ERROR - %v\n", err)
			issues++
		} else {
			fmt.Printf("OK (%d device config(s))\n", len(cfg.Devices))
		}
	}

	// 7. Check systemd service
	fmt.Print("Service (lyrebird-stream): ")
	status := getServiceStatus("lyrebird-stream")
	fmt.Println(status)

	// 8. Check lock directory
	fmt.Print("Lock Directory: ")
	lockDir := "/var/run/lyrebird"
	if info, err := os.Stat(lockDir); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("NOT CREATED (%s)\n", lockDir)
			fmt.Println("  Will be created when lyrebird-stream starts")
		} else {
			fmt.Printf("ERROR - %v\n", err)
			issues++
		}
	} else if !info.IsDir() {
		fmt.Printf("ERROR - %s is not a directory\n", lockDir)
		issues++
	} else {
		fmt.Println("OK")
	}

	// 9. Check MediaMTX (optional)
	fmt.Print("MediaMTX: ")
	if _, err := exec.LookPath("mediamtx"); err != nil {
		// Check if running as a service
		cmd := exec.Command("systemctl", "is-active", "mediamtx")
		if output, _ := cmd.Output(); strings.TrimSpace(string(output)) == "active" {
			fmt.Println("running (systemd service)")
		} else {
			fmt.Println("NOT FOUND or NOT RUNNING")
			fmt.Println("  Install MediaMTX: sudo lyrebird install-mediamtx")
		}
	} else {
		fmt.Println("found in PATH")
	}

	fmt.Println()
	if issues > 0 {
		fmt.Printf("Found %d issue(s) that may affect operation.\n", issues)
		return nil
	}
	fmt.Println("All checks passed. System is ready for streaming.")
	return nil
}

// runCheckSystem checks system compatibility.
func runCheckSystem(args []string) error {
	fmt.Println("System Compatibility Check")
	fmt.Println("==========================")
	fmt.Println()

	compatible := true

	// Kernel version
	fmt.Print("Kernel: ")
	if data, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			fmt.Println(parts[2])
		}
	} else {
		fmt.Println("unknown")
	}

	// Check if running as root (for full access)
	fmt.Print("Running as root: ")
	if os.Geteuid() == 0 {
		fmt.Println("yes")
	} else {
		fmt.Println("no (some features require sudo)")
	}

	// Check audio group membership
	fmt.Print("Audio group: ")
	cmd := exec.Command("groups")
	if output, err := cmd.Output(); err == nil {
		groups := string(output)
		if strings.Contains(groups, "audio") {
			fmt.Println("member")
		} else {
			fmt.Println("NOT A MEMBER - may need: sudo usermod -a -G audio $USER")
		}
	} else {
		fmt.Println("unknown")
	}

	// Required binaries
	required := []string{"ffmpeg"}
	optional := []string{"arecord", "aplay", "udevadm", "systemctl"}

	fmt.Println()
	fmt.Println("Required Tools:")
	for _, tool := range required {
		fmt.Printf("  %s: ", tool)
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Println("MISSING")
			compatible = false
		} else {
			fmt.Println("OK")
		}
	}

	fmt.Println()
	fmt.Println("Optional Tools:")
	for _, tool := range optional {
		fmt.Printf("  %s: ", tool)
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Println("not found")
		} else {
			fmt.Println("OK")
		}
	}

	fmt.Println()
	if compatible {
		fmt.Println("System is compatible with LyreBirdAudio.")
	} else {
		fmt.Println("System is MISSING required components.")
		fmt.Println("Install FFmpeg: sudo apt-get install ffmpeg")
	}

	return nil
}

// setupSignalHandler sets up signal handling for graceful shutdown.
func setupSignalHandler() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, shutting down...")
		cancel()
	}()

	return ctx
}
