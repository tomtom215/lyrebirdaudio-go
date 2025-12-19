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

// QualityTier represents audio quality presets.
type QualityTier string

const (
	// QualityLow optimizes for minimal bandwidth (voice/telephony).
	QualityLow QualityTier = "low"
	// QualityNormal provides balanced quality/bandwidth (default).
	QualityNormal QualityTier = "normal"
	// QualityHigh prioritizes audio quality (music/broadcast).
	QualityHigh QualityTier = "high"
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

// RecommendedSettings contains optimal settings for a device based on capabilities.
type RecommendedSettings struct {
	SampleRate int    // Recommended sample rate in Hz
	Channels   int    // Recommended channel count
	Codec      string // Recommended codec (opus, aac)
	Bitrate    string // Recommended bitrate (e.g., "128k")
	Format     string // ALSA format to use (e.g., S16_LE)
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

// Quality tier presets matching bash implementation.
var qualityPresets = map[QualityTier]RecommendedSettings{
	QualityLow: {
		SampleRate: 16000,
		Channels:   1,
		Codec:      "opus",
		Bitrate:    "24k",
		Format:     "S16_LE",
	},
	QualityNormal: {
		SampleRate: 48000,
		Channels:   2,
		Codec:      "opus",
		Bitrate:    "128k",
		Format:     "S16_LE",
	},
	QualityHigh: {
		SampleRate: 48000,
		Channels:   2,
		Codec:      "opus",
		Bitrate:    "256k",
		Format:     "S24_LE",
	},
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
	defer file.Close()

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
			// Check next lines for IN endpoint
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
				// Generate common rates within range
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
		// Default capture capabilities
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
		// Try to extract owner info
		if strings.Contains(content, "owner_pid") {
			// Parse owner_pid line
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

// RecommendSettings returns optimal settings based on device capabilities and quality tier.
//
// The algorithm:
//  1. Start with quality tier preset
//  2. Adjust if device doesn't support preset values
//  3. Prefer opus for low latency, aac for compatibility
//
// Reference: lyrebird-mic-check.sh recommend_settings() lines 802-870
func RecommendSettings(caps *Capabilities, tier QualityTier) *RecommendedSettings {
	// Get base preset
	preset, ok := qualityPresets[tier]
	if !ok {
		preset = qualityPresets[QualityNormal]
	}

	settings := &RecommendedSettings{
		SampleRate: preset.SampleRate,
		Channels:   preset.Channels,
		Codec:      preset.Codec,
		Bitrate:    preset.Bitrate,
		Format:     preset.Format,
	}

	// Adjust sample rate if not supported
	if len(caps.SampleRates) > 0 {
		if !containsInt(caps.SampleRates, settings.SampleRate) {
			// Find closest supported rate
			settings.SampleRate = findClosestRate(caps.SampleRates, settings.SampleRate)
		}
	}

	// Adjust channels if not supported
	if len(caps.Channels) > 0 {
		if !containsInt(caps.Channels, settings.Channels) {
			// Use max available channels up to desired
			for i := len(caps.Channels) - 1; i >= 0; i-- {
				if caps.Channels[i] <= settings.Channels {
					settings.Channels = caps.Channels[i]
					break
				}
			}
			// If all channels are higher, use minimum
			if settings.Channels > caps.Channels[len(caps.Channels)-1] {
				settings.Channels = caps.Channels[0]
			}
		}
	}

	// Adjust format if not supported
	if len(caps.Formats) > 0 {
		if !contains(caps.Formats, settings.Format) {
			// Prefer S16_LE, then S24_LE, then first available
			if contains(caps.Formats, "S16_LE") {
				settings.Format = "S16_LE"
			} else if contains(caps.Formats, "S24_LE") {
				settings.Format = "S24_LE"
			} else {
				settings.Format = caps.Formats[0]
			}
		}
	}

	// Adjust bitrate based on channels
	if settings.Channels == 1 {
		// Halve bitrate for mono
		settings.Bitrate = halveBitrate(settings.Bitrate)
	}

	return settings
}

// GetQualityPresets returns all available quality presets.
func GetQualityPresets() map[QualityTier]RecommendedSettings {
	// Return a copy to prevent modification
	result := make(map[QualityTier]RecommendedSettings)
	for k, v := range qualityPresets {
		result[k] = v
	}
	return result
}

// ParseQualityTier converts a string to QualityTier.
func ParseQualityTier(s string) (QualityTier, error) {
	switch strings.ToLower(s) {
	case "low", "l":
		return QualityLow, nil
	case "normal", "n", "medium", "m", "":
		return QualityNormal, nil
	case "high", "h":
		return QualityHigh, nil
	default:
		return "", fmt.Errorf("invalid quality tier %q: must be low, normal, or high", s)
	}
}

// findClosestRate finds the closest sample rate to the target.
func findClosestRate(rates []int, target int) int {
	if len(rates) == 0 {
		return target
	}

	closest := rates[0]
	minDiff := abs(rates[0] - target)

	for _, rate := range rates[1:] {
		diff := abs(rate - target)
		if diff < minDiff {
			minDiff = diff
			closest = rate
		}
	}

	return closest
}

// halveBitrate halves a bitrate string (e.g., "128k" -> "64k").
func halveBitrate(bitrate string) string {
	// Parse number and suffix
	numStr := strings.TrimRight(bitrate, "kKmM")
	suffix := bitrate[len(numStr):]

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return bitrate
	}

	return fmt.Sprintf("%d%s", num/2, suffix)
}

// abs returns absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// contains checks if a string slice contains a value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// containsInt checks if an int slice contains a value.
func containsInt(slice []int, val int) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
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
	// Check explicit rate list
	if containsInt(c.SampleRates, rate) {
		return true
	}

	// Check range
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
