// SPDX-License-Identifier: MIT

//go:build e2e

// Package e2e contains hardware-free end-to-end tests that drive the real
// integration stack: an actual MediaMTX server plus a real ffmpeg publisher
// generating a synthetic audio signal (no USB microphone required).
//
// These tests are gated behind the "e2e" build tag so they never run in the
// default `go test ./...` (which must stay fast and deterministic). Run them
// with:
//
//	go test -tags e2e ./test/e2e/...
//
// The MediaMTX and ffmpeg binaries are located via PATH, or via the
// LYREBIRD_MEDIAMTX_BIN / LYREBIRD_FFMPEG_BIN environment variables. If either
// is missing the tests skip rather than fail, so they are safe to include in a
// pipeline that may not always provide the binaries.
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/mediamtx"
)

// locateBinary returns the path to a required external binary, preferring the
// given environment variable and falling back to PATH. It skips the test when
// the binary cannot be found.
func locateBinary(t *testing.T, envVar, name string) string {
	t.Helper()
	if p := os.Getenv(envVar); p != "" {
		return p
	}
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not found on PATH and %s unset; skipping E2E test", name, envVar)
	}
	return p
}

// serverPorts holds the ports for one MediaMTX instance.
type serverPorts struct {
	api  int
	rtsp int
	rtp  int
	rtcp int
}

// reserveServerPorts allocates all ports at once, holding every listener open
// until all are chosen. Reserving them one-at-a-time is racy: each net.Listen
// on :0 that closes before the next call lets the OS hand back the same port,
// so the API and RTSP addresses could collide. RTP/RTCP must be consecutive.
func reserveServerPorts(t *testing.T) serverPorts {
	t.Helper()
	for tries := 0; tries < 100; tries++ {
		apiLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		rtspLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			_ = apiLn.Close()
			continue
		}
		rtpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			_ = apiLn.Close()
			_ = rtspLn.Close()
			continue
		}
		rtp := rtpConn.LocalAddr().(*net.UDPAddr).Port
		// MediaMTX requires the RTP port to be even and RTCP = RTP+1.
		if rtp%2 != 0 {
			_ = apiLn.Close()
			_ = rtspLn.Close()
			_ = rtpConn.Close()
			continue
		}
		rtcpConn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", rtp+1))
		if err != nil {
			// rtp+1 not free; try a fresh set.
			_ = apiLn.Close()
			_ = rtspLn.Close()
			_ = rtpConn.Close()
			continue
		}
		p := serverPorts{
			api:  apiLn.Addr().(*net.TCPAddr).Port,
			rtsp: rtspLn.Addr().(*net.TCPAddr).Port,
			rtp:  rtp,
			rtcp: rtp + 1,
		}
		// Release everything just before the server binds. A small TOCTOU
		// window remains, which the startup retry in startMediaMTX absorbs.
		_ = apiLn.Close()
		_ = rtspLn.Close()
		_ = rtpConn.Close()
		_ = rtcpConn.Close()
		return p
	}
	t.Fatal("could not reserve a free port set")
	return serverPorts{}
}

// startMediaMTX starts a MediaMTX server with the control API enabled and
// returns a client plus the chosen RTSP port. It retries with fresh ports if
// the server fails to come up (e.g. a port was taken in the reservation
// window), so the test is not flaky.
func startMediaMTX(t *testing.T, bin string) (*mediamtx.Client, int) {
	t.Helper()
	for attempt := 1; attempt <= 4; attempt++ {
		ports := reserveServerPorts(t)
		cfgPath := filepath.Join(t.TempDir(), "mediamtx.yml")
		cfg := fmt.Sprintf(`logLevel: error
api: yes
apiAddress: 127.0.0.1:%d
rtspAddress: 127.0.0.1:%d
rtpAddress: 127.0.0.1:%d
rtcpAddress: 127.0.0.1:%d
metrics: no
playback: no
webrtc: no
hls: no
rtmp: no
srt: no
paths:
  all_others:
`, ports.api, ports.rtsp, ports.rtp, ports.rtcp)
		if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
			t.Fatalf("write mediamtx config: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		var out bytes.Buffer
		proc := exec.CommandContext(ctx, bin, cfgPath)
		proc.Stdout = &out
		proc.Stderr = &out
		if err := proc.Start(); err != nil {
			cancel()
			t.Fatalf("start mediamtx: %v", err)
		}

		client := mediamtx.NewClient(fmt.Sprintf("http://127.0.0.1:%d", ports.api))

		// Wait up to 10s for the API, but bail early if the process exited.
		ready := false
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if client.Ping(context.Background()) == nil {
				ready = true
				break
			}
			if proc.ProcessState != nil { // exited
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if ready {
			t.Cleanup(func() {
				cancel()
				_ = proc.Wait()
			})
			return client, ports.rtsp
		}

		// Not ready: tear down and retry with fresh ports.
		cancel()
		_ = proc.Wait()
		t.Logf("mediamtx did not come up on attempt %d (retrying); output:\n%s", attempt, out.String())
	}
	t.Fatal("mediamtx failed to start after several attempts")
	return nil, 0
}

// waitFor polls cond until it returns true or the deadline elapses.
func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, what)
}

// TestE2E_MediaMTXClientAgainstRealServer starts a real MediaMTX server with
// the control API enabled, publishes a synthetic Opus audio stream with real
// ffmpeg, and asserts that every client method the daemon relies on works
// against the real wire format. This is the test that would have caught the
// "tracks decoded as []Track" regression: it exercises GetPath on a live path
// that actually has a track.
func TestE2E_MediaMTXClientAgainstRealServer(t *testing.T) {
	mediamtxBin := locateBinary(t, "LYREBIRD_MEDIAMTX_BIN", "mediamtx")
	ffmpegBin := locateBinary(t, "LYREBIRD_FFMPEG_BIN", "ffmpeg")

	client, rtspPort := startMediaMTX(t, mediamtxBin)
	ctx := context.Background()

	// GetGlobalConfig should confirm the API is enabled (proves the config the
	// installer writes actually works).
	gc, err := client.GetGlobalConfig(ctx)
	if err != nil {
		t.Fatalf("GetGlobalConfig: %v", err)
	}
	if !gc.API {
		t.Errorf("GlobalConfig.API = false, want true")
	}

	// Publish a synthetic Opus stream — no hardware, just a sine tone.
	pubCtx, cancelPub := context.WithCancel(ctx)
	defer cancelPub()
	rtspURL := fmt.Sprintf("rtsp://127.0.0.1:%d/testmic", rtspPort)
	var pubOut bytes.Buffer
	pub := exec.CommandContext(pubCtx, ffmpegBin,
		"-hide_banner", "-loglevel", "error", "-re",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-ac", "2", "-c:a", "libopus",
		"-f", "rtsp", "-rtsp_transport", "tcp", rtspURL,
	)
	pub.Stdout = &pubOut
	pub.Stderr = &pubOut
	if err := pub.Start(); err != nil {
		t.Fatalf("start ffmpeg publisher: %v", err)
	}
	t.Cleanup(func() {
		cancelPub()
		_ = pub.Wait()
		if t.Failed() {
			t.Logf("ffmpeg output:\n%s", pubOut.String())
		}
	})

	// Wait for the stream to become healthy (available + inbound bytes).
	waitFor(t, "stream testmic to become healthy", 20*time.Second, func() bool {
		healthy, err := client.IsStreamHealthy(context.Background(), "testmic")
		return err == nil && healthy
	})

	// ListPaths must include the published path.
	paths, err := client.ListPaths(ctx)
	if err != nil {
		t.Fatalf("ListPaths: %v", err)
	}
	var found bool
	for _, p := range paths {
		if p.Name == "testmic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListPaths did not include testmic; got %d paths", len(paths))
	}

	// GetPath must decode cleanly — the regression guard for the tracks bug.
	path, err := client.GetPath(ctx, "testmic")
	if err != nil {
		t.Fatalf("GetPath(testmic) failed to decode real server response: %v", err)
	}
	if !path.IsAvailable() {
		t.Error("GetPath: path not available")
	}
	if len(path.Tracks) == 0 || path.Tracks[0] != "Opus" {
		t.Errorf("GetPath: Tracks = %v, want [Opus]", path.Tracks)
	}
	if len(path.Tracks2) == 0 || path.Tracks2[0].Codec != "Opus" {
		t.Errorf("GetPath: Tracks2 = %+v, want one Opus track", path.Tracks2)
	}
	if path.TotalInboundBytes() <= 0 {
		t.Error("GetPath: TotalInboundBytes() <= 0, expected data flowing")
	}

	// GetStreamStats must classify the audio codec.
	stats, err := client.GetStreamStats(ctx, "testmic")
	if err != nil {
		t.Fatalf("GetStreamStats: %v", err)
	}
	if stats.AudioCodec != "Opus" {
		t.Errorf("GetStreamStats: AudioCodec = %q, want Opus", stats.AudioCodec)
	}
	if stats.Channels != 2 {
		t.Errorf("GetStreamStats: Channels = %d, want 2", stats.Channels)
	}

	// ListRTSPSessions must show the publishing session.
	sessions, err := client.ListRTSPSessions(ctx)
	if err != nil {
		t.Fatalf("ListRTSPSessions: %v", err)
	}
	var publishSeen bool
	for _, s := range sessions {
		if s.Path == "testmic" && s.State == "publish" {
			publishSeen = true
		}
	}
	if !publishSeen {
		t.Errorf("ListRTSPSessions did not show a publish session for testmic; got %+v", sessions)
	}
}
