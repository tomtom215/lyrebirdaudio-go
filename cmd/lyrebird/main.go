package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

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

			fmt.Sscanf(strings.TrimSpace(string(busnumData)), "%d", &busNum)
			fmt.Sscanf(strings.TrimSpace(string(devnumData)), "%d", &devNum)

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
	data, err := os.ReadFile(lockFile)
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

	fmt.Println("Setup command not yet implemented")
	fmt.Println("\nManual setup:")
	fmt.Println("  1. lyrebird install-mediamtx")
	fmt.Println("  2. sudo lyrebird usb-map")
	fmt.Println("  3. lyrebird detect")
	fmt.Println("  4. sudo systemctl start lyrebird-stream")
	return nil
}

// runInstallMediaMTX installs MediaMTX (stub for now).
func runInstallMediaMTX(args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("install-mediamtx requires root privileges (run with sudo)")
	}

	fmt.Println("MediaMTX installation not yet implemented")
	fmt.Println("\nManual installation:")
	fmt.Println("  wget https://github.com/bluenviron/mediamtx/releases/latest/download/mediamtx_linux_amd64.tar.gz")
	fmt.Println("  tar -xzf mediamtx_linux_amd64.tar.gz")
	fmt.Println("  sudo mv mediamtx /usr/local/bin/")
	return nil
}

// runTest tests configuration without modifying system (stub for now).
func runTest(args []string) error {
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

	fmt.Printf("Testing configuration: %s\n", configPath)
	fmt.Println("Test command not yet implemented")
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
		cmd := exec.Command(ffmpegPath, "-version")
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
