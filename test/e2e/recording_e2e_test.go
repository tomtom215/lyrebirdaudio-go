// SPDX-License-Identifier: MIT

//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/stream"
)

// TestE2E_LocalRecordingTee drives the REAL stream.Manager against a real
// MediaMTX server with local recording enabled, so the actual FFmpeg `tee`
// command that buildFFmpegCommand produces (RTSP publish + local segment
// recorder) is exercised end-to-end with a synthetic audio source — no USB
// hardware required.
//
// This is the regression guard for two things unit tests cannot cover:
//  1. The tee muxer option string — including the onfail=ignore guard on the
//     segment slave — is VALID ffmpeg syntax that a real ffmpeg accepts. A
//     malformed option string would make ffmpeg exit immediately, so neither the
//     live stream nor the segments below would ever appear.
//  2. Local recording actually writes segment files to disk while simultaneously
//     publishing a healthy live RTSP stream (the "durability backstop" premise).
func TestE2E_LocalRecordingTee(t *testing.T) {
	mediamtxBin := locateBinary(t, "LYREBIRD_MEDIAMTX_BIN", "mediamtx")
	ffmpegBin := locateBinary(t, "LYREBIRD_FFMPEG_BIN", "ffmpeg")

	client, rtspPort := startMediaMTX(t, mediamtxBin)

	recordDir := t.TempDir()
	lockDir := t.TempDir()
	logDir := t.TempDir() // capture ffmpeg stderr so a failed run is diagnosable

	// opus-in-ogg is a valid codec/container pairing, so the segment recorder can
	// mux the same encoded stream the RTSP output publishes.
	mgrCfg := &stream.ManagerConfig{
		DeviceName:      "e2e_rec",
		InputFormat:     "lavfi",
		RealtimeInput:   true, // pace the synthetic sine to real time for a healthy live publish
		ALSADevice:      "sine=frequency=440:sample_rate=48000",
		StreamName:      "e2e_rec",
		SampleRate:      48000,
		Channels:        2,
		Bitrate:         "128k",
		Codec:           "opus",
		RTSPURL:         "rtsp://127.0.0.1:" + strconv.Itoa(rtspPort) + "/e2e_rec",
		LockDir:         lockDir,
		LogDir:          logDir,
		FFmpegPath:      ffmpegBin,
		LocalRecordDir:  recordDir,
		SegmentDuration: 1, // 1s segments so files appear within the test window
		SegmentFormat:   "ogg",
		StopTimeout:     2 * time.Second,
		Backoff:         stream.NewBackoff(time.Second, 5*time.Second, 10),
	}

	mgr, err := stream.NewManager(mgrCfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- mgr.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runErr:
		case <-time.After(10 * time.Second):
			t.Log("manager did not stop within 10s of cancel")
		}
		_ = mgr.Close()
		// On failure, surface ffmpeg's own stderr (captured by the manager's
		// rotating log writer) so the root cause is visible in CI logs.
		if t.Failed() {
			logPath := filepath.Join(logDir, "ffmpeg-e2e_rec.log")
			if data, rerr := os.ReadFile(logPath); rerr == nil {
				t.Logf("ffmpeg stderr (%s):\n%s", logPath, string(data))
			} else {
				t.Logf("could not read ffmpeg log %s: %v", logPath, rerr)
			}
		}
	})

	// 1. The live RTSP stream must become healthy at MediaMTX.
	waitFor(t, "stream e2e_rec to become healthy", 25*time.Second, func() bool {
		healthy, err := client.IsStreamHealthy(context.Background(), "e2e_rec")
		return err == nil && healthy
	})

	// 2. Local segment files must be written to disk while streaming.
	waitFor(t, "local .ogg recording segments to appear", 15*time.Second, func() bool {
		return countSegments(recordDir, "e2e_rec_", ".ogg") >= 1
	})

	// Confirm the manager did not already exit with an error while we were waiting.
	select {
	case err := <-runErr:
		t.Fatalf("manager.Run exited early: %v", err)
	default:
	}
}

// countSegments returns how many non-empty files in dir start with prefix and
// end with ext. The non-empty check avoids counting a just-created 0-byte
// segment that is still being written.
func countSegments(dir, prefix, ext string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		if info, err := e.Info(); err == nil && info.Size() > 0 {
			n++
		}
	}
	return n
}
