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

// TestRegisterNewDevicesCardNumberChange verifies the USB re-enumeration fix:
// when an already-registered device reappears on a DIFFERENT ALSA card number,
// registerNewDevices tears down the stale stream (pinned to the old hw:<card>,0)
// and re-registers it against the new card number on the same poll, instead of
// leaving the manager driving the wrong/gone card for hours until backoff
// exhaustion and the 5-minute failed-stream recovery rebuild it.
func TestRegisterNewDevicesCardNumberChange(t *testing.T) {
	// Inject a synthetic device list; real registration is otherwise unreachable
	// without USB hardware under /proc/asound.
	origDetect := detectAudioDevices
	t.Cleanup(func() { detectAudioDevices = origDetect })

	card := 1
	detectAudioDevices = func(string) ([]*audio.Device, error) {
		return []*audio.Device{{Name: "usb_mic", CardNumber: card, USBID: "0d8c:0014"}}, nil
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := config.DefaultConfig()
	cfg.Stream.USBStabilizationDelay = 0 // keep the test fast (no 5s-per-registration wait)
	// LogDir empty => no rotating-log fd is opened for the abandoned manager.
	flags := daemonFlags{LockDir: t.TempDir()}
	sup := supervisor.New(supervisor.Config{})

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)
	cards := make(map[string]int)

	devName := audio.SanitizeDeviceName("usb_mic")

	// First poll: registers usb_mic on card 1.
	if n := registerNewDevices(ctx, logger, cfg, flags, "/fake/ffmpeg", sup, &mu, services, hashes, cards); n != 1 {
		t.Fatalf("first registration: got %d newly registered, want 1", n)
	}
	if got := cards[devName]; got != 1 {
		t.Fatalf("after first registration cards[%q] = %d, want 1", devName, got)
	}
	if !services[devName] {
		t.Fatalf("after first registration services[%q] = false, want true", devName)
	}
	if sup.ServiceCount() != 1 {
		t.Fatalf("after first registration ServiceCount = %d, want 1", sup.ServiceCount())
	}

	// Second poll, SAME card: no-op (already registered, card unchanged).
	if n := registerNewDevices(ctx, logger, cfg, flags, "/fake/ffmpeg", sup, &mu, services, hashes, cards); n != 0 {
		t.Fatalf("re-poll with unchanged card: got %d, want 0 (no re-registration)", n)
	}
	if sup.ServiceCount() != 1 {
		t.Fatalf("after unchanged re-poll ServiceCount = %d, want 1", sup.ServiceCount())
	}

	// Device re-enumerates to card 2.
	card = 2
	if n := registerNewDevices(ctx, logger, cfg, flags, "/fake/ffmpeg", sup, &mu, services, hashes, cards); n != 1 {
		t.Fatalf("after card change: got %d newly registered, want 1 (stale stream restarted on new card)", n)
	}
	if got := cards[devName]; got != 2 {
		t.Errorf("after card change cards[%q] = %d, want 2", devName, got)
	}
	if !services[devName] {
		t.Errorf("after card change services[%q] = false, want true", devName)
	}
	// Old stream removed, new one added: still exactly one service.
	if sup.ServiceCount() != 1 {
		t.Errorf("after card change ServiceCount = %d, want 1 (old removed, new added)", sup.ServiceCount())
	}
}
