// SPDX-License-Identifier: MIT

package audio

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Capabilities represents the audio capabilities of a USB device.
//
// This is detected by parsing /proc/asound/cardN/stream0 without
// opening the device, matching the non-invasive approach of
// lyrebird-mic-check.sh.
//
// Reference: lyrebird-mic-check.sh lines 617-750
type Capabilities struct {
	CardNumber  int      // ALSA card number
	DeviceName  string   // Device name
	Formats     []string // Supported formats (S16_LE, S24_LE, S32_LE, etc.)
	SampleRates []int    // Supported sample rates in Hz
	Channels    []int    // Supported channel counts
	BitDepths   []int    // Derived bit depths (16, 24, 32)
	MinRate     int      // Minimum sample rate
	MaxRate     int      // Maximum sample rate
	MinChannels int      // Minimum channels
	MaxChannels int      // Maximum channels
	IsBusy      bool     // True if device is currently in use
	BusyBy      string   // Process/application using the device (if known)
}

// Common ALSA formats and their bit depths.
var formatBitDepths = map[string]int{
	"S8":         8,
	"U8":         8,
	"S16_LE":     16,
	"S16_BE":     16,
	"U16_LE":     16,
	"U16_BE":     16,
	"S24_LE":     24,
	"S24_BE":     24,
	"U24_LE":     24,
	"U24_BE":     24,
	"S24_3LE":    24,
	"S24_3BE":    24,
	"S32_LE":     32,
	"S32_BE":     32,
	"U32_LE":     32,
	"U32_BE":     32,
	"FLOAT_LE":   32,
	"FLOAT_BE":   32,
	"FLOAT64_LE": 64,
	"FLOAT64_BE": 64,
}

// DetectCapabilities reads device capabilities from /proc/asound/cardN/stream0.
//
// This is a non-invasive detection that doesn't open the device or interrupt
// active streams, matching the behavior of lyrebird-mic-check.sh.
//
// Parameters:
//   - asoundPath: Path to /proc/asound directory
//   - cardNumber: ALSA card number to query
//
// Returns:
//   - Capabilities struct with all detected info
//   - Error if card doesn't exist or can't be read
//
// Reference: lyrebird-mic-check.sh get_device_capabilities() lines 617-750
func DetectCapabilities(asoundPath string, cardNumber int) (*Capabilities, error) {
	cardDir := filepath.Join(asoundPath, fmt.Sprintf("card%d", cardNumber))

	// Verify card exists
	if _, err := os.Stat(cardDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("card %d not found", cardNumber)
	}

	caps := &Capabilities{
		CardNumber: cardNumber,
	}

	// Read device name
	idPath := filepath.Join(cardDir, "id")
	// #nosec G304 -- reading from /proc/asound, controlled path
	if data, err := os.ReadFile(idPath); err == nil {
		caps.DeviceName = strings.TrimSpace(string(data))
	}

	// Parse stream0 for capture capabilities
	stream0Path := filepath.Join(cardDir, "stream0")
	if err := parseStreamFile(stream0Path, caps); err != nil {
		// Try pcm0c (capture device) as fallback
		pcmPath := filepath.Join(cardDir, "pcm0c", "info")
		if err2 := parsePCMInfo(pcmPath, caps); err2 != nil {
			// Return with minimal info rather than failing
			caps.Formats = []string{"S16_LE"}
			caps.SampleRates = []int{48000}
			caps.Channels = []int{2}
			caps.BitDepths = []int{16}
			caps.MinRate = 48000
			caps.MaxRate = 48000
			caps.MinChannels = 2
			caps.MaxChannels = 2
		}
	}

	// Check if device is busy
	caps.IsBusy, caps.BusyBy = checkDeviceBusy(cardDir, cardNumber)

	// Derive bit depths from formats
	if len(caps.BitDepths) == 0 {
		caps.BitDepths = deriveBitDepths(caps.Formats)
	}

	// Set min/max if not already set
	if len(caps.SampleRates) > 0 && caps.MinRate == 0 {
		caps.MinRate = caps.SampleRates[0]
		caps.MaxRate = caps.SampleRates[len(caps.SampleRates)-1]
	}
	if len(caps.Channels) > 0 && caps.MinChannels == 0 {
		caps.MinChannels = caps.Channels[0]
		caps.MaxChannels = caps.Channels[len(caps.Channels)-1]
	}

	return caps, nil
}

// parseStreamFile parses /proc/asound/cardN/stream0 for capabilities.
//
// Example stream0 content:
//
//	USB Audio
//	  Status: Stop
//	  Interface 1
//	    Altset 1
//	    Format: S16_LE
//	    Channels: 2
//	    Endpoint: 1 IN (ASYNC)
//	    Rates: 44100, 48000
//
// Reference: lyrebird-mic-check.sh parse_stream_file() lines 680-730
func parseStreamFile(path string, caps *Capabilities) error {
	// #nosec G304 -- reading from /proc/asound, controlled path
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	inCaptureSection := false

	var formats []string
	var rates []int
	var channels []int

	// Regex patterns matching bash implementation
	formatRe := regexp.MustCompile(`Format:\s+(\S+)`)
	channelsRe := regexp.MustCompile(`Channels:\s+(\d+)`)
	ratesRe := regexp.MustCompile(`Rates:\s+(.+)`)
	rateRangeRe := regexp.MustCompile(`(\d+)\s*-\s*(\d+)`)

	for scanner.Scan() {
		line := scanner.Text()

		// Look for capture endpoint (IN direction)
		if strings.Contains(line, "Endpoint:") && strings.Contains(line, "IN") {
			inCaptureSection = true
			continue
		}

		// Look for playback endpoint (OUT direction) to exit capture section
		if strings.Contains(line, "Endpoint:") && strings.Contains(line, "OUT") {
			inCaptureSection = false
			continue
		}

		// Also detect capture by interface description
		if strings.Contains(line, "Interface") || strings.Contains(line, "Altset") {
			inCaptureSection = true
		}

		// Parse format
		if match := formatRe.FindStringSubmatch(line); match != nil {
			format := match[1]
			if !contains(formats, format) {
				formats = append(formats, format)
			}
		}

		// Parse channels
		if match := channelsRe.FindStringSubmatch(line); match != nil {
			if ch, err := strconv.Atoi(match[1]); err == nil {
				if !containsInt(channels, ch) {
					channels = append(channels, ch)
				}
			}
		}

		// Parse rates
		if match := ratesRe.FindStringSubmatch(line); match != nil {
			rateStr := match[1]

			// Check for range format (e.g., "8000 - 96000")
			if rangeMatch := rateRangeRe.FindStringSubmatch(rateStr); rangeMatch != nil {
				minRate, _ := strconv.Atoi(rangeMatch[1])
				maxRate, _ := strconv.Atoi(rangeMatch[2])
				caps.MinRate = minRate
				caps.MaxRate = maxRate
				rates = generateRatesInRange(minRate, maxRate)
			} else {
				// Parse comma-separated rates
				for _, r := range strings.Split(rateStr, ",") {
					r = strings.TrimSpace(r)
					if rate, err := strconv.Atoi(r); err == nil {
						if !containsInt(rates, rate) {
							rates = append(rates, rate)
						}
					}
				}
			}
		}
	}

	// Use parsed values or defaults
	if len(formats) > 0 {
		caps.Formats = formats
	}
	if len(rates) > 0 {
		sort.Ints(rates)
		caps.SampleRates = rates
	}
	if len(channels) > 0 {
		sort.Ints(channels)
		caps.Channels = channels
	}

	// Mark as capture section found if we got IN endpoint
	if !inCaptureSection && len(formats) == 0 {
		return fmt.Errorf("no capture capabilities found")
	}

	return scanner.Err()
}

// parsePCMInfo parses /proc/asound/cardN/pcm0c/info as fallback.
func parsePCMInfo(path string, caps *Capabilities) error {
	// #nosec G304 -- reading from /proc/asound, controlled path
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(data)

	// Extract basic info - this is a simpler format
	if strings.Contains(content, "stream: CAPTURE") {
		if len(caps.Formats) == 0 {
			caps.Formats = []string{"S16_LE", "S24_LE"}
		}
		if len(caps.SampleRates) == 0 {
			caps.SampleRates = []int{44100, 48000}
		}
		if len(caps.Channels) == 0 {
			caps.Channels = []int{1, 2}
		}
	}

	return nil
}

// checkDeviceBusy checks if device is currently in use without opening it.
//
// Checks:
//   - /proc/asound/cardN/pcm0c/sub0/status - "RUNNING" indicates active
//   - /proc/asound/cardN/pcm0c/sub0/hw_params - Non-"closed" indicates in use
//
// Reference: lyrebird-mic-check.sh is_device_busy() lines 752-800
func checkDeviceBusy(cardDir string, cardNumber int) (busy bool, busyBy string) {
	// Check status file
	statusPath := filepath.Join(cardDir, "pcm0c", "sub0", "status")
	// #nosec G304 -- reading from /proc/asound, controlled path
	if data, err := os.ReadFile(statusPath); err == nil {
		content := strings.TrimSpace(string(data))
		if strings.Contains(content, "RUNNING") || strings.Contains(content, "PREPARED") {
			busy = true
		}
		if strings.Contains(content, "owner_pid") {
			for _, line := range strings.Split(content, "\n") {
				if strings.Contains(line, "owner_pid") {
					parts := strings.Split(line, ":")
					if len(parts) >= 2 {
						busyBy = strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}

	// Check hw_params file
	hwParamsPath := filepath.Join(cardDir, "pcm0c", "sub0", "hw_params")
	// #nosec G304 -- reading from /proc/asound, controlled path
	if data, err := os.ReadFile(hwParamsPath); err == nil {
		content := strings.TrimSpace(string(data))
		if content != "closed" && content != "" {
			busy = true
		}
	}

	return busy, busyBy
}

// deriveBitDepths extracts bit depths from format list.
func deriveBitDepths(formats []string) []int {
	seen := make(map[int]bool)
	var depths []int

	for _, f := range formats {
		if depth, ok := formatBitDepths[f]; ok {
			if !seen[depth] {
				seen[depth] = true
				depths = append(depths, depth)
			}
		}
	}

	sort.Ints(depths)
	return depths
}

// generateRatesInRange returns common sample rates within a given range.
func generateRatesInRange(minRate, maxRate int) []int {
	commonRates := []int{8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000, 352800, 384000}
	var result []int

	for _, rate := range commonRates {
		if rate >= minRate && rate <= maxRate {
			result = append(result, rate)
		}
	}

	return result
}

// CapabilitiesSummary returns a human-readable summary of capabilities.
func (c *Capabilities) CapabilitiesSummary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Card %d: %s\n", c.CardNumber, c.DeviceName))
	sb.WriteString(fmt.Sprintf("  Formats: %s\n", strings.Join(c.Formats, ", ")))
	sb.WriteString(fmt.Sprintf("  Sample Rates: %s\n", formatIntSlice(c.SampleRates)))
	sb.WriteString(fmt.Sprintf("  Channels: %s\n", formatIntSlice(c.Channels)))
	sb.WriteString(fmt.Sprintf("  Bit Depths: %s\n", formatIntSlice(c.BitDepths)))

	if c.MinRate > 0 && c.MaxRate > 0 {
		sb.WriteString(fmt.Sprintf("  Rate Range: %d - %d Hz\n", c.MinRate, c.MaxRate))
	}

	if c.IsBusy {
		status := "In Use"
		if c.BusyBy != "" {
			status = fmt.Sprintf("In Use (by PID %s)", c.BusyBy)
		}
		sb.WriteString(fmt.Sprintf("  Status: %s\n", status))
	} else {
		sb.WriteString("  Status: Available\n")
	}

	return sb.String()
}

// formatIntSlice formats an int slice as comma-separated string.
func formatIntSlice(slice []int) string {
	if len(slice) == 0 {
		return "(none)"
	}

	strs := make([]string, len(slice))
	for i, v := range slice {
		strs[i] = strconv.Itoa(v)
	}
	return strings.Join(strs, ", ")
}

// SupportsRate checks if the device supports a specific sample rate.
func (c *Capabilities) SupportsRate(rate int) bool {
	if containsInt(c.SampleRates, rate) {
		return true
	}
	if c.MinRate > 0 && c.MaxRate > 0 {
		return rate >= c.MinRate && rate <= c.MaxRate
	}
	return false
}

// SupportsChannels checks if the device supports a specific channel count.
func (c *Capabilities) SupportsChannels(channels int) bool {
	if containsInt(c.Channels, channels) {
		return true
	}
	if c.MinChannels > 0 && c.MaxChannels > 0 {
		return channels >= c.MinChannels && channels <= c.MaxChannels
	}
	return false
}

// SupportsFormat checks if the device supports a specific format.
func (c *Capabilities) SupportsFormat(format string) bool {
	return contains(c.Formats, format)
}
