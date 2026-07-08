// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

// runTest tests configuration without modifying system.
//
// Tests:
//  1. Config file syntax and validation
//  2. Device availability
//  3. FFmpeg command generation
//  4. MediaMTX connectivity
//  5. RTSP URL accessibility
func runTest(args []string) error {
	configPath := defaultConfigPath
	verbose := false

	// Parse flags
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--config" && i+1 < len(args):
			configPath = args[i+1]
			i++
		case args[i] == "-v" || args[i] == "--verbose":
			verbose = true
		}
	}

	fmt.Printf("Testing configuration: %s\n\n", configPath)

	// `test` is an advisory dry-run: an unloadable/invalid config is a hard
	// failure (it returns an error below), but every other check is a soft
	// warning so the command stays usable before devices and servers are
	// provisioned. allPassed only drives the human-facing summary. The
	// scriptable capability gate is `check-system`; the doctor is `diagnose`.
	allPassed := true

	// Test 1: Config syntax and validation
	fmt.Print("[1/5] Config syntax: ")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("FAILED\n      %v\n", err)
		return fmt.Errorf("config test failed: %w", err)
	}
	fmt.Println("OK")
	if verbose {
		fmt.Printf("      Default: %dHz, %dch, %s, %s\n",
			cfg.Default.SampleRate, cfg.Default.Channels, cfg.Default.Codec, cfg.Default.Bitrate)
		if len(cfg.Devices) > 0 {
			fmt.Printf("      Devices: %d configured\n", len(cfg.Devices))
		}
	}

	// Test 2: Device availability
	fmt.Print("[2/5] Device availability: ")
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil || len(devices) == 0 {
		fmt.Println("WARNING - No USB audio devices found")
		allPassed = false
		if verbose {
			fmt.Println("      Connect a USB audio device to stream")
		}
	} else {
		fmt.Printf("OK (%d device(s))\n", len(devices))
		if verbose {
			for _, d := range devices {
				devCfg := cfg.GetDeviceConfig(d.FriendlyName())
				fmt.Printf("      - %s (hw:%d,0) -> %dHz, %dch, %s\n",
					d.Name, d.CardNumber, devCfg.SampleRate, devCfg.Channels, devCfg.Codec)
			}
		}
	}

	// Test 3: FFmpeg command generation
	fmt.Print("[3/5] FFmpeg command: ")
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("FAILED - FFmpeg not found")
		allPassed = false
	} else {
		testArgs := []string{
			"-hide_banner",
			"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
			"-t", "0.1",
			"-c:a", cfg.Default.Codec,
			"-b:a", cfg.Default.Bitrate,
			"-f", "null", "-",
		}
		cmd := exec.Command(ffmpegPath, testArgs...) // #nosec G204 G702 -- ffmpegPath is from exec.LookPath, not user input
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Println("WARNING - FFmpeg test failed")
			allPassed = false
			if verbose {
				fmt.Printf("      %s\n", strings.TrimSpace(string(output)))
			}
		} else {
			fmt.Println("OK")
			if verbose {
				fmt.Printf("      Codec: %s, Bitrate: %s\n", cfg.Default.Codec, cfg.Default.Bitrate)
			}
		}
	}

	// Test 4: MediaMTX connectivity
	fmt.Print("[4/5] MediaMTX API: ")
	apiURL := cfg.MediaMTX.APIURL
	if apiURL == "" {
		apiURL = "http://localhost:9997"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(apiURL + "/v3/paths/list") // #nosec G704 -- apiURL is from config, not user HTTP request input
	if err != nil {
		fmt.Println("WARNING - Not reachable")
		allPassed = false
		if verbose {
			fmt.Printf("      URL: %s\n", apiURL)
			fmt.Printf("      Error: %v\n", err)
		}
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Println("OK")
		} else {
			fmt.Printf("WARNING - Status %d\n", resp.StatusCode)
			allPassed = false
		}
	}

	// Test 5: RTSP URL accessibility
	fmt.Print("[5/5] RTSP port: ")
	rtspURL := cfg.MediaMTX.RTSPURL
	if rtspURL == "" {
		rtspURL = "rtsp://localhost:8554"
	}
	rtspHost := strings.TrimPrefix(rtspURL, "rtsp://")
	if idx := strings.Index(rtspHost, "/"); idx != -1 {
		rtspHost = rtspHost[:idx]
	}
	conn, err := net.DialTimeout("tcp", rtspHost, 2*time.Second) // #nosec G704 -- rtspHost is from config, not user HTTP request input
	if err != nil {
		fmt.Println("WARNING - Not accessible")
		allPassed = false
		if verbose {
			fmt.Printf("      Address: %s\n", rtspHost)
		}
	} else {
		_ = conn.Close()
		fmt.Println("OK")
		if verbose {
			fmt.Printf("      RTSP URL: %s\n", rtspURL)
		}
	}

	fmt.Println()
	if allPassed {
		fmt.Println("All tests passed!")
	} else {
		fmt.Println("Some tests failed. Check the output above for details.")
	}

	return nil
}
