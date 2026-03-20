// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/udev"
)

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
			outputPath = args[i+1] // #nosec G602 -- bounds checked by i+1 < len(args) in the case condition
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
	return getUSBBusDevFromCardWithSysRoot(cardNum, "/sys")
}

// getUSBBusDevFromCardWithSysRoot is the injectable implementation for testing.
// sysRoot defaults to "/sys" in production.
func getUSBBusDevFromCardWithSysRoot(cardNum int, sysRoot string) (busNum, devNum int, err error) {
	// The USB device info is available via sysfs
	// {sysRoot}/class/sound/card{N}/device -> links to the USB device
	cardPath := filepath.Join(sysRoot, "class", "sound", fmt.Sprintf("card%d", cardNum), "device")

	// Resolve the symlink to get the actual device path
	devicePath, err := filepath.EvalSymlinks(cardPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to resolve card device path: %w", err)
	}

	// Walk up to find the USB device (look for busnum/devnum files).
	// NOTE: partial-state reset — busNum and devNum are reset to 0 before
	// each attempt so a failed parse in one directory cannot contaminate the
	// next iteration (replaces the old fragile `continue` idiom).
	for {
		busnumPath := filepath.Join(devicePath, "busnum")
		devnumPath := filepath.Join(devicePath, "devnum")

		if _, statErr := os.Stat(busnumPath); statErr == nil {
			// Found the USB device directory — try to read bus/dev numbers.
			busNum, devNum = 0, 0                          // reset partial state before each parse attempt
			busnumData, readErr := os.ReadFile(busnumPath) // #nosec G304 -- path derived from sysRoot + known sysfs layout
			if readErr != nil {
				return 0, 0, fmt.Errorf("failed to read busnum: %w", readErr)
			}
			devnumData, readErr := os.ReadFile(devnumPath) // #nosec G304 -- path derived from sysRoot + known sysfs layout
			if readErr != nil {
				return 0, 0, fmt.Errorf("failed to read devnum: %w", readErr)
			}

			_, busParseErr := fmt.Sscanf(strings.TrimSpace(string(busnumData)), "%d", &busNum)
			_, devParseErr := fmt.Sscanf(strings.TrimSpace(string(devnumData)), "%d", &devNum)

			if busParseErr == nil && devParseErr == nil && busNum > 0 && devNum > 0 {
				return busNum, devNum, nil
			}
			// Parse failed or values out of range — try parent directory
		}

		// Move up one directory
		parent := filepath.Dir(devicePath)
		if parent == devicePath || parent == "/" || parent == sysRoot {
			break
		}
		devicePath = parent
	}

	return 0, 0, fmt.Errorf("USB bus/dev numbers not found for card %d", cardNum)
}
