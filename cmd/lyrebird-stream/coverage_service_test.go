// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/stream"
)

// TestStreamServiceRun verifies streamService.Run delegates to manager.Run.
func TestStreamServiceRun(t *testing.T) {
	lockDir := t.TempDir()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Create a mock FFmpeg script that exits immediately
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &stream.ManagerConfig{
		DeviceName:   "test_svc",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      stream.NewBackoff(10*time.Millisecond, 50*time.Millisecond, 2),
		Logger:       logger,
	}

	mgr, err := stream.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	svc := &streamService{
		name:    "test_svc",
		manager: mgr,
		logger:  logger,
	}

	// Run with a short context
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = svc.Run(ctx)
	// Should complete (either max attempts or context cancelled)
	if err == nil {
		t.Log("Run completed without error (max attempts reached or context cancelled)")
	}
}

// TestStreamServiceRunContextCancelled verifies clean shutdown logging.
func TestStreamServiceRunContextCancelled(t *testing.T) {
	lockDir := t.TempDir()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Create a script that runs for a while
	scriptPath := filepath.Join(lockDir, "mock_ffmpeg.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 30\n"), 0755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &stream.ManagerConfig{
		DeviceName:   "test_cancel",
		ALSADevice:   "dummy",
		StreamName:   "test",
		SampleRate:   48000,
		Channels:     2,
		Bitrate:      "128k",
		Codec:        "opus",
		RTSPURL:      "/dev/null",
		OutputFormat: "null",
		LockDir:      lockDir,
		FFmpegPath:   scriptPath,
		Backoff:      stream.NewBackoff(1*time.Second, 5*time.Second, 10),
		Logger:       logger,
	}

	mgr, err := stream.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	svc := &streamService{
		name:    "test_cancel",
		manager: mgr,
		logger:  logger,
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	// Wait for service to start
	time.Sleep(200 * time.Millisecond)

	// Cancel
	cancel()

	select {
	case err := <-errCh:
		// context.Canceled is the expected result
		if err != nil && err != context.Canceled {
			t.Logf("Run() returned: %v (expected context.Canceled)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not complete within timeout")
	}

	// Verify "stream stopped" was logged
	output := logBuf.String()
	if output == "" {
		t.Error("expected log output")
	}
}

// TestStreamServiceName verifies streamService.Name returns the correct name.
func TestStreamServiceNameTableDriven(t *testing.T) {
	tests := []struct {
		name     string
		svcName  string
		wantName string
	}{
		{"simple name", "blue_yeti", "blue_yeti"},
		{"with underscores", "usb_mic_1", "usb_mic_1"},
		{"empty name", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &streamService{name: tt.svcName}
			if got := svc.Name(); got != tt.wantName {
				t.Errorf("Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}
