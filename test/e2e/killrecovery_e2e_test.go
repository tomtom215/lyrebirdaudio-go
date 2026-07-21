// SPDX-License-Identifier: MIT

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/mediamtx"
	"github.com/tomtom215/lyrebirdaudio-go/internal/stream"
)

// TestE2E_FFmpegKillRecovery is the core 24/7 recovery loop under real fault
// injection: a stream.Manager publishes a synthetic Opus stream to a real
// MediaMTX server, and the test SIGKILLs the real ffmpeg process mid-publish
// (simulating an ffmpeg crash, an OOM kill, or a USB stack reset taking the
// process down). The manager must detect the exit, back off, restart ffmpeg,
// and the stream must become healthy again on the SAME MediaMTX server —
// twice in a row, proving backoff state does not wedge recovery.
//
// After shutdown, the test asserts the process's fd count returns to its
// pre-manager baseline: restart cycles must not leak descriptors (lock files,
// log writers, stderr pipes) — the resource-exhaustion failure mode that
// kills month-long unattended deployments.
func TestE2E_FFmpegKillRecovery(t *testing.T) {
	mediamtxBin := locateBinary(t, "LYREBIRD_MEDIAMTX_BIN", "mediamtx")
	ffmpegBin := locateBinary(t, "LYREBIRD_FFMPEG_BIN", "ffmpeg")

	client, rtspPort := startMediaMTX(t, mediamtxBin)

	lockDir := t.TempDir()
	logDir := t.TempDir()
	const streamName = "e2e_kill"
	rtspURL := fmt.Sprintf("rtsp://127.0.0.1:%d/%s", rtspPort, streamName)

	baselineFDs := countOpenFDs(t)

	mgrCfg := &stream.ManagerConfig{
		DeviceName:    streamName,
		InputFormat:   "lavfi",
		RealtimeInput: true,
		ALSADevice:    "sine=frequency=440:sample_rate=48000",
		StreamName:    streamName,
		SampleRate:    48000,
		Channels:      2,
		Bitrate:       "128k",
		Codec:         "opus",
		RTSPURL:       rtspURL,
		LockDir:       lockDir,
		LogDir:        logDir,
		FFmpegPath:    ffmpegBin,
		StopTimeout:   2 * time.Second,
		// Short backoff so recovery happens within the test window; enough
		// attempts that two kills can never exhaust the budget.
		Backoff: stream.NewBackoff(500*time.Millisecond, 2*time.Second, 20),
	}

	mgr, err := stream.NewManager(mgrCfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- mgr.Run(ctx) }()
	stopped := false // set once the test body has shut the manager down itself
	t.Cleanup(func() {
		if !stopped {
			cancel()
			select {
			case <-runErr:
			case <-time.After(10 * time.Second):
				t.Log("manager did not stop within 10s of cancel")
			}
		}
		_ = mgr.Close()
		if t.Failed() {
			logPath := filepath.Join(logDir, "ffmpeg-"+streamName+".log")
			if data, rerr := os.ReadFile(logPath); rerr == nil {
				t.Logf("ffmpeg stderr (%s):\n%s", logPath, string(data))
			}
		}
	})

	healthy := func() bool {
		ok, herr := client.IsStreamHealthy(context.Background(), streamName)
		return herr == nil && ok
	}

	waitFor(t, "initial publish to become healthy", 30*time.Second, healthy)

	for cycle := 1; cycle <= 2; cycle++ {
		pid := findFFmpegPID(t, rtspURL)
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			t.Fatalf("cycle %d: SIGKILL ffmpeg pid %d: %v", cycle, pid, err)
		}
		t.Logf("cycle %d: killed ffmpeg pid %d", cycle, pid)

		// MediaMTX must notice the publisher died...
		waitFor(t, fmt.Sprintf("cycle %d: stream to go unhealthy after kill", cycle),
			20*time.Second, func() bool { return !healthy() })

		// ...and the manager must restart ffmpeg and re-publish.
		waitFor(t, fmt.Sprintf("cycle %d: stream to recover after kill", cycle),
			30*time.Second, healthy)

		// The manager must still be in its run loop, not exited.
		select {
		case err := <-runErr:
			t.Fatalf("cycle %d: manager.Run exited: %v", cycle, err)
		default:
		}
	}

	// Shut down and verify fd hygiene: everything the restart cycles opened
	// (lock file, rotating log, stderr pipes) must be closed again. Poll to
	// let exec's pipe-drain goroutines finish.
	cancel()
	select {
	case <-runErr:
	case <-time.After(10 * time.Second):
		t.Fatal("manager did not stop within 10s of cancel")
	}
	stopped = true
	if err := mgr.Close(); err != nil {
		t.Errorf("manager Close: %v", err)
	}

	const fdSlack = 2 // tolerate transient runtime fds (netpoll etc.)
	deadline := time.Now().Add(10 * time.Second)
	var finalFDs int
	for time.Now().Before(deadline) {
		finalFDs = countOpenFDs(t)
		if finalFDs <= baselineFDs+fdSlack {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("fd count after 2 kill/restart cycles = %d, baseline %d (+%d slack): restart cycles leak file descriptors",
		finalFDs, baselineFDs, fdSlack)
}

// findFFmpegPID locates the manager's ffmpeg child by the unique RTSP URL on
// its command line, via /proc (no pgrep dependency).
func findFFmpegPID(t *testing.T, rtspURL string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir("/proc")
		if err != nil {
			t.Fatalf("read /proc: %v", err)
		}
		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}
			cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
			if err != nil {
				continue // process vanished or not ours
			}
			args := strings.Split(string(cmdline), "\x00")
			if len(args) == 0 || !strings.Contains(args[0], "ffmpeg") {
				continue
			}
			for _, a := range args[1:] {
				if a == rtspURL {
					return pid
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no ffmpeg process publishing %s found within 10s", rtspURL)
	return 0
}

// countOpenFDs returns the number of open file descriptors of this process.
func countOpenFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	return len(entries)
}

// TestE2E_MediaMTXRestartRecovery injects the OTHER side's failure: the
// MediaMTX server itself is killed and restarted on the same ports while the
// stream.Manager keeps running. FFmpeg's RTSP-over-TCP connection dies with
// the server, the manager's backoff restarts ffmpeg, and once the new server
// instance is up the stream must become healthy again — the field scenario of
// a MediaMTX crash/redeploy at 3 AM with nobody watching.
func TestE2E_MediaMTXRestartRecovery(t *testing.T) {
	mediamtxBin := locateBinary(t, "LYREBIRD_MEDIAMTX_BIN", "mediamtx")
	ffmpegBin := locateBinary(t, "LYREBIRD_FFMPEG_BIN", "ffmpeg")

	server := startRestartableMediaMTX(t, mediamtxBin)

	lockDir := t.TempDir()
	logDir := t.TempDir()
	const streamName = "e2e_mtxrestart"
	rtspURL := fmt.Sprintf("rtsp://127.0.0.1:%d/%s", server.ports.rtsp, streamName)

	mgrCfg := &stream.ManagerConfig{
		DeviceName:    streamName,
		InputFormat:   "lavfi",
		RealtimeInput: true,
		ALSADevice:    "sine=frequency=440:sample_rate=48000",
		StreamName:    streamName,
		SampleRate:    48000,
		Channels:      2,
		Bitrate:       "128k",
		Codec:         "opus",
		RTSPURL:       rtspURL,
		LockDir:       lockDir,
		LogDir:        logDir,
		FFmpegPath:    ffmpegBin,
		StopTimeout:   2 * time.Second,
		Backoff:       stream.NewBackoff(500*time.Millisecond, 2*time.Second, 30),
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
		if t.Failed() {
			logPath := filepath.Join(logDir, "ffmpeg-"+streamName+".log")
			if data, rerr := os.ReadFile(logPath); rerr == nil {
				t.Logf("ffmpeg stderr (%s):\n%s", logPath, string(data))
			}
		}
	})

	healthy := func() bool {
		ok, herr := server.client.IsStreamHealthy(context.Background(), streamName)
		return herr == nil && ok
	}

	waitFor(t, "initial publish to become healthy", 30*time.Second, healthy)

	// Kill the server. FFmpeg loses its TCP connection and exits; the manager
	// enters backoff-restart against a dead server (connection refused).
	server.kill(t)
	waitFor(t, "API to go down after server kill", 10*time.Second, func() bool {
		return server.client.Ping(context.Background()) != nil
	})

	// Give the manager time to burn a couple of failed restart attempts
	// against the dead server, then bring MediaMTX back on the SAME ports.
	time.Sleep(2 * time.Second)
	server.restart(t)

	waitFor(t, "stream to recover after MediaMTX restart", 45*time.Second, healthy)

	select {
	case err := <-runErr:
		t.Fatalf("manager.Run exited during MediaMTX restart: %v", err)
	default:
	}
}

// restartableServer is a MediaMTX instance that can be killed and restarted
// on the same ports within one test.
type restartableServer struct {
	bin     string
	cfgPath string
	ports   serverPorts
	client  *mediamtx.Client
	proc    *exec.Cmd
}

// startRestartableMediaMTX starts MediaMTX like startMediaMTX but keeps the
// process handle and config file so the test can kill and restart the server
// on the same ports. Startup retries with fresh ports absorb reservation races.
func startRestartableMediaMTX(t *testing.T, bin string) *restartableServer {
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

		proc := exec.Command(bin, cfgPath) // #nosec G204 -- test binary path
		if err := proc.Start(); err != nil {
			t.Fatalf("start mediamtx: %v", err)
		}

		client := mediamtx.NewClient(fmt.Sprintf("http://127.0.0.1:%d", ports.api))
		ready := false
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if client.Ping(context.Background()) == nil {
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if ready {
			s := &restartableServer{bin: bin, cfgPath: cfgPath, ports: ports, client: client, proc: proc}
			t.Cleanup(func() { s.stop() })
			return s
		}

		_ = proc.Process.Kill()
		_ = proc.Wait()
		t.Logf("mediamtx did not come up on attempt %d (retrying)", attempt)
	}
	t.Fatal("mediamtx failed to start after several attempts")
	return nil
}

func (s *restartableServer) kill(t *testing.T) {
	t.Helper()
	if s.proc != nil && s.proc.Process != nil {
		if err := s.proc.Process.Kill(); err != nil {
			t.Fatalf("kill mediamtx: %v", err)
		}
		_ = s.proc.Wait()
		s.proc = nil
	}
}

func (s *restartableServer) restart(t *testing.T) {
	t.Helper()
	proc := exec.Command(s.bin, s.cfgPath) // #nosec G204 -- test binary path
	if err := proc.Start(); err != nil {
		t.Fatalf("restart mediamtx: %v", err)
	}
	s.proc = proc
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if s.client.Ping(context.Background()) == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("restarted mediamtx did not come up within 15s")
}

func (s *restartableServer) stop() {
	if s.proc != nil && s.proc.Process != nil {
		_ = s.proc.Process.Kill()
		_ = s.proc.Wait()
		s.proc = nil
	}
}
