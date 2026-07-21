// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestRegisterNewDevicesUnsanitizableNameStableIdentity verifies that the
// daemon registers a device whose raw name sanitizes to the TIMESTAMPED
// "unknown_device_<unix>" fallback (e.g. a fully non-ASCII or symbols-only
// USB product string) under a deterministic identity derived from its USB ID.
//
// Without a stable identity, every 10-second device poll computes a NEW name
// for the same physical device (the timestamp advances), so the poller
// registers a brand-new stream service each poll: unbounded growth of
// managers, lock files, goroutines and failing FFmpeg processes over an
// unattended deployment — and the device's config/MediaMTX path changes on
// every poll, so its stream is never usable.
func TestRegisterNewDevicesUnsanitizableNameStableIdentity(t *testing.T) {
	origDetect := detectAudioDevices
	t.Cleanup(func() { detectAudioDevices = origDetect })

	detectAudioDevices = func(string) ([]*audio.Device, error) {
		return []*audio.Device{{
			Name:       "!!!", // sanitizes to nothing → timestamped fallback
			CardNumber: 1,
			USBID:      "0d8c:0014",
			VendorID:   "0d8c",
			ProductID:  "0014",
		}}, nil
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := config.DefaultConfig()
	cfg.Stream.USBStabilizationDelay = 0
	flags := daemonFlags{LockDir: t.TempDir()}
	sup := supervisor.New(supervisor.Config{})

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)
	cards := make(map[string]int)

	if n := registerNewDevices(ctx, logger, cfg, flags, "/fake/ffmpeg", sup, &mu, services, hashes, cards); n != 1 {
		t.Fatalf("first registration: got %d newly registered, want 1", n)
	}

	// The registry key must be the deterministic USB-derived identity, not a
	// timestamped throwaway.
	const wantName = "usb_0d8c_0014"
	if !services[wantName] {
		t.Fatalf("services registered under %v, want key %q (deterministic USB identity)",
			keysOf(services), wantName)
	}

	// A later poll of the SAME device must be a no-op, not a fresh registration.
	if n := registerNewDevices(ctx, logger, cfg, flags, "/fake/ffmpeg", sup, &mu, services, hashes, cards); n != 0 {
		t.Fatalf("re-poll: got %d newly registered, want 0 (identity must be stable across polls)", n)
	}
	if got := sup.ServiceCount(); got != 1 {
		t.Fatalf("after re-poll ServiceCount = %d, want 1", got)
	}
}

// keysOf returns the keys of a string-keyed map for error messages.
func keysOf(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
