// SPDX-License-Identifier: MIT

package audio

import (
	"fmt"
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

// RecommendedSettings contains optimal settings for a device based on capabilities.
type RecommendedSettings struct {
	SampleRate int    // Recommended sample rate in Hz
	Channels   int    // Recommended channel count
	Codec      string // Recommended codec (opus, aac)
	Bitrate    string // Recommended bitrate (e.g., "128k")
	Format     string // ALSA format to use (e.g., S16_LE)
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
		settings.Bitrate = halveBitrate(settings.Bitrate)
	}

	return settings
}

// GetQualityPresets returns all available quality presets.
func GetQualityPresets() map[QualityTier]RecommendedSettings {
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
