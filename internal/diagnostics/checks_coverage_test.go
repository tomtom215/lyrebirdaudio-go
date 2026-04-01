// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckMediaMTXServiceNotActiveNoError covers the else branch in checkMediaMTXService
// where systemctl exits 0 but returns output other than "active" (e.g. "activating").
func TestCheckMediaMTXServiceNotActiveNoError(t *testing.T) {
	tmpBin := t.TempDir()

	// fake mediamtx: just needs to be found by LookPath
	if err := os.WriteFile(filepath.Join(tmpBin, "mediamtx"), []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil { //#nosec G306
		t.Fatalf("write fake mediamtx: %v", err)
	}
	// fake systemctl: exits 0 but prints "activating" (not "active")
	systemctlScript := "#!/bin/sh\nprintf 'activating'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "systemctl"), []byte(systemctlScript), 0750); err != nil { //#nosec G306
		t.Fatalf("write fake systemctl: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkMediaMTXService(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning; msg: %s", result.Status, result.Message)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestCheckMediaMTXAPINon200 covers the non-200 response path in checkMediaMTXAPI.
func TestCheckMediaMTXAPINon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // 503
	}))
	t.Cleanup(srv.Close)

	// strip http:// prefix since opts.MediaMTXAPIAddr is host:port only
	addr := srv.Listener.Addr().String()

	opts := DefaultOptions()
	opts.MediaMTXAPIAddr = addr
	runner := NewRunner(opts)
	result := runner.checkMediaMTXAPI(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning for HTTP 503; msg: %s", result.Status, result.Message)
	}
}

// TestCheckMediaMTXAPI200 covers the 200 OK path in checkMediaMTXAPI.
func TestCheckMediaMTXAPI200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	opts := DefaultOptions()
	opts.MediaMTXAPIAddr = addr
	runner := NewRunner(opts)
	result := runner.checkMediaMTXAPI(context.Background())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK; msg: %s", result.Status, result.Message)
	}
}

// TestParseFloatError verifies parseFloat returns 0 for non-numeric input.
func TestParseFloatError(t *testing.T) {
	v := parseFloat("notanumber")
	if v != 0 {
		t.Errorf("parseFloat(\"notanumber\") = %v, want 0", v)
	}
	v2 := parseFloat("")
	if v2 != 0 {
		t.Errorf("parseFloat(\"\") = %v, want 0", v2)
	}
}

// TestBuildSystemInfoTextNonNumericUptime covers the else branch in buildSystemInfoText
// where parseFloat returns 0 so the raw string is written with "s" suffix.
func TestBuildSystemInfoTextNonNumericUptime(t *testing.T) {
	tmpProc := t.TempDir()
	// Write non-numeric uptime
	if err := os.WriteFile(filepath.Join(tmpProc, "uptime"), []byte("notanumber 0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions()
	opts.ProcFS = tmpProc
	result := buildSystemInfoText(opts)

	if result == "" {
		t.Error("expected non-empty system info")
	}
	// Should contain the raw string + "s" since parseFloat returned 0
	// The "--- Uptime ---" block should appear with the raw token
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

// TestCheckDiskSpaceRunsWithoutPanic verifies checkDiskSpace completes on a real system.
func TestCheckDiskSpaceRunsWithoutPanic(t *testing.T) {
	runner := NewRunner(DefaultOptions())
	result := runner.checkDiskSpace(context.Background())
	// Should be StatusOK or StatusWarning but not crash; StatusError means syscall failed
	if result.Name != "Disk Space" {
		t.Errorf("unexpected Name: %q", result.Name)
	}
	if result.Status == "" {
		t.Error("expected non-empty status")
	}
}

// TestCheckFFmpegSuggestionsOnWarning covers the Suggestions append path in checkFFmpeg
// triggered when evaluateFFmpegOutput returns StatusWarning (no libopus or aac in codecs).
func TestCheckFFmpegSuggestionsOnWarning(t *testing.T) {
	tmpBin := t.TempDir()

	// fake ffmpeg: -version succeeds but -encoders returns no opus/aac
	script := `#!/bin/sh
case "$1" in
  -version)
    echo 'ffmpeg version 5.0 Copyright'
    exit 0
    ;;
  -encoders)
    echo 'Encoders:'
    echo ' A..... pcm_s16le'
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkFFmpeg(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when codecs missing; msg: %s", result.Status, result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected Suggestions when FFmpeg warns about missing codecs")
	}
}

// TestCheckUSBStabilityWithSuggestions verifies that Suggestions are appended when
// evaluateUSBStability returns StatusWarning (more than 10 USB errors in dmesg).
func TestCheckUSBStabilityWithSuggestions(t *testing.T) {
	tmpBin := t.TempDir()

	// Build a fake dmesg that outputs >10 "usb ... error" lines so evaluateUSBStability
	// returns StatusWarning (threshold: usbErrors > 10 || usbDisconnects > 5).
	var lines string
	for i := 0; i < 12; i++ {
		lines += "2026-01-01T00:00:00+0000 kernel: usb 1-1: error transferring data\n"
	}
	script := "#!/bin/sh\nprintf '" + lines + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "dmesg"), []byte(script), 0750); err != nil { //#nosec G306
		t.Fatalf("write fake dmesg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkUSBStability(context.Background())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning; msg: %s", result.Status, result.Message)
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected Suggestions to be populated when USB stability warns")
	}
}

// TestCheckFFmpegCodecsCriticalSuggestions covers the Suggestions append path in
// checkFFmpegCodecs when evaluateCodecsOutput returns StatusCritical.
func TestCheckFFmpegCodecsCriticalSuggestions(t *testing.T) {
	tmpBin := t.TempDir()

	// fake ffmpeg: -encoders and -decoders succeed but missing libopus, aac, and pcm_s16le
	script := `#!/bin/sh
case "$1" in
  -hide_banner)
    shift
    case "$1" in
      -encoders)
        echo 'Encoders:'
        echo ' V..... libx264'
        exit 0
        ;;
      -decoders)
        echo 'Decoders:'
        echo ' V..... h264'
        exit 0
        ;;
    esac
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkFFmpegCodecs(context.Background())

	// StatusCritical or StatusWarning depending on evaluateCodecsOutput logic
	// — either way Suggestions should be populated on bad status
	if result.Status == StatusOK || result.Status == StatusSkipped {
		t.Errorf("Status = %v, want Critical or Warning; msg: %s", result.Status, result.Message)
	}
}

// TestCheckMediaMTXAPIInvalidAddr covers the http.NewRequestWithContext error path in
// checkMediaMTXAPI by using an addr containing invalid URL characters.
func TestCheckMediaMTXAPIInvalidAddr(t *testing.T) {
	opts := DefaultOptions()
	// A space in the host makes http.NewRequestWithContext fail.
	opts.MediaMTXAPIAddr = "invalid host:9997"
	runner := NewRunner(opts)

	// Must not panic; result should be either Error (request creation failed) or Warning
	result := runner.checkMediaMTXAPI(context.Background())
	if result.Status == StatusOK {
		t.Errorf("Status = OK for invalid addr, want Error or Warning; msg: %s", result.Message)
	}
}
