// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckProcessStabilitySkippedPath verifies that checkProcessStability
// handles the case where the journalctl command fails (returns "skipped" status).
// Uses a pre-cancelled context to make exec.CommandContext fail immediately.
func TestCheckProcessStabilitySkippedPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancel so exec.CommandContext fails immediately.

	runner := NewRunner(DefaultOptions())
	result := runner.checkProcessStability(ctx)

	if result.Name != "Process Stability" {
		t.Errorf("Name = %q, want %q", result.Name, "Process Stability")
	}
	// With a cancelled context, journalctl cannot run → StatusOK + "skipped" message.
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when journalctl unavailable", result.Status)
	}
	if result.Message != "Process stability check skipped" {
		t.Errorf("Message = %q, want %q", result.Message, "Process stability check skipped")
	}
}

// TestCheckTimeSynchronizationSuccessPath verifies that checkTimeSynchronization
// calls evaluateTimeSyncOutput when timedatectl exits successfully.
// Creates a fake timedatectl script in a temp directory and prepends it to PATH.
func TestCheckTimeSynchronizationSuccessPath(t *testing.T) {
	// Build a fake timedatectl that exits 0 and prints timedatectl-like output.
	tmpBin := t.TempDir()
	script := "#!/bin/sh\nprintf 'NTPSynchronized=yes\\nNTPService=active\\nSystemNTPService=systemd-timesyncd\\n'\n"
	scriptPath := filepath.Join(tmpBin, "timedatectl")
	if err := os.WriteFile(scriptPath, []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake timedatectl: %v", err)
	}

	// Prepend our fake binary directory to PATH; t.Setenv restores on cleanup.
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkTimeSynchronization(context.Background())

	if result.Name != "Time Sync" {
		t.Errorf("Name = %q, want %q", result.Name, "Time Sync")
	}
	// evaluateTimeSyncOutput should be called; result should be StatusOK or StatusWarning.
	if result.Status == StatusError {
		t.Errorf("unexpected StatusError: %s", result.Message)
	}
	if result.Message == "Time sync check skipped (timedatectl not available)" {
		t.Error("expected evaluateTimeSyncOutput to be called, got skipped message")
	}
}

// TestCheckProcessStabilitySuccessPath verifies that checkProcessStability
// calls evaluateProcessRestarts when journalctl exits successfully.
// Uses a fake journalctl that outputs nothing (no restarts detected).
func TestCheckProcessStabilitySuccessPath(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake journalctl: exit 0, empty output.
	script := "#!/bin/sh\nexit 0\n"
	scriptPath := filepath.Join(tmpBin, "journalctl")
	if err := os.WriteFile(scriptPath, []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake journalctl: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkProcessStability(context.Background())

	if result.Name != "Process Stability" {
		t.Errorf("Name = %q, want %q", result.Name, "Process Stability")
	}
	if result.Message == "Process stability check skipped" {
		t.Error("expected evaluateProcessRestarts to be called, not skipped")
	}
	// Empty output → no restarts detected → StatusOK.
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK for empty journalctl output", result.Status)
	}
}

// TestCheckPrerequisitesAllPresent verifies the StatusOK path when all required
// tools are available. Uses fake binaries in a temp PATH.
func TestCheckPrerequisitesAllPresent(t *testing.T) {
	tmpBin := t.TempDir()
	// Create fake executables for all required and optional tools.
	for _, name := range []string{"ffmpeg", "arecord", "aplay", "udevadm", "systemctl"} {
		script := "#!/bin/sh\nexit 0\n"
		if err := os.WriteFile(filepath.Join(tmpBin, name), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
			t.Fatalf("write fake %s: %v", name, err)
		}
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkPrerequisites(context.Background())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when all tools present; msg: %s", result.Status, result.Message)
	}
	if result.Message != "All required tools available" {
		t.Errorf("Message = %q, want %q", result.Message, "All required tools available")
	}
}

// TestCheckPrerequisitesMissingOptional verifies the StatusWarning path when
// all required tools are present but some optional tools are missing.
func TestCheckPrerequisitesMissingOptional(t *testing.T) {
	tmpBin := t.TempDir()
	// Only create the required tool (ffmpeg); omit optional ones.
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	// Use ONLY our temp dir so optional tools (arecord etc.) are not found.
	t.Setenv("PATH", tmpBin)

	runner := NewRunner(DefaultOptions())
	result := runner.checkPrerequisites(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when optional tools missing; msg: %s", result.Status, result.Message)
	}
}

// TestCheckVersionsWithFakeFFmpeg verifies that checkVersions includes the ffmpeg
// version when the binary is available. Uses a fake ffmpeg that prints version info.
func TestCheckVersionsWithFakeFFmpeg(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ffmpeg that prints a version line.
	script := "#!/bin/sh\necho 'ffmpeg version 6.0 Copyright (c) 2000-2023 the FFmpeg developers'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkVersions(context.Background())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Details == "" {
		t.Error("expected version details to be populated with fake ffmpeg output")
	}
}

// TestCheckUSBStabilitySkippedPath verifies the StatusSkipped path when dmesg fails.
// Uses a pre-cancelled context to make exec.CommandContext fail immediately.
func TestCheckUSBStabilitySkippedPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := NewRunner(DefaultOptions())
	result := runner.checkUSBStability(ctx)

	if result.Name != "USB Stability" {
		t.Errorf("Name = %q, want %q", result.Name, "USB Stability")
	}
	if result.Status != StatusSkipped {
		t.Errorf("Status = %v, want StatusSkipped when dmesg fails", result.Status)
	}
}

// TestCheckUSBStabilityWithFakeDmesg verifies that evaluateUSBStability is called
// when dmesg exits successfully. Uses a fake dmesg with clean output (no USB errors).
func TestCheckUSBStabilityWithFakeDmesg(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake dmesg: exit 0, no USB error/warn lines.
	script := "#!/bin/sh\necho '2026-01-01T00:00:00+0000 kernel: USB device attached'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "dmesg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake dmesg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkUSBStability(context.Background())

	if result.Status == StatusSkipped {
		t.Error("expected evaluateUSBStability to be called, not skipped")
	}
}

// TestCheckFFmpegCodecsWithFakeFFmpeg verifies that evaluateCodecsOutput is called
// when a fake ffmpeg binary is available.
func TestCheckFFmpegCodecsWithFakeFFmpeg(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ffmpeg: outputs minimal encoder list when called with -encoders,
	// minimal decoder list when called with -decoders.
	script := `#!/bin/sh
case "$*" in
  *-encoders*) printf ' A....  libopus\n A....  aac\n'; exit 0;;
  *-decoders*) printf ' A....  pcm_s16le\n'; exit 0;;
  *) exit 0;;
esac
`
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkFFmpegCodecs(context.Background())

	if result.Name != "FFmpeg Codecs" {
		t.Errorf("Name = %q, want %q", result.Name, "FFmpeg Codecs")
	}
	// With libopus, aac, and pcm_s16le present, should be StatusOK.
	if result.Status == StatusSkipped {
		t.Error("expected checkFFmpegCodecs to run, not skip (fake ffmpeg should be found)")
	}
}

// TestCheckFFmpegWithFakeBinary verifies that evaluateFFmpegOutput is called when
// a fake ffmpeg binary is available and version check succeeds.
func TestCheckFFmpegWithFakeBinary(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ffmpeg that prints a version string and encoder list.
	script := `#!/bin/sh
case "$*" in
  *-version*) echo 'ffmpeg version 6.0 Copyright (c) 2000-2023 the FFmpeg developers'; exit 0;;
  *-encoders*) printf ' A....  libopus\n A....  aac\n'; exit 0;;
  *) exit 0;;
esac
`
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkFFmpeg(context.Background())

	if result.Name != "FFmpeg" {
		t.Errorf("Name = %q, want %q", result.Name, "FFmpeg")
	}
	if result.Status == StatusCritical && result.Message == "FFmpeg not found" {
		t.Error("expected fake ffmpeg to be found via PATH, got 'not found'")
	}
}

// TestCheckAudioCapabilitiesAmixerFound verifies the StatusOK path when amixer
// is available and the info command succeeds.
func TestCheckAudioCapabilitiesAmixerFound(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake amixer: exit 0 with minimal output.
	script := "#!/bin/sh\necho 'ALSA mixer info'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "amixer"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake amixer: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkAudioCapabilities(context.Background())

	if result.Name != "Audio Capabilities" {
		t.Errorf("Name = %q, want %q", result.Name, "Audio Capabilities")
	}
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when amixer succeeds; msg: %s", result.Status, result.Message)
	}
	if result.Details == "" {
		t.Error("expected Details to be populated from amixer info output")
	}
}

// TestCheckAudioCapabilitiesAmixerFails verifies the StatusWarning path when amixer
// is installed but the info command fails.
func TestCheckAudioCapabilitiesAmixerFails(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake amixer: exit 1 (command fails).
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "amixer"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake amixer: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkAudioCapabilities(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when amixer info fails; msg: %s", result.Status, result.Message)
	}
	if result.Message != "ALSA mixer check failed" {
		t.Errorf("Message = %q, want %q", result.Message, "ALSA mixer check failed")
	}
}

// TestCheckTCPResourcesSkippedPath verifies the "skipped" path when ss fails.
func TestCheckTCPResourcesSkippedPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := NewRunner(DefaultOptions())
	result := runner.checkTCPResources(ctx)

	if result.Name != "TCP Resources" {
		t.Errorf("Name = %q, want %q", result.Name, "TCP Resources")
	}
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when ss is skipped", result.Status)
	}
	if result.Message != "TCP check skipped" {
		t.Errorf("Message = %q, want %q", result.Message, "TCP check skipped")
	}
}

// TestCheckMediaMTXServiceFound verifies paths when mediamtx is installed.
func TestCheckMediaMTXServiceFound(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake mediamtx binary (just needs to exist in PATH).
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "mediamtx"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake mediamtx: %v", err)
	}
	// Fake systemctl: reports mediamtx as "active".
	systemctlScript := "#!/bin/sh\necho 'active'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "systemctl"), []byte(systemctlScript), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake systemctl: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkMediaMTXService(context.Background())

	if result.Name != "MediaMTX Service" {
		t.Errorf("Name = %q, want %q", result.Name, "MediaMTX Service")
	}
	// mediamtx binary found + systemctl reports active → StatusOK.
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when mediamtx active; msg: %s", result.Status, result.Message)
	}
}
