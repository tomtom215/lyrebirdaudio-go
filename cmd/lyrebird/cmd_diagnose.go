// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/udev"
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

	allPassed := true

	// Test 1: Config syntax and validation
	fmt.Print("[1/5] Config syntax: ")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("FAILED\n      %v\n", err)
		// Can't continue without valid config
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
		// Test that FFmpeg can at least parse a basic command
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
	// Extract host:port from RTSP URL
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

// runDiagnose runs system diagnostics and optionally creates a support bundle.
func runDiagnose(args []string) error {
	// Parse --bundle flag (B-5 / GAP-9)
	bundlePath := ""
	remaining := args[:0]
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--bundle=") {
			bundlePath = strings.TrimPrefix(args[i], "--bundle=")
		} else if args[i] == "--bundle" && i+1 < len(args) {
			i++
			bundlePath = args[i]
		} else {
			remaining = append(remaining, args[i])
		}
	}
	_ = remaining

	fmt.Println("LyreBird System Diagnostics")
	fmt.Println("===========================")
	fmt.Println()

	issues := 0

	// 1. Check FFmpeg
	fmt.Print("FFmpeg: ")
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("NOT FOUND - audio encoding will fail")
		issues++
	} else {
		// Get version
		cmd := exec.Command(ffmpegPath, "-version") // #nosec G204 -- ffmpegPath is from exec.LookPath
		output, _ := cmd.Output()
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			fmt.Println(strings.TrimSpace(lines[0]))
		} else {
			fmt.Printf("found at %s\n", ffmpegPath)
		}
	}

	// 2. Check ALSA tools
	fmt.Print("ALSA (arecord): ")
	if _, err := exec.LookPath("arecord"); err != nil {
		fmt.Println("NOT FOUND - may affect device detection")
		issues++
	} else {
		fmt.Println("OK")
	}

	// 3. Check /proc/asound
	fmt.Print("/proc/asound: ")
	if _, err := os.Stat("/proc/asound"); os.IsNotExist(err) {
		fmt.Println("NOT FOUND - ALSA not available")
		issues++
	} else {
		fmt.Println("OK")
	}

	// 4. Check USB audio devices
	fmt.Print("USB Audio Devices: ")
	devices, err := audio.DetectDevices("/proc/asound")
	if err != nil {
		fmt.Printf("error - %v\n", err)
		issues++
	} else if len(devices) == 0 {
		fmt.Println("none detected")
	} else {
		fmt.Printf("%d device(s) found\n", len(devices))
		for _, dev := range devices {
			fmt.Printf("  - Card %d: %s\n", dev.CardNumber, dev.Name)
		}
	}

	// 5. Check udev rules
	fmt.Print("udev Rules: ")
	if _, err := os.Stat(udev.RulesFilePath); os.IsNotExist(err) {
		fmt.Printf("NOT CONFIGURED (%s not found)\n", udev.RulesFilePath)
		fmt.Println("  Run 'sudo lyrebird usb-map' to create persistent device mappings")
	} else {
		fmt.Printf("OK (%s exists)\n", udev.RulesFilePath)
	}

	// 6. Check config file
	fmt.Print("Configuration: ")
	if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
		fmt.Println("using defaults (no config file)")
	} else {
		cfg, err := config.LoadConfig(defaultConfigPath)
		if err != nil {
			fmt.Printf("ERROR - %v\n", err)
			issues++
		} else {
			fmt.Printf("OK (%d device config(s))\n", len(cfg.Devices))
		}
	}

	// 7. Check systemd service
	fmt.Print("Service (lyrebird-stream): ")
	status := getServiceStatus("lyrebird-stream")
	fmt.Println(status)

	// 8. Check lock directory
	fmt.Print("Lock Directory: ")
	lockDir := "/var/run/lyrebird"
	if info, err := os.Stat(lockDir); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("NOT CREATED (%s)\n", lockDir)
			fmt.Println("  Will be created when lyrebird-stream starts")
		} else {
			fmt.Printf("ERROR - %v\n", err)
			issues++
		}
	} else if !info.IsDir() {
		fmt.Printf("ERROR - %s is not a directory\n", lockDir)
		issues++
	} else {
		fmt.Println("OK")
	}

	// 9. Check MediaMTX (optional)
	fmt.Print("MediaMTX: ")
	if _, err := exec.LookPath("mediamtx"); err != nil {
		// Check if running as a service
		cmd := exec.Command("systemctl", "is-active", "mediamtx")
		if output, _ := cmd.Output(); strings.TrimSpace(string(output)) == "active" {
			fmt.Println("running (systemd service)")
		} else {
			fmt.Println("NOT FOUND or NOT RUNNING")
			fmt.Println("  Install MediaMTX: sudo lyrebird install-mediamtx")
		}
	} else {
		fmt.Println("found in PATH")
	}

	fmt.Println()
	if issues > 0 {
		fmt.Printf("Found %d issue(s) that may affect operation.\n", issues)
	} else {
		fmt.Println("All checks passed. System is ready for streaming.")
	}

	// B-5 / GAP-9: Create support bundle if --bundle was requested.
	if bundlePath != "" {
		return createDiagnosticBundle(bundlePath)
	}
	return nil
}

// createDiagnosticBundle collects diagnostic information into a tar.gz archive
// suitable for remote support engineers (GAP-9 / B-5).
func createDiagnosticBundle(outputPath string) error {
	// Sanitize output path to prevent path traversal.
	outputPath = filepath.Clean(outputPath)
	fmt.Printf("\nCreating diagnostic bundle: %s\n", outputPath)

	// Collect data into a temporary directory.
	tmpDir, err := os.MkdirTemp("", "lyrebird-bundle-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	writeFile := func(name, content string) {
		if err := os.WriteFile(filepath.Join(tmpDir, filepath.Clean(name)), []byte(content), 0600); err != nil {
			fmt.Printf("  warning: failed to write %s: %v\n", name, err)
		}
	}

	runCmd := func(name string, cmdArgs ...string) string {
		// #nosec G204 -- cmdArgs are from hardcoded lists, not user input
		out, err := exec.Command(cmdArgs[0], cmdArgs[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Sprintf("command failed: %v\n%s", err, out)
		}
		return string(out)
	}

	// Collect system info.
	writeFile("system_info.txt", runCmd("uname", "uname", "-a"))
	writeFile("os_release.txt", func() string {
		data, _ := os.ReadFile("/etc/os-release")
		return string(data)
	}())
	writeFile("uptime.txt", runCmd("uptime", "uptime"))
	writeFile("dmesg.txt", runCmd("dmesg", "dmesg", "--time-format=iso", "-T"))

	// Collect lyrebird diagnostics.
	writeFile("lyrebird_status.txt", runCmd("lyrebird status", "lyrebird", "status"))
	writeFile("lyrebird_devices.txt", runCmd("lyrebird devices", "lyrebird", "devices"))

	// Collect service logs (last 500 lines).
	writeFile("journalctl.txt", runCmd("journalctl", "journalctl", "-u", "lyrebird-stream", "-n", "500", "--no-pager"))
	writeFile("journalctl_mediamtx.txt", runCmd("journalctl mediamtx", "journalctl", "-u", "mediamtx", "-n", "100", "--no-pager"))

	// Collect config (if exists).
	if data, err := os.ReadFile(defaultConfigPath); err == nil {
		writeFile("config.yaml", string(data))
	}

	// Collect health endpoint response.
	httpClient := &http.Client{Timeout: 5 * time.Second}
	if resp, err := httpClient.Get("http://127.0.0.1:9998/healthz"); err == nil { //#nosec G107 -- localhost only
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		writeFile("healthz.json", string(body))
	}
	if resp, err := httpClient.Get("http://127.0.0.1:9998/metrics"); err == nil { //#nosec G107 -- localhost only
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		writeFile("metrics.txt", string(body))
	}

	// Create tar.gz archive.
	outFile, err := os.Create(outputPath) //#nosec G304 -- outputPath is from CLI argument
	if err != nil {
		return fmt.Errorf("failed to create bundle file: %w", err)
	}
	defer outFile.Close()

	if err := createTarGz(outFile, tmpDir); err != nil {
		return fmt.Errorf("failed to create bundle archive: %w", err)
	}

	fmt.Printf("Bundle created: %s\n", outputPath)
	fmt.Println("Send this file to support for remote analysis.")
	return nil
}

// createTarGz creates a gzip-compressed tar archive of srcDir at outFile.
func createTarGz(outFile *os.File, srcDir string) error {
	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		hdr := &tar.Header{
			Name:    relPath,
			Mode:    0600,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tarWriter.WriteHeader(hdr); err != nil {
			return err
		}

		// #nosec G304,G122 -- path is from filepath.Walk on our own tmpDir
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tarWriter, f)
		return err
	})
}

// runCheckSystem checks system compatibility.
func runCheckSystem(args []string) error {
	fmt.Println("System Compatibility Check")
	fmt.Println("==========================")
	fmt.Println()

	compatible := true

	// Kernel version
	fmt.Print("Kernel: ")
	if data, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			fmt.Println(parts[2])
		}
	} else {
		fmt.Println("unknown")
	}

	// Check if running as root (for full access)
	fmt.Print("Running as root: ")
	if os.Geteuid() == 0 {
		fmt.Println("yes")
	} else {
		fmt.Println("no (some features require sudo)")
	}

	// Check audio group membership
	fmt.Print("Audio group: ")
	cmd := exec.Command("groups")
	if output, err := cmd.Output(); err == nil {
		groups := string(output)
		if strings.Contains(groups, "audio") {
			fmt.Println("member")
		} else {
			fmt.Println("NOT A MEMBER - may need: sudo usermod -a -G audio $USER")
		}
	} else {
		fmt.Println("unknown")
	}

	// Required binaries
	required := []string{"ffmpeg"}
	optional := []string{"arecord", "aplay", "udevadm", "systemctl"}

	fmt.Println()
	fmt.Println("Required Tools:")
	for _, tool := range required {
		fmt.Printf("  %s: ", tool)
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Println("MISSING")
			compatible = false
		} else {
			fmt.Println("OK")
		}
	}

	fmt.Println()
	fmt.Println("Optional Tools:")
	for _, tool := range optional {
		fmt.Printf("  %s: ", tool)
		if _, err := exec.LookPath(tool); err != nil {
			fmt.Println("not found")
		} else {
			fmt.Println("OK")
		}
	}

	fmt.Println()
	if compatible {
		fmt.Println("System is compatible with LyreBirdAudio.")
	} else {
		fmt.Println("System is MISSING required components.")
		fmt.Println("Install FFmpeg: sudo apt-get install ffmpeg")
	}

	return nil
}
