package audio

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SanitizeDeviceName sanitizes a device name for safe use in configuration and file paths.
//
// This implementation MUST match the bash version in lyrebird-mic-check.sh (lines 395-426)
// exactly, as config lookups depend on identical output.
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
//
// Reference: lyrebird-mic-check.sh sanitize_device_name()
func SanitizeDeviceName(name string) string {
	// Security: Reject suspicious patterns
	// Matches bash: if [[ "$name" =~ \.\. ]] || [[ "$name" =~ [/$] ]] || [[ "$name" =~ ^- ]]
	if strings.Contains(name, "..") ||
		strings.ContainsAny(name, "/$") ||
		strings.HasPrefix(name, "-") {
		return timestampFallback()
	}

	// Truncate to 64 characters
	// Matches bash: if [[ ${#name} -gt 64 ]]; then name="${name:0:64}"; fi
	if len(name) > 64 {
		name = name[:64]
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
