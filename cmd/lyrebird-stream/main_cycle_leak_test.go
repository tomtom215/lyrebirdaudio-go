// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/audio"
	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// TestRegisterRemoveCyclesNoResourceLeak drives the daemon's real
// register→run→remove wiring (supervisor + streamService + stream.Manager +
// flock + rotating log writer + a spawned mock-ffmpeg process) through many
// full cycles — the exact loop the stall detector, SIGHUP reload handler and
// failed-stream recovery execute for months on an unattended station — and
// asserts that file descriptors and goroutines return to baseline. One leaked
// fd or goroutine per cycle is invisible in any single test but fatal over a
// multi-year deployment; this is the regression net for that class of bug.
func TestRegisterRemoveCyclesNoResourceLeak(t *testing.T) {
	origDetect := detectAudioDevices
	t.Cleanup(func() { detectAudioDevices = origDetect })
	detectAudioDevices = func(string) ([]*audio.Device, error) {
		return []*audio.Device{{Name: "cycle_mic", CardNumber: 1, USBID: "0d8c:0014",
			VendorID: "0d8c", ProductID: "0014"}}, nil
	}

	scriptPath := filepath.Join(t.TempDir(), "mock_ffmpeg.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 60\n"), 0755); err != nil {
		t.Fatalf("write mock ffmpeg: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := config.DefaultConfig()
	cfg.Stream.USBStabilizationDelay = 0
	cfg.Stream.StopTimeout = 2 * time.Second
	flags := daemonFlags{LockDir: t.TempDir(), LogDir: t.TempDir()}

	sup := supervisor.New(supervisor.Config{ShutdownTimeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	supDone := make(chan error, 1)
	go func() { supDone <- sup.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-supDone:
		case <-time.After(10 * time.Second):
			t.Log("supervisor did not stop within 10s")
		}
	})

	var mu sync.RWMutex
	services := make(map[string]bool)
	hashes := make(map[string]string)
	cards := make(map[string]int)

	const devName = "cycle_mic"

	runOneCycle := func(cycle int) {
		t.Helper()
		if n := registerNewDevices(ctx, logger, cfg, flags, scriptPath, sup,
			&mu, services, hashes, cards); n != 1 {
			t.Fatalf("cycle %d: registered %d devices, want 1", cycle, n)
		}
		// Wait for the service to actually run (ffmpeg spawned) so every cycle
		// exercises the full resource path, not just map bookkeeping.
		deadline := time.Now().Add(10 * time.Second)
		running := false
		for time.Now().Before(deadline) {
			for _, st := range sup.Status() {
				if st.Name == devName && st.State == supervisor.ServiceStateRunning {
					running = true
				}
			}
			if running {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !running {
			t.Fatalf("cycle %d: service never reached running state", cycle)
		}
		// The same removal the stall detector / reload handler performs.
		if err := sup.Remove(devName); err != nil {
			t.Fatalf("cycle %d: Remove: %v", cycle, err)
		}
		mu.Lock()
		delete(services, devName)
		delete(hashes, devName)
		mu.Unlock()
	}

	// Warm-up cycle: let lazily-initialized runtime state (netpoll, exec pipes,
	// race-detector bookkeeping) come into existence before baselining, then
	// give the just-finished teardown (process reaping, pipe closes) a moment
	// to drain.
	runOneCycle(0)
	time.Sleep(300 * time.Millisecond)
	baselineFDs := countFDs(t)
	baselineGoroutines := runtime.NumGoroutine()

	const cycles = 12
	for i := 1; i <= cycles; i++ {
		runOneCycle(i)
	}

	// Everything per-cycle (lock fd, log-writer fd, stderr pipes, kill-timer
	// goroutines bounded by StopTimeout) must drain back to baseline.
	const fdSlack = 3
	const goroutineSlack = 4
	deadline := time.Now().Add(15 * time.Second)
	var fds, gr int
	for time.Now().Before(deadline) {
		fds = countFDs(t)
		gr = runtime.NumGoroutine()
		if fds <= baselineFDs+fdSlack && gr <= baselineGoroutines+goroutineSlack {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("after %d register/remove cycles: fds=%d (baseline %d +%d slack), goroutines=%d (baseline %d +%d slack): per-cycle resource leak",
		cycles, fds, baselineFDs, fdSlack, gr, baselineGoroutines, goroutineSlack)
}

// countFDs returns the number of open file descriptors of this process.
func countFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	return len(entries)
}
