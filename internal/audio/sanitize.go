// SPDX-License-Identifier: MIT

package audio

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	// MaxDeviceNameLength is the maximum length for sanitized device names.
	// This matches the bash implementation's 64 character limit.
	MaxDeviceNameLength = 64

	// MaxRawInputLength is the maximum raw input length we'll process.
	// Inputs longer than this are immediately rejected to prevent
	// memory exhaustion from malicious inputs.
	MaxRawInputLength = 1024
)

// SanitizeDeviceName sanitizes a device name for safe use in configuration and file paths.
//
// This implementation MUST match the bash version in lyrebird-mic-check.sh (lines 395-426)
// exactly, as config lookups depend on identical output.
//
// Input validation:
//   - Empty input returns timestamped fallback
//   - Input longer than 1024 bytes returns timestamped fallback (security measure)
//   - Control characters (0x00-0x1F) trigger timestamped fallback
//
// Sanitization rules:
//  1. Reject suspicious patterns (path traversal, command injection): return timestamped fallback
//  2. Truncate to 64 characters maximum
//  3. Replace non-alphanumeric characters with underscore
//  4. Collapse consecutive underscores
//  5. Strip leading and trailing underscores
//  6. Prefix "dev_" if starts with digit
//  7. Return timestamped fallback if empty after sanitization
//
// Examples:
//
//	"Blue Yeti" → "Blue_Yeti"
//	"USB-Audio-Device" → "USB_Audio_Device"
//	"5GHz" → "dev_5GHz"
//	"../etc/passwd" → "unknown_device_1234567890"
//	"" → "unknown_device_1234567890"
//
// Reference: lyrebird-mic-check.sh sanitize_device_name()
func SanitizeDeviceName(name string) string {
	// Early validation: reject empty input
	if name == "" {
		return timestampFallback()
	}

	// Security: reject excessively long input to prevent memory exhaustion
	if len(name) > MaxRawInputLength {
		return timestampFallback()
	}

	// Security: reject input containing control characters (0x00-0x1F except tab/newline)
	// These could cause issues in various contexts (file systems, terminals, configs)
	if containsControlChars(name) {
		return timestampFallback()
	}

	// Security: Reject suspicious patterns
	// Matches bash: if [[ "$name" =~ \.\. ]] || [[ "$name" =~ [/$] ]] || [[ "$name" =~ ^- ]]
	if strings.Contains(name, "..") ||
		strings.ContainsAny(name, "/$") ||
		strings.HasPrefix(name, "-") {
		return timestampFallback()
	}

	// Truncate to MaxDeviceNameLength characters
	// Matches bash: if [[ ${#name} -gt 64 ]]; then name="${name:0:64}"; fi
	if len(name) > MaxDeviceNameLength {
		name = name[:MaxDeviceNameLength]
	}

	// Replace non-alphanumeric with underscore
	// Matches bash: sed 's/[^a-zA-Z0-9]/_/g; s/__*/_/g; s/^_//; s/_$//'
	sanitized := replaceNonAlphanumeric(name)

	// Collapse consecutive underscores
	sanitized = collapseUnderscores(sanitized)

	// Strip leading and trailing underscores
	sanitized = strings.Trim(sanitized, "_")

	// Prefix "dev_" if starts with digit
	// Matches bash: if [[ "$sanitized" =~ ^[0-9] ]]; then sanitized="dev_${sanitized}"; fi
	if len(sanitized) > 0 && isDigit(sanitized[0]) {
		sanitized = "dev_" + sanitized
	}

	// Fallback if empty
	// Matches bash: if [[ -z "$sanitized" ]]; then sanitized="unknown_device_$(date +%s)"; fi
	if sanitized == "" {
		return timestampFallback()
	}

	return sanitized
}

// replaceNonAlphanumeric replaces any character that is not a-z, A-Z, or 0-9 with underscore.
// Matches bash: sed 's/[^a-zA-Z0-9]/_/g'
func replaceNonAlphanumeric(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for i := 0; i < len(s); i++ {
		c := s[i]
		if isAlphanumeric(c) {
			result.WriteByte(c)
		} else {
			result.WriteByte('_')
		}
	}

	return result.String()
}

// collapseUnderscores replaces consecutive underscores with a single underscore.
// Matches bash: sed 's/__*/_/g'
func collapseUnderscores(s string) string {
	// Use regex to match one or more underscores and replace with single underscore
	re := regexp.MustCompile(`_+`)
	return re.ReplaceAllString(s, "_")
}

// isAlphanumeric checks if a byte is a-z, A-Z, or 0-9.
func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// isDigit checks if a byte is 0-9.
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// timestampFallback returns "unknown_device_" followed by Unix timestamp.
// Matches bash: printf 'unknown_device_%s\n' "$(date +%s)"
func timestampFallback() string {
	return fmt.Sprintf("unknown_device_%d", time.Now().Unix())
}

// containsControlChars checks if a string contains control characters (0x00-0x1F)
// except for common whitespace (tab 0x09, newline 0x0A, carriage return 0x0D).
// Control characters can cause issues in file paths, terminals, and config files.
func containsControlChars(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		// Check for control characters (0x00-0x1F) except tab, newline, carriage return
		if c < 0x20 && c != 0x09 && c != 0x0A && c != 0x0D {
			return true
		}
		// Also check for DEL (0x7F)
		if c == 0x7F {
			return true
		}
	}
	return false
}
