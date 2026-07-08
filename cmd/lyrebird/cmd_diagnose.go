// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/udev"
)

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

	// issues counts every reported problem for the human summary; blocking counts
	// only the deterministic, streaming-blocking ones (missing required binary,
	// unparseable config). M-cli1: the command exits non-zero when blocking > 0
	// so automation can detect real breakage, while environmental gaps that are
	// expected during provisioning (no device yet, ALSA absent in a container,
	// server not started) stay exit 0 with a printed warning.
	issues := 0
	blocking := 0

	// 1. Check FFmpeg
	fmt.Print("FFmpeg: ")
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Println("NOT FOUND - audio encoding will fail")
		issues++
		blocking++ // required for streaming
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
			blocking++ // a config file that exists but won't parse blocks startup
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
	switch {
	case blocking > 0:
		fmt.Printf("Found %d issue(s); %d block streaming and must be fixed.\n", issues, blocking)
	case issues > 0:
		fmt.Printf("Found %d issue(s) that may affect operation.\n", issues)
	default:
		fmt.Println("All checks passed. System is ready for streaming.")
	}

	// B-5 / GAP-9: Create support bundle if --bundle was requested. The bundle is
	// written even when blocking issues exist — capturing a broken system is the
	// whole point — but a bundle-write failure is surfaced first.
	if bundlePath != "" {
		if err := createDiagnosticBundle(bundlePath); err != nil {
			return err
		}
	}
	// M-cli1: non-zero exit on deterministic, streaming-blocking problems.
	if blocking > 0 {
		return fmt.Errorf("diagnostics found %d blocking issue(s); see report above", blocking)
	}
	return nil
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
		return nil
	}
	fmt.Println("System is MISSING required components.")
	fmt.Println("Install FFmpeg: sudo apt-get install ffmpeg")
	// M-cli1: a missing required tool is a deterministic incompatibility, so
	// exit non-zero for scripts and provisioning automation.
	return fmt.Errorf("system is missing required components; see report above")
}
