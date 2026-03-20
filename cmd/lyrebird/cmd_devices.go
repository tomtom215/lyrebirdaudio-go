// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
)

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
	// Parse optional quality tier flag
	qualityStr := "normal"
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--quality="):
			qualityStr = strings.TrimPrefix(args[i], "--quality=")
		case args[i] == "--quality" && i+1 < len(args):
			qualityStr = args[i+1]
			i++
		}
	}
	tier, err := audio.ParseQualityTier(qualityStr)
	if err != nil {
		return fmt.Errorf("invalid quality tier: %w", err)
	}

	// Scan for USB audio devices
	devices, err := audio.DetectDevices(asoundPath)
	if err != nil {
		return fmt.Errorf("failed to scan devices: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No USB audio devices detected")
		return nil
	}

	fmt.Printf("Detected %d device(s) with actual capabilities and recommended settings:\n\n", len(devices))

	for _, dev := range devices {
		fmt.Printf("Device: %s\n", dev.Name)
		fmt.Printf("  ALSA ID:  hw:%d,0\n", dev.CardNumber)
		fmt.Printf("  USB ID:   %s\n", dev.USBID)

		// Detect actual device capabilities from /proc/asound/cardN/stream0
		caps, capsErr := audio.DetectCapabilities(asoundPath, dev.CardNumber)
		if capsErr != nil {
			// Fallback to defaults when stream0 is unavailable (e.g., test env)
			fmt.Printf("  Capabilities: (unavailable: %v)\n", capsErr)
			fmt.Printf("  Recommended settings (defaults):\n")
			fmt.Printf("    Sample rate: 48000 Hz\n")
			fmt.Printf("    Channels:    2 (stereo)\n")
			fmt.Printf("    Codec:       opus\n")
			fmt.Printf("    Bitrate:     128k\n")
		} else {
			rec := audio.RecommendSettings(caps, tier)
			fmt.Printf("  Capabilities:\n")
			fmt.Printf("    Formats:      %s\n", strings.Join(caps.Formats, ", "))
			fmt.Printf("    Sample rates: %s Hz\n", formatIntSliceForDetect(caps.SampleRates))
			fmt.Printf("    Channels:     %s\n", formatIntSliceForDetect(caps.Channels))
			if caps.IsBusy {
				busyMsg := "in use"
				if caps.BusyBy != "" {
					busyMsg = fmt.Sprintf("in use (PID %s)", caps.BusyBy)
				}
				fmt.Printf("    Status:       %s\n", busyMsg)
			} else {
				fmt.Printf("    Status:       available\n")
			}
			fmt.Printf("  Recommended settings (%s quality):\n", tier)
			fmt.Printf("    Sample rate: %d Hz\n", rec.SampleRate)
			channels := "stereo"
			if rec.Channels == 1 {
				channels = "mono"
			}
			fmt.Printf("    Channels:    %d (%s)\n", rec.Channels, channels)
			fmt.Printf("    Codec:       %s\n", rec.Codec)
			fmt.Printf("    Bitrate:     %s\n", rec.Bitrate)
			fmt.Printf("    Format:      %s\n", rec.Format)
		}
		fmt.Println()
	}

	fmt.Println("Note: Configure per-device settings in /etc/lyrebird/config.yaml")
	fmt.Println("      Use --quality=low|normal|high to change recommendation tier.")
	return nil
}

// formatIntSliceForDetect formats an int slice as comma-separated string.
func formatIntSliceForDetect(vals []int) string {
	strs := make([]string, len(vals))
	for i, v := range vals {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(strs, ", ")
}
