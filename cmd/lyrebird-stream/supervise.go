// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

// superviseRestartDelay is the pause between a recovered panic and restarting a
// background task, so a task that panics immediately on every run cannot spin
// the CPU. It is a var (not a const) only so tests can shorten it.
var superviseRestartDelay = 2 * time.Second

// runSupervised runs fn and keeps it alive for the life of ctx.
//
// The daemon's background loops (device poller, SIGHUP reload handler, stall
// detector, failed-stream recovery, segment retention, disk-space monitor) are
// long-lived goroutines. Without protection, an unexpected panic in any of them
// (a nil deref on malformed config, an unforeseen edge case in stall handling)
// would crash the ENTIRE process, dropping every audio stream at once — a
// disproportionate blast radius for a system whose whole premise is autonomous
// per-device recovery over months of unattended operation.
//
// runSupervised recovers such a panic, logs it with a stack trace (visible in
// journald), and restarts fn after superviseRestartDelay, so the fault is
// contained to the one subsystem, which self-heals, while the audio streams and
// the other loops keep running. fn is expected to block until ctx is cancelled;
// when it returns with ctx already done, runSupervised exits without restarting.
// If fn returns WITHOUT ctx being cancelled (only possible via a recovered
// panic here, since every wrapped loop returns solely on ctx.Done()), it is
// restarted after the delay.
func runSupervised(ctx context.Context, logger *slog.Logger, name string, fn func()) {
	for {
		if ctx.Err() != nil {
			return
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					if logger != nil {
						logger.Error("background task panicked; restarting after delay",
							"task", name,
							"panic", fmt.Sprintf("%v", r),
							"stack", string(debug.Stack()))
					}
				}
			}()
			fn()
		}()

		// fn returned: normally because ctx was cancelled (clean shutdown), or
		// because a panic was recovered above. Stop on shutdown; otherwise pause
		// briefly (so a tight panic loop cannot spin) and restart.
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(superviseRestartDelay):
		}
	}
}
