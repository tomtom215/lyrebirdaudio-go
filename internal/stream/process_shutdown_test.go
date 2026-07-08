// SPDX-License-Identifier: MIT

package stream

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestShutdownDeliversSIGINTForFinalization verifies that on context
// cancellation ffmpeg receives a graceful SIGINT (so the tee/segment muxer can
// write its container trailer) rather than an immediate SIGKILL. The previous
// implementation built the command with exec.CommandContext(ctx), so os/exec
// SIGKILLed ffmpeg the instant the context was cancelled, truncating the
// in-progress recording segment on every shutdown/reload/restart.
//
// The fake ffmpeg traps SIGINT, writes a marker file ("finalizes"), and exits
// cleanly. If it were SIGKILLed the trap would never run and the marker would
// be absent — which is exactly what this test guards against.
func TestShutdownDeliversSIGINTForFinalization(t *testing.T) {
	lockDir := t.TempDir()
	scriptDir := t.TempDir()
	marker := filepath.Join(scriptDir, "finalized")

	scriptPath := filepath.Join(scriptDir, "ff.sh")
	script := fmt.Sprintf("#!/bin/sh\ntrap 'echo done > %q; exit 0' INT\nwhile true; do sleep 0.05; done\n", marker)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil { //nolint:gosec // test helper script must be executable
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	cfg := &ManagerConfig{
		DeviceName:   "test_sigint",
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
		Backoff:      NewBackoff(1*time.Second, 10*time.Second, 5),
		StopTimeout:  3 * time.Second, // generous; the trap exits well within it
	}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.startFFmpeg(ctx) }()

	if !waitForState(t, mgr, StateRunning, 3*time.Second) {
		cancel()
		t.Fatal("fake ffmpeg never reached running state")
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("startFFmpeg did not return after context cancellation")
	}

	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("ffmpeg was killed before it could finalize (SIGINT not delivered): marker missing: %v", statErr)
	}
}
