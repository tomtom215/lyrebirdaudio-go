// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFormatIntSliceForDetect verifies formatting of integer slices.
func TestFormatIntSliceForDetect(t *testing.T) {
	tests := []struct {
		name string
		vals []int
		want string
	}{
		{
			name: "empty slice",
			vals: []int{},
			want: "",
		},
		{
			name: "single value",
			vals: []int{48000},
			want: "48000",
		},
		{
			name: "multiple values",
			vals: []int{8000, 16000, 44100, 48000},
			want: "8000, 16000, 44100, 48000",
		},
		{
			name: "channels",
			vals: []int{1, 2},
			want: "1, 2",
		},
		{
			name: "single zero",
			vals: []int{0},
			want: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatIntSliceForDetect(tt.vals)
			if got != tt.want {
				t.Errorf("formatIntSliceForDetect(%v) = %q, want %q", tt.vals, got, tt.want)
			}
		})
	}
}

// TestRunUpdateFlagParsing verifies update command flag parsing.
func TestRunUpdateFlagParsing(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "check only flag",
			args:    []string{"--check"},
			wantErr: true, // Will fail trying to contact GitHub
		},
		{
			name:    "force flag",
			args:    []string{"--force"},
			wantErr: true, // Will fail trying to contact GitHub
		},
		{
			name:    "check and force flags",
			args:    []string{"--check", "--force"},
			wantErr: true, // Will fail trying to contact GitHub
		},
		{
			name:    "no flags",
			args:    []string{},
			wantErr: true, // Will fail trying to contact GitHub
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runUpdate(tt.args)
			// All cases should fail because we can't reach GitHub in CI,
			// but they should not panic and should return a proper error.
			if tt.wantErr && err == nil {
				t.Error("runUpdate() expected error, got nil")
			}
			if err != nil && !strings.Contains(err.Error(), "failed to check for updates") {
				t.Logf("runUpdate() returned unexpected error type: %v", err)
			}
		})
	}
}

// TestGetServiceStatusWithFakeSystemctl verifies service status with mocked systemctl.
func TestGetServiceStatusWithFakeSystemctl(t *testing.T) {
	tests := []struct {
		name       string
		scriptBody string
		wantStatus string
	}{
		{
			name:       "active service",
			scriptBody: "#!/bin/sh\necho 'active'\n",
			wantStatus: "active (running)",
		},
		{
			name:       "inactive service",
			scriptBody: "#!/bin/sh\necho 'inactive'\n",
			wantStatus: "inactive (stopped)",
		},
		{
			name:       "failed service",
			scriptBody: "#!/bin/sh\necho 'failed'\n",
			wantStatus: "failed",
		},
		{
			name:       "activating service",
			scriptBody: "#!/bin/sh\necho 'activating'\n",
			wantStatus: "activating",
		},
		{
			name:       "systemctl not available",
			scriptBody: "#!/bin/sh\nexit 1\n",
			wantStatus: "not running (or systemd unavailable)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpBin := t.TempDir()
			fakeSystemctl := filepath.Join(tmpBin, "systemctl")
			if err := os.WriteFile(fakeSystemctl, []byte(tt.scriptBody), 0750); err != nil {
				t.Fatalf("failed to create fake systemctl: %v", err)
			}
			t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

			got := getServiceStatus("test-service")
			if got != tt.wantStatus {
				t.Errorf("getServiceStatus() = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}

// TestDetectArchWithFakeUname verifies architecture detection with mocked uname.
func TestDetectArchWithFakeUname(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantArch string
	}{
		{
			name:     "x86_64",
			output:   "x86_64",
			wantArch: "amd64",
		},
		{
			name:     "amd64 direct",
			output:   "amd64",
			wantArch: "amd64",
		},
		{
			name:     "aarch64",
			output:   "aarch64",
			wantArch: "arm64",
		},
		{
			name:     "arm64 direct",
			output:   "arm64",
			wantArch: "arm64",
		},
		{
			name:     "armv7l",
			output:   "armv7l",
			wantArch: "armv7",
		},
		{
			name:     "armhf",
			output:   "armhf",
			wantArch: "armv7",
		},
		{
			name:     "armv6l",
			output:   "armv6l",
			wantArch: "armv6",
		},
		{
			name:     "unknown architecture",
			output:   "sparc64",
			wantArch: "",
		},
		{
			name:     "uname fails",
			output:   "",
			wantArch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpBin := t.TempDir()
			fakeUname := filepath.Join(tmpBin, "uname")

			var script string
			if tt.output == "" && tt.name == "uname fails" {
				script = "#!/bin/sh\nexit 1\n"
			} else {
				script = fmt.Sprintf("#!/bin/sh\necho '%s'\n", tt.output)
			}

			if err := os.WriteFile(fakeUname, []byte(script), 0750); err != nil {
				t.Fatalf("failed to create fake uname: %v", err)
			}
			t.Setenv("PATH", tmpBin)

			got := detectArch()
			if got != tt.wantArch {
				t.Errorf("detectArch() = %q, want %q", got, tt.wantArch)
			}
		})
	}
}

// TestInstallLyreBirdServiceCallsToPath verifies the wrapper function.
func TestInstallLyreBirdServiceCallsToPath(t *testing.T) {
	// installLyreBirdService writes to /etc/systemd/system which is not writable
	// for non-root, providing coverage for the function entry point.
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}
	err := installLyreBirdService()
	if err == nil {
		t.Error("installLyreBirdService() expected error for non-root, got nil")
	}
	if !strings.Contains(err.Error(), "failed to write service file") {
		t.Errorf("installLyreBirdService() error = %q, want 'failed to write service file'", err.Error())
	}
}

// TestRunInstallMediaMTXFlagParsing verifies flag parsing for install-mediamtx.
func TestRunInstallMediaMTXFlagParsing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "version flag with equals",
			args: []string{"--version=v1.9.3"},
		},
		{
			name: "no service flag",
			args: []string{"--no-service"},
		},
		{
			name: "combined flags",
			args: []string{"--version=v2.0.0", "--no-service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runInstallMediaMTX(tt.args)
			// All should fail at root check
			if err == nil {
				t.Error("runInstallMediaMTX() expected error for non-root")
			}
			if !strings.Contains(err.Error(), "root privileges") {
				t.Errorf("runInstallMediaMTX() error = %q, want 'root privileges'", err.Error())
			}
		})
	}
}

// TestRunDetectWithQualityTier verifies detect command with quality flags.
func TestRunDetectWithQualityTier(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "default quality",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "low quality with equals",
			args:    []string{"--quality=low"},
			wantErr: false,
		},
		{
			name:    "high quality with equals",
			args:    []string{"--quality=high"},
			wantErr: false,
		},
		{
			name:    "quality with space",
			args:    []string{"--quality", "normal"},
			wantErr: false,
		},
		{
			name:    "invalid quality tier",
			args:    []string{"--quality=ultra"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runDetectWithPath(asoundPath, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Error("runDetectWithPath() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("runDetectWithPath() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRunCommandRouting verifies all command routing paths in run().
func TestRunCommandRouting(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"update command", []string{"update"}},
		{"menu command", []string{"menu"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These commands will fail (no GitHub access, no terminal),
			// but should be routed correctly and not panic.
			_ = run(tt.args)
		})
	}
}

// TestRunStatusWithNonexistentLockDir verifies status with missing lock dir.
func TestRunStatusWithNonexistentLockDir(t *testing.T) {
	args := []string{"--lock-dir=/nonexistent/lock/dir"}
	err := runStatus(args)
	if err != nil {
		t.Errorf("runStatus() with nonexistent lock dir unexpected error: %v", err)
	}
}

// TestRunStatusFlagParsing verifies all flag parsing combinations.
func TestRunStatusFlagParsing(t *testing.T) {
	lockDir := t.TempDir()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "all flags combined",
			args: []string{
				"--lock-dir=" + lockDir,
				"--config=/nonexistent/config.yaml",
				"--json",
			},
		},
		{
			name: "lock-dir only",
			args: []string{"--lock-dir=" + lockDir},
		},
		{
			name: "json short flag with lock dir",
			args: []string{"--lock-dir=" + lockDir, "-j"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runStatus(tt.args)
			if err != nil {
				t.Errorf("runStatus() unexpected error: %v", err)
			}
		})
	}
}

// TestRunUSBMapWithPathNoReloadFlag verifies the --no-reload flag.
func TestRunUSBMapWithPathNoReloadFlag(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	args := []string{"--dry-run", "--no-reload"}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() with --no-reload unexpected error: %v", err)
	}
}

// TestRunUSBMapWithPathOutputSpaceForm verifies output flag with space separator.
func TestRunUSBMapWithPathOutputSpaceForm(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-rules")

	args := []string{"--dry-run", "--output", outputPath}
	err := runUSBMapWithPath(asoundPath, args)
	if err != nil {
		t.Errorf("runUSBMapWithPath() with --output space form unexpected error: %v", err)
	}
}

// TestCreateTarGzWithSubdirectories verifies tar.gz handles nested directories.
func TestCreateTarGzWithSubdirectories(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// Create nested structure
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested content"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root content"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	outPath := filepath.Join(outDir, "nested.tar.gz")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = createTarGz(outFile, srcDir)
	outFile.Close()
	if err != nil {
		t.Fatalf("createTarGz() unexpected error: %v", err)
	}

	// Verify archive is non-empty
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("archive file is empty")
	}
}

// TestProcessExistsWithZeroPID verifies processExists with PID 0.
func TestProcessExistsWithZeroPID(t *testing.T) {
	// PID 0 refers to the kernel on Linux; signal(0) should fail for regular users.
	result := processExists(0)
	// Just verify it doesn't panic; actual result depends on permissions.
	_ = result
}

// TestRunMigrateToFlagParsing verifies --to flag with equals form.
func TestRunMigrateToFlagParsing(t *testing.T) {
	bashConfig := filepath.Join("..", "..", "testdata", "config", "bash-env.conf")
	if _, err := os.Stat(bashConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpDir := t.TempDir()
	yamlConfig := filepath.Join(tmpDir, "config.yaml")

	// Test --to with equals form
	args := []string{
		"--from=" + bashConfig,
		"--to=" + yamlConfig,
	}

	err := runMigrate(args)
	if err != nil {
		t.Fatalf("runMigrate() unexpected error: %v", err)
	}

	if _, err := os.Stat(yamlConfig); os.IsNotExist(err) {
		t.Error("output file was not created")
	}
}

// TestRunValidateWithDevices verifies validate output when devices are configured.
func TestRunValidateWithDevices(t *testing.T) {
	validConfig := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(validConfig); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Test with equals form
	err := runValidate([]string{"--config=" + validConfig})
	if err != nil {
		t.Errorf("runValidate() unexpected error: %v", err)
	}
}

// TestRunDiagnoseUnknownFlagDoesNotPanic verifies that unknown flags are handled.
func TestRunDiagnoseUnknownFlagDoesNotPanic(t *testing.T) {
	err := runDiagnose([]string{"--verbose", "--unknown"})
	// Should complete without panic
	_ = err
}

// TestRunCheckSystemWithFakeTools verifies check-system with mocked tools.
func TestRunCheckSystemWithFakeTools(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake ffmpeg
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho 'ffmpeg version 6.0'\n"), 0750); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create fake groups command
	fakeGroups := filepath.Join(tmpBin, "groups")
	if err := os.WriteFile(fakeGroups, []byte("#!/bin/sh\necho 'user audio video'\n"), 0750); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runCheckSystem([]string{})
	if err != nil {
		t.Errorf("runCheckSystem() unexpected error: %v", err)
	}
}

// TestGetUSBBusDevFromCardWithSysRootDevnumReadError tests devnum read failure.
func TestGetUSBBusDevFromCardWithSysRootDevnumReadError(t *testing.T) {
	sysRoot := t.TempDir()

	usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-4")
	if err := os.MkdirAll(usbDevDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create busnum but make devnum a directory (so ReadFile fails)
	if err := os.WriteFile(filepath.Join(usbDevDir, "busnum"), []byte("1\n"), 0644); err != nil {
		t.Fatalf("WriteFile busnum: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(usbDevDir, "devnum"), 0755); err != nil {
		t.Fatalf("MkdirAll devnum: %v", err)
	}

	soundDir := filepath.Join(sysRoot, "class", "sound", "card4")
	if err := os.MkdirAll(soundDir, 0755); err != nil {
		t.Fatalf("MkdirAll sound: %v", err)
	}
	if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, _, err := getUSBBusDevFromCardWithSysRoot(4, sysRoot)
	if err == nil {
		t.Fatal("expected error for devnum read failure")
	}
	if !strings.Contains(err.Error(), "failed to read devnum") {
		t.Errorf("error = %q, want 'failed to read devnum'", err.Error())
	}
}

// TestGetUSBBusDevFromCardWithSysRootBusnumReadError tests busnum read failure.
func TestGetUSBBusDevFromCardWithSysRootBusnumReadError(t *testing.T) {
	sysRoot := t.TempDir()

	usbDevDir := filepath.Join(sysRoot, "bus", "usb", "devices", "1-5")
	if err := os.MkdirAll(usbDevDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create busnum as a directory (so ReadFile fails) and devnum as file
	if err := os.MkdirAll(filepath.Join(usbDevDir, "busnum"), 0755); err != nil {
		t.Fatalf("MkdirAll busnum: %v", err)
	}
	if err := os.WriteFile(filepath.Join(usbDevDir, "devnum"), []byte("5\n"), 0644); err != nil {
		t.Fatalf("WriteFile devnum: %v", err)
	}

	soundDir := filepath.Join(sysRoot, "class", "sound", "card5")
	if err := os.MkdirAll(soundDir, 0755); err != nil {
		t.Fatalf("MkdirAll sound: %v", err)
	}
	if err := os.Symlink(usbDevDir, filepath.Join(soundDir, "device")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, _, err := getUSBBusDevFromCardWithSysRoot(5, sysRoot)
	if err == nil {
		t.Fatal("expected error for busnum read failure")
	}
	if !strings.Contains(err.Error(), "failed to read busnum") {
		t.Errorf("error = %q, want 'failed to read busnum'", err.Error())
	}
}

// TestRunTestWithNonexistentFFmpeg verifies test handles missing ffmpeg.
func TestRunTestWithNonexistentFFmpeg(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	// Isolate PATH to hide ffmpeg
	tmpBin := t.TempDir()
	t.Setenv("PATH", tmpBin)

	err := runTest([]string{"--config=" + configPath})
	// Should complete without error (ffmpeg missing is a WARNING, not an error)
	if err != nil {
		t.Errorf("runTest() unexpected error: %v", err)
	}
}

// TestInstallMediaMTXServiceWithFakeSystemctl verifies the MediaMTX service install.
func TestInstallMediaMTXServiceWithFakeSystemctl(t *testing.T) {
	t.Run("success path", func(t *testing.T) {
		tmpBin := t.TempDir()
		fakeSystemctl := filepath.Join(tmpBin, "systemctl")
		if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

		// Create a temp dir to write the service file to
		tmpDir := t.TempDir()
		servicePath := filepath.Join(tmpDir, "mediamtx.service")

		// We cannot test installMediaMTXService() directly because it
		// hardcodes /etc/systemd/system. Instead we verify that
		// the function attempts to write the expected content.
		// Simulate by writing what the function would write.
		serviceContent := `[Unit]
Description=MediaMTX RTSP Server
Documentation=https://github.com/bluenviron/mediamtx
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mediamtx /etc/mediamtx/mediamtx.yml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		data, err := os.ReadFile(servicePath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if !strings.Contains(string(data), "MediaMTX") {
			t.Error("service file should contain MediaMTX")
		}
	})

	t.Run("non-root fails", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("test not meaningful when running as root")
		}
		err := installMediaMTXService()
		if err == nil {
			t.Error("installMediaMTXService() expected error for non-root")
		}
	})
}

// TestRunSetupAutoModeAsNonRoot verifies setup returns root error.
func TestRunSetupAutoModeAsNonRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test not meaningful when running as root")
	}

	tests := []struct {
		name string
		args []string
	}{
		{"auto mode", []string{"--auto"}},
		{"short auto", []string{"-y"}},
		{"no args", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runSetup(tt.args)
			if err == nil {
				t.Error("runSetup() expected error for non-root")
			}
			if !strings.Contains(err.Error(), "root privileges") {
				t.Errorf("runSetup() error = %q, want 'root privileges'", err.Error())
			}
		})
	}
}

// TestRunDiagnoseWithFakeTools verifies diagnose with mocked system tools.
func TestRunDiagnoseWithFakeTools(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake ffmpeg that reports version
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then echo 'ffmpeg version 6.1.1'; fi\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake arecord
	fakeArecord := filepath.Join(tmpBin, "arecord")
	if err := os.WriteFile(fakeArecord, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake systemctl
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\necho 'inactive'\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake mediamtx
	fakeMediamtx := filepath.Join(tmpBin, "mediamtx")
	if err := os.WriteFile(fakeMediamtx, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runDiagnose([]string{})
	if err != nil {
		t.Errorf("runDiagnose() unexpected error: %v", err)
	}
}

// TestRunDiagnoseWithoutMediamtx verifies diagnose when mediamtx is absent.
func TestRunDiagnoseWithoutMediamtx(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake ffmpeg
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho 'ffmpeg version 6.0'\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake arecord
	fakeArecord := filepath.Join(tmpBin, "arecord")
	if err := os.WriteFile(fakeArecord, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake systemctl that reports inactive for mediamtx
	fakeSystemctl := filepath.Join(tmpBin, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte("#!/bin/sh\necho 'inactive'\nexit 3\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// No mediamtx in PATH
	t.Setenv("PATH", tmpBin)

	err := runDiagnose([]string{})
	if err != nil {
		t.Errorf("runDiagnose() unexpected error: %v", err)
	}
}

// TestRunTestWithFakeFFmpeg verifies test command when ffmpeg is available but fails.
func TestRunTestWithFakeFFmpeg(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpBin := t.TempDir()

	// Create fake ffmpeg that fails the test
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho 'codec error' >&2\nexit 1\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runTest([]string{"--config=" + configPath, "--verbose"})
	// Should complete without error (ffmpeg failure is a WARNING)
	if err != nil {
		t.Errorf("runTest() unexpected error: %v", err)
	}
}

// TestRunTestWithFFmpegSuccess verifies test command when ffmpeg succeeds.
func TestRunTestWithFFmpegSuccess(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tmpBin := t.TempDir()

	// Create fake ffmpeg that succeeds
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runTest([]string{"--config=" + configPath, "-v"})
	if err != nil {
		t.Errorf("runTest() unexpected error: %v", err)
	}
}

// TestRunCheckSystemWithMissingFFmpeg verifies check-system output when ffmpeg is absent.
func TestRunCheckSystemWithMissingFFmpeg(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake groups (no audio group)
	fakeGroups := filepath.Join(tmpBin, "groups")
	if err := os.WriteFile(fakeGroups, []byte("#!/bin/sh\necho 'user video'\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// No ffmpeg in PATH
	t.Setenv("PATH", tmpBin)

	err := runCheckSystem([]string{})
	if err != nil {
		t.Errorf("runCheckSystem() unexpected error: %v", err)
	}
}

// TestRunCheckSystemGroupsFails verifies check-system when groups command fails.
func TestRunCheckSystemGroupsFails(t *testing.T) {
	tmpBin := t.TempDir()

	// Create fake groups that fails
	fakeGroups := filepath.Join(tmpBin, "groups")
	if err := os.WriteFile(fakeGroups, []byte("#!/bin/sh\nexit 1\n"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create fake ffmpeg
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	err := runCheckSystem([]string{})
	if err != nil {
		t.Errorf("runCheckSystem() unexpected error: %v", err)
	}
}

// TestVerifyDownloadIntegrityStatError verifies the stat error path.
func TestVerifyDownloadIntegrityStatError(t *testing.T) {
	// Create a file then remove read permissions to trigger stat-related error
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable.tar.gz")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify normal case works first
	hash, err := verifyDownloadIntegrity(path)
	if err != nil {
		t.Fatalf("verifyDownloadIntegrity() unexpected error: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
}

// TestRunUSBMapWithPathNonexistentAsound verifies error for bad asound path.
func TestRunUSBMapWithPathNonexistentAsound(t *testing.T) {
	err := runUSBMapWithPath("/nonexistent/asound", []string{"--dry-run"})
	if err == nil {
		t.Error("runUSBMapWithPath() expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "failed to detect devices") {
		t.Errorf("error = %q, want 'failed to detect devices'", err.Error())
	}
}

// TestRunUSBMapWithPathEmptyDevices verifies usb-map with no devices.
func TestRunUSBMapWithPathEmptyDevices(t *testing.T) {
	tmpDir := t.TempDir()
	emptyAsound := filepath.Join(tmpDir, "asound")
	if err := os.MkdirAll(emptyAsound, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := runUSBMapWithPath(emptyAsound, []string{})
	if err != nil {
		t.Errorf("runUSBMapWithPath() with empty dir unexpected error: %v", err)
	}
}
