package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Device represents a USB audio device detected from ALSA.
//
// Fields match the bash implementation output from get_device_info():
//   - card_number: ALSA card number (0-31)
//   - device_name: Short name from /proc/asound/cardN/id
//   - usb_id: USB vendor:product ID (e.g., "0d8c:0014")
//   - device_id_path: Persistent ID from /dev/snd/by-id/ (optional)
//
// Reference: lyrebird-mic-check.sh get_device_info() lines 551-615
type Device struct {
	CardNumber int    // ALSA card number (0-31)
	Name       string // Device name from /proc/asound/cardN/id
	USBID      string // USB vendor:product ID (e.g., "0d8c:0014")
	VendorID   string // USB vendor ID (e.g., "0d8c")
	ProductID  string // USB product ID (e.g., "0014")
	DeviceID   string // Device ID from /dev/snd/by-id/ (optional)
}

// FriendlyName returns the sanitized device name for config lookup.
// This is the "friendly name" format used for configuration.
//
// Example: "USB Audio Device" → "USB_Audio_Device"
func (d *Device) FriendlyName() string {
	return SanitizeDeviceName(d.Name)
}

// FullDeviceID returns the sanitized full device ID for config lookup.
// This is the "full ID" format: uppercase, USB_ prefix.
//
// Format: "usb-Manufacturer_Model_Serial-xxx" → "USB_MANUFACTURER_MODEL_SERIAL_XXX"
//
// Reference: lyrebird-mic-check.sh lines 1956-1962
func (d *Device) FullDeviceID() string {
	if d.DeviceID == "" {
		return ""
	}

	// Remove "usb-" prefix if present
	id := strings.TrimPrefix(d.DeviceID, "usb-")

	// Replace non-alphanumeric with underscore
	sanitized := replaceNonAlphanumeric(id)

	// Collapse consecutive underscores
	sanitized = collapseUnderscores(sanitized)

	// Strip leading/trailing underscores
	sanitized = strings.Trim(sanitized, "_")

	// Convert to uppercase and add USB_ prefix
	return "USB_" + strings.ToUpper(sanitized)
}

// DetectDevices scans /proc/asound for USB audio devices.
//
// Returns a list of USB audio devices sorted by card number.
// Non-USB devices (missing /proc/asound/cardN/usbid) are skipped.
//
// Parameters:
//   - asoundPath: Path to /proc/asound directory (for testing can be custom path)
//
// Returns:
//   - Slice of detected USB devices
//   - Error if asoundPath doesn't exist or can't be read
//
// Reference: lyrebird-mic-check.sh lines 1902-1927
func DetectDevices(asoundPath string) ([]*Device, error) {
	// Verify directory exists
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("asound directory not found: %s", asoundPath)
	}

	// Find all card directories
	pattern := filepath.Join(asoundPath, "card[0-9]*")
	cardDirs, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob card directories: %w", err)
	}

	var devices []*Device

	for _, cardDir := range cardDirs {
		// Extract card number from directory name
		baseName := filepath.Base(cardDir)
		cardNumStr := strings.TrimPrefix(baseName, "card")
		cardNum, err := strconv.Atoi(cardNumStr)
		if err != nil {
			continue // Skip invalid card directory
		}

		// Check if this is a USB device (has usbid file)
		usbIDPath := filepath.Join(cardDir, "usbid")
		if _, err := os.Stat(usbIDPath); os.IsNotExist(err) {
			continue // Skip non-USB device
		}

		// Get device information
		dev, err := GetDeviceInfo(asoundPath, cardNum)
		if err != nil {
			continue // Skip device if we can't read info
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

// GetDeviceInfo reads device information for a specific ALSA card.
//
// Reads from:
//   - /proc/asound/cardN/id: Short device name
//   - /proc/asound/cardN/usbid: USB vendor:product ID
//   - /dev/snd/by-id/*: Persistent device ID (optional)
//
// Parameters:
//   - asoundPath: Path to /proc/asound directory
//   - cardNumber: ALSA card number (0-31)
//
// Returns:
//   - Device information
//   - Error if card doesn't exist or is not a USB device
//
// Reference: lyrebird-mic-check.sh get_device_info() lines 551-615
func GetDeviceInfo(asoundPath string, cardNumber int) (*Device, error) {
	cardDir := filepath.Join(asoundPath, fmt.Sprintf("card%d", cardNumber))

	// Verify card directory exists
	if _, err := os.Stat(cardDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("card %d not found", cardNumber)
	}

	// Verify this is a USB device
	usbIDPath := filepath.Join(cardDir, "usbid")
	if _, err := os.Stat(usbIDPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("card %d is not a USB device", cardNumber)
	}

	// Read USB ID
	// #nosec G304 - Reading from /proc/asound (kernel filesystem)
	usbIDBytes, err := os.ReadFile(usbIDPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read usbid: %w", err)
	}
	usbID := strings.TrimSpace(string(usbIDBytes))

	// Parse USB ID into vendor and product
	vendorID, productID, err := ParseUSBID(usbID)
	if err != nil {
		return nil, fmt.Errorf("invalid USB ID format: %w", err)
	}

	// Read device name
	idPath := filepath.Join(cardDir, "id")
	// #nosec G304 - Reading from /proc/asound (kernel filesystem)
	nameBytes, err := os.ReadFile(idPath)
	name := "unknown"
	if err == nil {
		name = strings.TrimSpace(string(nameBytes))
	}

	// Fallback to card number if name is empty
	if name == "" {
		name = fmt.Sprintf("card%d", cardNumber)
	}

	// Try to find device ID from /dev/snd/by-id/
	// Note: This may not be available in all environments
	deviceIDPath := findDeviceIDPath(cardNumber)

	return &Device{
		CardNumber: cardNumber,
		Name:       name,
		USBID:      usbID,
		VendorID:   vendorID,
		ProductID:  productID,
		DeviceID:   deviceIDPath,
	}, nil
}

// ParseUSBID parses a USB ID string into vendor and product IDs.
//
// Format: "VVVV:PPPP" where V=vendor hex, P=product hex
//
// Example: "0d8c:0014" → vendor="0d8c", product="0014"
//
// Returns:
//   - vendorID: 4-character hex string
//   - productID: 4-character hex string
//   - error: if format is invalid
func ParseUSBID(usbID string) (vendorID, productID string, err error) {
	parts := strings.Split(usbID, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid USB ID format: expected VVVV:PPPP, got %q", usbID)
	}

	vendorID = strings.TrimSpace(parts[0])
	productID = strings.TrimSpace(parts[1])

	// Validate format (4 hex digits each)
	if len(vendorID) != 4 || len(productID) != 4 {
		return "", "", fmt.Errorf("invalid USB ID format: expected 4-digit hex, got %q", usbID)
	}

	return vendorID, productID, nil
}

// findDeviceIDPath searches /dev/snd/by-id/ for persistent device ID.
//
// Returns the basename of the symlink that points to controlC{cardNumber},
// or empty string if not found.
//
// Reference: lyrebird-mic-check.sh lines 593-605
func findDeviceIDPath(cardNumber int) string {
	byIDDir := "/dev/snd/by-id"
	controlTarget := fmt.Sprintf("controlC%d", cardNumber)

	entries, err := os.ReadDir(byIDDir)
	if err != nil {
		return "" // Directory doesn't exist or can't be read
	}

	for _, entry := range entries {
		if !entry.Type().IsRegular() && entry.Type()&os.ModeSymlink == 0 {
			continue // Not a symlink
		}

		linkPath := filepath.Join(byIDDir, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}

		// Resolve to absolute path
		absTarget, err := filepath.Abs(filepath.Join(byIDDir, target))
		if err != nil {
			continue
		}

		// Check if this symlink points to our card's control device
		if strings.HasSuffix(absTarget, controlTarget) {
			return entry.Name()
		}
	}

	return "" // Not found
}
