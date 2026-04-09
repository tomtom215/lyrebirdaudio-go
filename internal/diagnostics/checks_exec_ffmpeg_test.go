// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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

// TestCheckFFmpegCodecsEncoderQueryError verifies the StatusError path when the
// ffmpeg -encoders query fails.
func TestCheckFFmpegCodecsEncoderQueryError(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ffmpeg: exits 1 on any call.
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkFFmpegCodecs(context.Background())

	if result.Status != StatusError {
		t.Errorf("Status = %v, want StatusError when ffmpeg -encoders fails; msg: %s", result.Status, result.Message)
	}
}

// TestCheckFFmpegCodecsDecoderQueryError verifies the StatusError path when the
// ffmpeg -decoders query fails (but -encoders succeeds).
func TestCheckFFmpegCodecsDecoderQueryError(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ffmpeg: succeeds on -encoders but fails on -decoders.
	script := `#!/bin/sh
case "$*" in
  *-encoders*) printf ' A....  libopus\n A....  aac\n'; exit 0;;
  *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkFFmpegCodecs(context.Background())

	if result.Status != StatusError {
		t.Errorf("Status = %v, want StatusError when ffmpeg -decoders fails; msg: %s", result.Status, result.Message)
	}
}

// TestCheckFFmpegVersionFails verifies the StatusWarning path when ffmpeg is
// found but the -version command fails.
func TestCheckFFmpegVersionFails(t *testing.T) {
	tmpBin := t.TempDir()
	// Fake ffmpeg: always exits 1.
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(tmpBin, "ffmpeg"), []byte(script), 0750); err != nil { //#nosec G306 -- test helper executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpBin+":"+originalPath)

	runner := NewRunner(DefaultOptions())
	result := runner.checkFFmpeg(context.Background())

	// ffmpeg found but -version fails → StatusWarning.
	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when -version fails; msg: %s", result.Status, result.Message)
	}
	if result.Message != "FFmpeg found but version check failed" {
		t.Errorf("Message = %q, want %q", result.Message, "FFmpeg found but version check failed")
	}
}
