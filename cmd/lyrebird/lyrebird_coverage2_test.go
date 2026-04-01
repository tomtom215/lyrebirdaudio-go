// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// makeBashConfig writes a minimal (empty) bash config file to path.
func makeBashConfig(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("# empty bash config\n"), 0600); err != nil {
		t.Fatalf("makeBashConfig: %v", err)
	}
}

// makeAsoundDir creates a minimal /proc/asound-like structure under root
// with one USB audio card that has a stream0 file for capability detection.
// If busy is true, the pcm0c/sub0/status file is written with RUNNING state.
func makeAsoundDir(t *testing.T, root string, busy bool) {
	t.Helper()
	cardDir := filepath.Join(root, "asound", "card0")
	if err := os.MkdirAll(cardDir, 0750); err != nil {
		t.Fatalf("MkdirAll cardDir: %v", err)
	}

	// card id and usbid required by audio.DetectDevices
	if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("TestMic"), 0644); err != nil {
		t.Fatalf("write id: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "usbid"), []byte("0d8c:0014"), 0644); err != nil {
		t.Fatalf("write usbid: %v", err)
	}

	// stream0 file consumed by audio.DetectCapabilities
	stream0 := "USB Audio\n" +
		"  Status: Stop\n" +
		"  Interface 1\n" +
		"    Altset 1\n" +
		"    Format: S16_LE\n" +
		"    Channels: 2\n" +
		"    Endpoint: 1 IN (ASYNC)\n" +
		"    Rates: 44100, 48000\n"
	if err := os.WriteFile(filepath.Join(cardDir, "stream0"), []byte(stream0), 0644); err != nil {
		t.Fatalf("write stream0: %v", err)
	}

	if busy {
		statusDir := filepath.Join(cardDir, "pcm0c", "sub0")
		if err := os.MkdirAll(statusDir, 0750); err != nil {
			t.Fatalf("MkdirAll statusDir: %v", err)
		}
		statusContent := "state: RUNNING\nowner_pid: 42\n"
		if err := os.WriteFile(filepath.Join(statusDir, "status"), []byte(statusContent), 0644); err != nil {
			t.Fatalf("write status: %v", err)
		}
	}
}

// TestRunMigrateMkdirAllError covers cmd_config.go:66-68 — the os.MkdirAll
// error branch. A regular file is created at the path that would need to be
// the parent directory, causing os.MkdirAll to return ENOTDIR. This fails
// even when running as root because a file cannot be treated as a directory.
func TestRunMigrateMkdirAllError(t *testing.T) {
	tmpDir := t.TempDir()
	bashCfg := filepath.Join(tmpDir, "bash.conf")
	makeBashConfig(t, bashCfg)

	// Create a regular file at "blocked" — attempting os.MkdirAll("blocked/x")
	// fails with ENOTDIR because a file occupies that path.
	blocked := filepath.Join(tmpDir, "blocked")
	if err := os.WriteFile(blocked, []byte("occupied"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	toPath := filepath.Join(blocked, "config.yaml")
	err := runMigrate([]string{"--from=" + bashCfg, "--to=" + toPath, "--force"})
	if err == nil {
		t.Error("runMigrate() expected MkdirAll error, got nil")
	}
}

// TestRunMigrateSaveError covers cmd_config.go:81-83 — the cfg.Save error
// branch. A read-only directory is created at the target parent so that
// os.MkdirAll succeeds (dir exists) but cfg.Save fails (no write permission).
func TestRunMigrateSaveError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skip as root: write-permission is not enforced")
	}

	tmpDir := t.TempDir()
	bashCfg := filepath.Join(tmpDir, "bash.conf")
	makeBashConfig(t, bashCfg)

	// Create target directory then make it read-only.
	targetDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(targetDir, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(targetDir, 0500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	// Restore permissions so t.TempDir cleanup can remove it.
	t.Cleanup(func() { _ = os.Chmod(targetDir, 0750) })

	toPath := filepath.Join(targetDir, "config.yaml")
	err := runMigrate([]string{"--from=" + bashCfg, "--to=" + toPath, "--force"})
	if err == nil {
		t.Error("runMigrate() expected Save error for read-only dir, got nil")
	}
}

// TestRunDetectCapabilitiesSuccess covers cmd_devices.go:101-126 — the
// DetectCapabilities success path in runDetectWithPath, including format,
// sample rate, channel and codec recommendation output. A minimal stream0
// file is written so DetectCapabilities succeeds instead of falling back.
func TestRunDetectCapabilitiesSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	makeAsoundDir(t, tmpDir, false /* not busy */)

	asoundPath := filepath.Join(tmpDir, "asound")
	err := runDetectWithPath(asoundPath, []string{})
	if err != nil {
		t.Errorf("runDetectWithPath() unexpected error: %v", err)
	}
}

// TestRunDetectCapabilitiesSuccessHighQuality covers runDetectWithPath with
// the --quality=high flag to ensure the quality tier parsing and recommendation
// code path is exercised together with a real capabilities struct.
func TestRunDetectCapabilitiesSuccessHighQuality(t *testing.T) {
	tmpDir := t.TempDir()
	makeAsoundDir(t, tmpDir, false /* not busy */)

	asoundPath := filepath.Join(tmpDir, "asound")
	err := runDetectWithPath(asoundPath, []string{"--quality=high"})
	if err != nil {
		t.Errorf("runDetectWithPath(--quality=high) unexpected error: %v", err)
	}
}

// TestRunDetectCapabilitiesBusyDevice covers cmd_devices.go:107-112 — the
// caps.IsBusy branch in the DetectCapabilities success path, which prints
// a "in use (PID X)" status line. The pcm0c/sub0/status file is written
// with "RUNNING" and "owner_pid" to trigger both busy sub-branches.
func TestRunDetectCapabilitiesBusyDevice(t *testing.T) {
	tmpDir := t.TempDir()
	makeAsoundDir(t, tmpDir, true /* busy */)

	asoundPath := filepath.Join(tmpDir, "asound")
	err := runDetectWithPath(asoundPath, []string{})
	if err != nil {
		t.Errorf("runDetectWithPath() busy device unexpected error: %v", err)
	}
}

// TestCreateDiagnosticBundleCreateError covers cmd_bundle.go:98-100 — the
// os.Create error branch in createDiagnosticBundle. A directory is created
// at the output path so os.Create fails with EISDIR.
func TestCreateDiagnosticBundleCreateError(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a directory at the intended output path.
	blockedPath := filepath.Join(tmpDir, "output-is-dir")
	if err := os.Mkdir(blockedPath, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	err := createDiagnosticBundle(blockedPath)
	if err == nil {
		t.Error("createDiagnosticBundle() expected error when output is a directory, got nil")
	}
}

// TestRunDevicesWithPathDeviceID covers cmd_devices.go:40-42 — the
// dev.DeviceID != "" branch. We create a minimal asound structure and a
// by-id directory with a symlink pointing to controlC0, so that
// audio.GetDeviceInfo populates DeviceID via findDeviceIDPathIn.
// NOTE: GetDeviceInfo uses the hardcoded /dev/snd/by-id path; this test
// instead exercises runDevicesWithPath indirectly via a device that has
// no DeviceID (covering the surrounding code). The DeviceID branch itself
// is covered only when /dev/snd/by-id contains matching symlinks, which is
// a real-hardware condition. This test ensures the rest of runDevicesWithPath
// is exercised.
func TestRunDevicesWithPathNoDeviceID(t *testing.T) {
	tmpDir := t.TempDir()
	cardDir := filepath.Join(tmpDir, "asound", "card0")
	if err := os.MkdirAll(cardDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("TestMic"), 0644); err != nil {
		t.Fatalf("write id: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "usbid"), []byte("0d8c:0014"), 0644); err != nil {
		t.Fatalf("write usbid: %v", err)
	}

	err := runDevicesWithPath(filepath.Join(tmpDir, "asound"), []string{})
	if err != nil {
		t.Errorf("runDevicesWithPath() unexpected error: %v", err)
	}
}
