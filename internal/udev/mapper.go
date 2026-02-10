// SPDX-License-Identifier: MIT

package udev

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// usbPortPathRegex is pre-compiled at package level to avoid recompiling on every call.
// Pattern: bus-port or bus-port.subport.subport...
// Examples: "1-1", "1-1.4", "2-3.1.2"
var usbPortPathRegex = regexp.MustCompile(`^[0-9]+-[0-9]+(\.[0-9]+)*$`)

// USBPortInfo contains information about a USB device's physical port.
type USBPortInfo struct {
	PortPath string // Physical USB port (e.g., "1-1.4")
	Product  string // Product name (if available)
	Serial   string // Serial number (if available)
}

// GetUSBPhysicalPort detects the physical USB port for a device.
//
// This is CRITICAL for supporting multiple identical USB devices.
// The implementation matches the bash version which fixed bugs where
// Device 5 could incorrectly match a USB hub instead of the actual device.
//
// Method: Scan ALL /sys/bus/usb/devices/* directories and validate
// BOTH busnum AND devnum to find the correct device.
//
// Parameters:
//   - sysfsPath: Path to /sys/bus/usb/devices (or testdata equivalent)
//   - busNum: USB bus number (from lsusb or /sys/.../busnum)
//   - devNum: USB device number (from lsusb or /sys/.../devnum)
//
// Returns:
//   - portPath: Physical port path (e.g., "1-1.4")
//   - product: Product name (may be empty)
//   - serial: Serial number (may be empty)
//   - error: if device not found or parameters invalid
//
// Example:
//
//	port, product, serial, err := GetUSBPhysicalPort("/sys/bus/usb/devices", 1, 5)
//	// port = "1-1.4", product = "Yeti Stereo Microphone", serial = "REV8_12345"
//
// Reference: usb-audio-mapper.sh get_usb_physical_port() lines 221-342
func GetUSBPhysicalPort(sysfsPath string, busNum, devNum int) (portPath, product, serial string, err error) {
	// Validate inputs
	if busNum < 0 || devNum < 0 {
		return "", "", "", fmt.Errorf("invalid bus/dev number: bus=%d dev=%d", busNum, devNum)
	}

	// Verify sysfs path exists
	if _, err := os.Stat(sysfsPath); os.IsNotExist(err) {
		return "", "", "", fmt.Errorf("sysfs path not found: %s", sysfsPath)
	}

	// Method 1: Search through ALL USB devices in sysfs
	// This avoids incorrect path guessing (Device 5 matching hub bug)
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read sysfs directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Only check directories with USB port naming pattern: ^[0-9]+-[0-9]+(\.[0-9]+)*$
		// Examples: "1-1", "1-1.4", "2-3.1.2"
		basename := entry.Name()
		if !IsValidUSBPortPath(basename) {
			continue
		}

		devicePath := filepath.Join(sysfsPath, basename)

		// Read bus and device numbers
		deviceBusNum, deviceDevNum, err := readBusDevNum(devicePath)
		if err != nil {
			continue // Skip devices we can't read
		}

		// Check if THIS device matches our target bus/dev numbers
		if deviceBusNum == busNum && deviceDevNum == devNum {
			// Found it!
			portPath = basename

			// Read optional product name
			// #nosec G304 - Reading from /sys/bus/usb (kernel filesystem)
			if productBytes, err := os.ReadFile(filepath.Join(devicePath, "product")); err == nil {
				product = strings.TrimSpace(string(productBytes))
			}

			// Read optional serial number
			// #nosec G304 - Reading from /sys/bus/usb (kernel filesystem)
			if serialBytes, err := os.ReadFile(filepath.Join(devicePath, "serial")); err == nil {
				serial = strings.TrimSpace(string(serialBytes))
			}

			return portPath, product, serial, nil
		}
	}

	// Not found
	return "", "", "", fmt.Errorf("USB device not found: bus=%d dev=%d", busNum, devNum)
}

// IsValidUSBPortPath checks if a string matches USB port path pattern.
//
// Valid patterns: "1-1", "1-1.4", "2-3.1.2", etc.
// Pattern: ^[0-9]+-[0-9]+(\.[0-9]+)*$
//
// Reference: usb-audio-mapper.sh line 251
func IsValidUSBPortPath(path string) bool {
	return usbPortPathRegex.MatchString(path)
}

// SafeBase10 converts a string to base-10 integer, handling leading zeros.
//
// This prevents octal interpretation (e.g., "08" is invalid in octal but valid as decimal 8).
//
// Parameters:
//   - s: String to convert (may have leading zeros)
//
// Returns:
//   - int: Parsed value
//   - error: if string is not a valid positive integer
//
// Example:
//
//	SafeBase10("005") → 5, nil
//	SafeBase10("08")  → 8, nil
//	SafeBase10("abc") → 0, error
//
// Reference: usb-audio-mapper.sh safe_base10() function
func SafeBase10(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}

	// Remove leading zeros but keep at least one digit
	s = strings.TrimLeft(s, "0")
	if s == "" {
		s = "0"
	}

	// Parse as base-10 integer
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer: %w", err)
	}

	// Reject negative numbers
	if val < 0 {
		return 0, fmt.Errorf("negative numbers not allowed: %d", val)
	}

	return val, nil
}

// readBusDevNum reads busnum and devnum from a sysfs device directory.
//
// Returns:
//   - busNum: USB bus number
//   - devNum: USB device number
//   - error: if files don't exist or can't be parsed
func readBusDevNum(devicePath string) (busNum, devNum int, err error) {
	// Read busnum file
	// #nosec G304 - Reading from /sys/bus/usb (kernel filesystem)
	busBytes, err := os.ReadFile(filepath.Join(devicePath, "busnum"))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read busnum: %w", err)
	}

	busNum, err = SafeBase10(string(busBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse busnum: %w", err)
	}

	// Read devnum file
	// #nosec G304 - Reading from /sys/bus/usb (kernel filesystem)
	devBytes, err := os.ReadFile(filepath.Join(devicePath, "devnum"))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read devnum: %w", err)
	}

	devNum, err = SafeBase10(string(devBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse devnum: %w", err)
	}

	return busNum, devNum, nil
}
