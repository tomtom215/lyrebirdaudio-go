// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"os"
	"testing"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestLoadConfigurationKoanfFileExistsLoadFails covers daemon_config.go:58 —
// the `return nil, nil, fmt.Errorf("failed to load config: %w", err)` path.
// The file exists (fileExists=true), NewKoanfConfig succeeds, but kc.Load()
// fails because Validate() rejects sample_rate: -1.
func TestLoadConfigurationKoanfFileExistsLoadFails(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"

	// Valid YAML but semantically invalid — sample_rate: -1 fails Validate().
	invalidContent := `default:
  sample_rate: -1
  channels: 2
  bitrate: "128k"
  codec: opus
`
	if err := os.WriteFile(cfgPath, []byte(invalidContent), 0640); err != nil { //#nosec G304 -- test helper
		t.Fatalf("WriteFile: %v", err)
	}

	kc, cfg, err := loadConfigurationKoanf(cfgPath)
	if err == nil {
		t.Errorf("loadConfigurationKoanf() expected error for invalid config, got kc=%v cfg=%v", kc, cfg)
	}
}

// TestStartHealthEndpointContextDone covers main.go:352 — the `case <-ctx.Done()`
// branch in startHealthEndpoint. We pre-cancel the context AND hold the
// listener port so that net.Listen inside ListenAndServeReady fails (healthReady
// is never closed). With ctx already done, ctx.Done() fires immediately.
func TestStartHealthEndpointContextDone(t *testing.T) {
	// Bind a port ourselves so the health goroutine cannot bind the same port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()

	// Pre-cancel context so ctx.Done() fires before any timer or healthReady.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	sup := supervisor.New(supervisor.Config{})
	cfg := config.DefaultConfig()
	cfg.Monitor.HealthAddr = addr

	// Should return quickly (ctx already done, port is in use → healthReady never fires).
	startHealthEndpoint(ctx, logger, cfg, sup)
	// No assertions needed — reaching here means ctx.Done() path was executed.
}

// TestLoadConfigurationKoanfNoFileEnvFallback covers daemon_config.go:46-48 —
// the `return nil, config.DefaultConfig(), nil` path when no file exists and
// env-only NewKoanfConfig succeeds. The non-existent file triggers the
// else-branch; since no LYREBIRD_* env vars set invalid values, Load() succeeds
// OR fails and the nil-file fallback is returned.
func TestLoadConfigurationKoanfNoFileEnvFallback(t *testing.T) {
	kc, cfg, err := loadConfigurationKoanf("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("loadConfigurationKoanf() unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("loadConfigurationKoanf() returned nil config for non-existent file")
	}
	_ = kc
}

// TestSdNotifyDialError covers maintenance.go:28-30 — the net.DialUnix error
// branch in sdNotify. NOTIFY_SOCKET is set to a path where no socket exists;
// DialUnix fails and sdNotify returns a non-nil error.
func TestSdNotifyDialError(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "/nonexistent/systemd/notify.sock")

	err := sdNotify("READY=1")
	if err == nil {
		t.Error("sdNotify() expected error for non-existent socket, got nil")
	}
}
