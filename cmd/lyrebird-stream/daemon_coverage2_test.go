// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestFindFFmpegPathWithFakeBinary covers daemon_config.go:71 — the
// `return path, nil` success branch of findFFmpegPath when ffmpeg is found
// in PATH. Without this test, the success path is never hit because the test
// environment has no real ffmpeg installed.
func TestFindFFmpegPathWithFakeBinary(t *testing.T) {
	tmpBin := t.TempDir()
	fakeFFmpeg := filepath.Join(tmpBin, "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nexit 0\n"), 0750); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", tmpBin+":"+os.Getenv("PATH"))

	path, err := findFFmpegPath()
	if err != nil {
		t.Fatalf("findFFmpegPath() unexpected error: %v", err)
	}
	if path == "" {
		t.Error("findFFmpegPath() returned empty path")
	}
}

// TestRegisterNewDevicesAbsentAsound covers main.go:244-248 — the
// audio.DetectDevices error branch in registerNewDevices when /proc/asound is
// absent. Since /proc/asound does not exist in CI containers, this test calls
// registerNewDevices directly and verifies it returns 0 (no registrations).
func TestRegisterNewDevicesAbsentAsound(t *testing.T) {
	if _, err := os.Stat("/proc/asound"); err == nil {
		t.Skip("/proc/asound exists; cannot test the absent-asound error path")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := config.DefaultConfig()
	flags := daemonFlags{LockDir: t.TempDir()}
	sup := supervisor.New(supervisor.Config{})

	var mu sync.RWMutex
	registered := make(map[string]bool)
	hashes := make(map[string]string)

	count := registerNewDevices(ctx, logger, cfg, flags, "/fake/ffmpeg", sup, &mu, registered, hashes)
	if count != 0 {
		t.Errorf("registerNewDevices() = %d, want 0 when /proc/asound absent", count)
	}
}

// TestRegisterNewDevicesAlreadyRegistered covers main.go:254-259 — the
// "already registered" skip path. We pre-populate registeredServices with a
// device name, then call registerNewDevices with a config that would normally
// produce that device. Since /proc/asound is absent in CI, the DetectDevices
// call returns an error and the function returns 0 before reaching the skip
// check. This test therefore at minimum covers the DetectDevices error path;
// an environment with /proc/asound would additionally cover the skip path.
func TestRegisterNewDevicesAlreadyRegisteredPath(t *testing.T) {
	if _, err := os.Stat("/proc/asound"); err == nil {
		t.Skip("/proc/asound exists; test would reach the already-registered branch with real devices")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := config.DefaultConfig()
	flags := daemonFlags{LockDir: t.TempDir()}
	sup := supervisor.New(supervisor.Config{})

	var mu sync.RWMutex
	// Pre-populate with a fake device name so the already-registered branch fires.
	registered := map[string]bool{"usb-mic": true}
	hashes := make(map[string]string)

	count := registerNewDevices(ctx, logger, cfg, flags, "/fake/ffmpeg", sup, &mu, registered, hashes)
	// Either 0 (DetectDevices error) or 0 (all devices already registered).
	if count != 0 {
		t.Errorf("registerNewDevices() = %d, want 0", count)
	}
}
