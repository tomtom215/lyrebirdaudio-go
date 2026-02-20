# Production Readiness Assessment: LyreBirdAudio-Go

**For 24/7/365 Unattended Wildlife Bioacoustics Field Deployment**

**Date**: 2026-02-20
**Assessor**: Claude Opus 4.6 (automated code audit)
**Scope**: Systematic code audit with exact file:line evidence. Every finding verified against source code.

---

## Executive Summary

LyreBirdAudio-Go is a well-engineered system with strong test coverage (~87% internal average), thorough security hardening (18 systemd directives, least-privilege file permissions), and a mature supervision tree based on the battle-tested `thejerf/suture` library. Previous audit rounds (59 peer review issues, 3 Opus deep-audit bugs) have all been resolved.

However, for **unattended 24/7/365 field deployment** in remote wildlife bioacoustics stations, several gaps remain that distinguish "good server software" from "industrial field-grade reliability." The two CRITICAL findings represent real data-loss risks specific to the field deployment use case.

**Verdict**: Not yet production-ready for remote unattended field deployment without addressing C-1 and C-2. Ready for supervised deployments where operator intervention is available within minutes.

---

## Findings

### CRITICAL -- Audio Data Loss Risks

#### C-1: No Local Audio Recording (RTSP-Only Streaming)

**Verified**: `internal/stream/manager.go:636-697`

The `buildFFmpegCommand()` function constructs FFmpeg arguments that always output to a single destination: the RTSP URL (or `null` for testing). There is no tee muxer, no `-f segment` output, and no local file recording of any kind.

```go
// manager.go:686-691
if outputFormat != "" {
    args = append(args, "-f", outputFormat, cfg.RTSPURL)
} else {
    args = append(args, cfg.RTSPURL)
}
```

Codebase-wide search for `local.*record`, `.wav`, `.flac`, `file.*output`, `save.*audio`, `tee.*muxer` returns zero matches in non-test code.

**Impact**: If MediaMTX is down, restarting, crashed, or the RTSP connection drops, audio data is permanently lost. In a remote field deployment, any gap in MediaMTX availability means irreplaceable bioacoustics data is gone. A bird call at 3 AM during a MediaMTX restart is never recovered.

**Recommendation**: Add FFmpeg tee muxer output to both RTSP and local files with `-f segment` for time-based file rotation. This provides a local buffer that survives MediaMTX outages.

---

#### C-2: No FFmpeg RTSP Reconnection Flags

**Verified**: `internal/stream/manager.go:636-697`

FFmpeg is launched without `-reconnect`, `-reconnect_streamed`, `-reconnect_delay_max`, or `-reconnect_at_eof` flags. Codebase-wide search for `-reconnect` returns zero matches.

The full argument list built by `buildFFmpegCommand()` is: `-f alsa -i <device> -ar <rate> -ac <channels> [-thread_queue_size N] -c:a <codec> -b:a <bitrate> -f rtsp <url>`. No reconnection parameters.

**Impact**: If MediaMTX restarts (e.g., after an update or crash), FFmpeg immediately exits because the RTSP connection drops. The stream manager then enters exponential backoff starting at `InitialRestartDelay` (configurable, default 10s), doubling up to `MaxRestartDelay` (default 300s). This means up to 5+ minutes of silence after a simple MediaMTX restart, even though the server is back within seconds.

**Recommendation**: Add `-reconnect 1 -reconnect_streamed 1 -reconnect_delay_max 30` to FFmpeg RTSP output args. This allows FFmpeg to reconnect transparently without process restart.

---

### HIGH -- Reliability & Recovery Risks

#### H-1: FFmpeg Kill Timeout Hardcoded at 2 Seconds

**Verified**: `internal/stream/manager.go:586`

```go
killCtx, killCancel := context.WithTimeout(context.Background(), 2*time.Second)
```

When gracefully stopping FFmpeg (SIGINT), the force-kill timeout is only 2 seconds.

**Impact**: Audio codecs like Opus need time to flush their encoder buffer. 2 seconds may be insufficient if FFmpeg is writing buffered data, potentially losing the last few seconds of audio on every clean shutdown or stream restart. For a 24/7 deployment that periodically reconfigures, this accumulates.

**Recommendation**: Increase to 5-10 seconds. Make it configurable via `ManagerConfig`.

---

#### H-2: Stall Detection is Slow (15 Minutes Minimum)

**Verified**: `cmd/lyrebird-stream/main.go:454-464`

```go
checkInterval := cfg.Monitor.Interval  // line 454-456: defaults to 5 min
// ...
const maxStallChecks = 3               // line 464
```

Time to detect a stalled stream = `maxStallChecks` x `checkInterval` = 3 x 5 min = **15 minutes minimum**.

**Impact**: A USB microphone that silently disconnects (no error, just no data) goes undetected for 15 minutes. In wildlife bioacoustics, this could mean missing an entire dawn chorus.

**Recommendation**: Separate stall-check interval from general health interval. Stall checks should run every 30-60 seconds with a 2-minute detection threshold.

---

#### H-3: Health Endpoint Bind Failure is Silently Swallowed

**Verified**: `cmd/lyrebird-stream/main.go:400-404`

```go
go func() {
    if err := health.ListenAndServe(ctx, "127.0.0.1:9998", healthHandler); err != nil {
        logger.Warn("health endpoint error", "error", err)
    }
}()
```

The `health.ListenAndServe` function (`internal/health/health.go:91-118`) starts the HTTP server in a goroutine and blocks on `<-ctx.Done()`. If port 9998 is already in use, the inner goroutine sends the error to `errCh`, but that channel is only drained *after* the context is cancelled (line 118). So a bind failure is effectively invisible until shutdown.

The daemon logs a warning but continues without health monitoring. No external system knows the health endpoint is dead.

**Impact**: Any monitoring system (systemd, Prometheus, custom watchdog) relying on the health endpoint gets no data, but the daemon appears healthy from its own perspective.

**Recommendation**: Add a startup probe that verifies the health endpoint is actually listening before completing initialization. Consider using a `net.Listen()` call first with the listener passed to `http.Serve()`, so bind errors are detected synchronously.

---

#### H-4: No Persistent Failure Metrics

**Verified**: `internal/stream/manager.go:140-143`

```go
startTime time.Time
attempts  atomic.Int32
failures  atomic.Int32
```

All stream failure metrics are in-memory only. `cmd/lyrebird-stream/main.go:462-463` shows `prevBytes` and `stallCount` are also ephemeral maps.

**Impact**: After any daemon restart, all history is lost. You cannot answer "how many times did stream X fail last week?" or "when was the last stall event?". For a remote field deployment, post-hoc failure analysis is impossible.

**Recommendation**: Log structured failure events that can be parsed from journald, or write a simple state file to disk. At minimum, emit structured log lines at `Info` level for every failure/recovery event with machine-parseable fields.

---

#### H-5: `Acquire()` Uses Blocking `time.Sleep` Without Context

**Verified**: `internal/lock/filelock.go:124`

```go
time.Sleep(100 * time.Millisecond)
```

The non-context `Acquire()` method polls with `time.Sleep` in a tight loop with no way to be interrupted by shutdown.

**Status**: **MITIGATED** in the daemon path. The daemon uses `AcquireContext()` (`internal/stream/manager.go:456`), which correctly uses `ticker.C` + `ctx.Done()`. The `Acquire()` method remains available for future callers who may not know to use the context variant.

**Recommendation**: Deprecate `Acquire()` or make it delegate to `AcquireContext(context.Background(), timeout)`.

---

### MEDIUM -- Operational Gaps

#### M-1: Backup Timestamps Use Local Time, Not UTC

**Verified**: `internal/config/backup.go:84`

```go
timestamp := time.Now().Format(BackupTimestampFormat)
```

`time.Now()` returns local time, which depends on the system timezone. The format string `BackupTimestampFormat` ("2006-01-02T15-04-05") has no timezone indicator.

**Impact**: In a field deployment, if the device's timezone changes (NTP correction, manual change, DST), backup ordering becomes ambiguous. Restoring "the backup from before the config change at 14:00" is unreliable.

**Recommendation**: Use `time.Now().UTC()`.

---

#### M-2: `deviceConfigHash` Omits `InputFormat` and `OutputFormat`

**Verified**: `cmd/lyrebird-stream/main.go:685-694`

```go
func deviceConfigHash(devCfg config.DeviceConfig, rtspURL string) string {
    return fmt.Sprintf("%d/%d/%s/%s/%d/%s",
        devCfg.SampleRate,
        devCfg.Channels,
        devCfg.Bitrate,
        devCfg.Codec,
        devCfg.ThreadQueue,
        rtspURL,
    )
}
```

The hash includes `SampleRate`, `Channels`, `Bitrate`, `Codec`, `ThreadQueue`, and `rtspURL` but NOT `InputFormat` or `OutputFormat` from `ManagerConfig` (defined at `internal/stream/manager.go:79,87`).

**Impact**: Currently these fields aren't in `DeviceConfig` (verified: `internal/config/config.go:36-42` shows `DeviceConfig` has only `SampleRate`, `Channels`, `Bitrate`, `Codec`, `ThreadQueue`), so this is theoretical. But `InputFormat` and `OutputFormat` are set on `ManagerConfig` and if ever made configurable per-device, a SIGHUP reload wouldn't trigger stream restart.

**Recommendation**: Document that `InputFormat`/`OutputFormat` are not hot-reloadable, or add them to `DeviceConfig` and the hash when they become configurable.

---

#### M-3: Log Rotation Silently Discards Errors

**Verified**: `internal/stream/logrotate.go:132`

```go
_ = w.rotate()
```

Rotation errors are explicitly ignored. Additionally, `compressFile()` (lines 274-276, 282-284) silently returns on any error (file read failure, gzip create failure, write failure).

Codebase-wide search for `ENOSPC`, `disk.*full`, `no space` returns zero matches in non-doc code.

**Impact**: If the disk is full (ENOSPC), log rotation fails silently. FFmpeg continues writing to the current file, potentially filling the disk completely and causing broader system failures. No disk space monitoring exists.

**Recommendation**: At minimum, log rotation failures should be logged. Ideally, implement disk space monitoring and emit warnings when free space drops below a threshold.

---

#### M-4: `supervisor.Remove()` Doesn't Wait for Service Stop

**Verified**: `internal/supervisor/supervisor.go:316-340`

```go
func (s *Supervisor) Remove(name string) error {
    // ...
    if err := s.suture.Remove(entry.token); err != nil { ... }
    if entry.wrapper != nil {
        entry.wrapper.Stop()  // Cancels context
    }
    delete(s.services, name)  // Immediate map deletion
    // ...
}
```

`Remove()` calls `suture.Remove()` and `entry.wrapper.Stop()` (which cancels the context), then immediately deletes from the map. It does not wait for the service's `Serve()` goroutine to return.

**Impact**: When the daemon restarts a stalled stream (stall detection or SIGHUP reload in `main.go`), it removes the old service and re-registers. If the old FFmpeg process hasn't fully exited yet, the new manager may encounter the old process's lock file or a port conflict. The `streamService.Run()` method does call `manager.Close()` on exit (`main.go:571`), but the timing is racy.

**Recommendation**: Add synchronization so `Remove()` blocks until the service's `Serve()` returns, or add a configurable delay before re-registration.

---

#### M-5: Compression Goroutine Not Context-Bounded

**Verified**: `internal/stream/logrotate.go:206-213`

```go
w.wg.Add(1)
go func() {
    defer w.wg.Done()
    w.compressFile(rotatedPath)
    w.cleanup()
}()
```

Compression runs in a goroutine with no context. However, `Close()` (line 149-165) calls `w.wg.Wait()`, so goroutines are properly joined on shutdown.

**Status**: **LOW RISK**. The `wg.Wait()` prevents leaks. But during shutdown, compression of a large log file could delay process exit. With the default 10MB max log size, gzip compression completes quickly, so the practical impact is minimal.

---

### LOW -- Nice-to-Have Improvements

#### L-1: No Prometheus Metrics Endpoint

Codebase search for `prometheus`, `metrics`, `/metrics` returns matches only in documentation (existing audit reports noting the gap). No actual Prometheus instrumentation code exists.

**Impact**: Standard monitoring stack (Prometheus + Grafana) cannot scrape this service. For field deployments with centralized monitoring, this is a gap.

---

#### L-2: No Diagnostic Bundle Export

No `diagnose --bundle` or `diagnose --export` command exists. Search for `bundle` and `export` in the CLI code returns no matches.

**Impact**: Remote troubleshooting requires SSH and manual log gathering.

---

#### L-3: No Field Technician Runbook

No `docs/RUNBOOK.md` or similar operational guide exists.

**Impact**: Non-developer field operators have no reference for common issues (device not detected, stream stalled, disk full, etc.).

---

#### L-4: Low Daemon Test Coverage

`cmd/lyrebird-stream` at ~32.7% coverage (per CLAUDE.md dashboard). Core daemon goroutine code (recovery, health checks, stall detection) is largely untested.

**Impact**: Confidence in daemon-level behavior relies on integration testing and manual verification rather than unit tests.

---

## Summary Table

| ID | Severity | Finding | File:Line | Status |
|----|----------|---------|-----------|--------|
| C-1 | **CRITICAL** | No local audio recording | `manager.go:636-697` | **FIXED** - Tee muxer with local segment recording |
| C-2 | **CRITICAL** | No FFmpeg reconnect flags | `manager.go:636-697` | **FIXED** - `-reconnect` flags for RTSP output |
| H-1 | HIGH | 2s FFmpeg kill timeout | `manager.go:586` | **FIXED** - Configurable `StopTimeout` (default 5s) |
| H-2 | HIGH | 15-min stall detection | `main.go:454-464` | **FIXED** - Separate `StallCheckInterval` (default 60s) |
| H-3 | HIGH | Health bind failure silent | `main.go:400-404` | **FIXED** - Synchronous bind via `ListenAndServeReady` |
| H-4 | HIGH | No persistent metrics | `manager.go:140-143` | **FIXED** - Structured `stream_event` log entries |
| H-5 | HIGH | `Acquire()` blocking sleep | `filelock.go:124` | **FIXED** - Delegated to `AcquireContext` |
| M-1 | MEDIUM | Backup uses local time | `backup.go:84` | **FIXED** - `time.Now().UTC()` |
| M-2 | MEDIUM | Config hash incomplete | `main.go:685-694` | **FIXED** - Hash includes stream config fields |
| M-3 | MEDIUM | Log rotation silent errors | `logrotate.go:132` | **FIXED** - `WithRotateLogger` option, errors logged |
| M-4 | MEDIUM | `Remove()` doesn't wait | `supervisor.go:316-340` | **FIXED** - `done` channel with 10s timeout |
| M-5 | MEDIUM | Compression no context | `logrotate.go:206-213` | LOW RISK |
| L-1 | LOW | No Prometheus metrics | -- | OPEN |
| L-2 | LOW | No diagnostic bundle | -- | OPEN |
| L-3 | LOW | No runbook | -- | OPEN |
| L-4 | LOW | Low daemon coverage | -- | OPEN |

---

## Prioritized Remediation Roadmap

### Phase 1: Data Safety (address before any field deployment)
1. **C-1**: Add FFmpeg tee muxer with local segment recording
2. **C-2**: Add FFmpeg RTSP reconnection flags

### Phase 2: Reliability Hardening (address before unattended deployment)
3. **H-2**: Reduce stall detection to 30-60s intervals
4. **H-1**: Increase FFmpeg kill timeout to 5-10s, make configurable
5. **H-3**: Add health endpoint startup probe
6. **H-4**: Emit structured failure/recovery log events

### Phase 3: Operational Polish
7. **M-3**: Log rotation error reporting
8. **M-1**: UTC timestamps for backups
9. **M-4**: Synchronous `Remove()` with wait-for-stop
10. **L-3**: Create field technician runbook

### Phase 4: Monitoring & Observability
11. **L-1**: Add Prometheus metrics endpoint
12. **L-2**: Add diagnostic bundle export
13. **L-4**: Increase daemon test coverage

---

## Positive Observations

The following aspects are notable strengths for production deployment:

- **Supervision tree**: Erlang-style supervision via `suture/v4` with configurable backoff, graceful shutdown, and dynamic service registration. Well-tested at 96.4% coverage.
- **Signal handling**: SIGINT/SIGTERM for graceful shutdown, SIGHUP for hot-reload with config hash comparison to restart only affected streams.
- **Security hardening**: 18 systemd directives, least-privilege file permissions throughout, localhost-only network bindings.
- **Stale lock detection**: flock(2)-based locking with PID-based staleness detection that correctly handles long-running processes (C-1 fix in lock code).
- **USB hot-plug support**: Periodic device polling (10s) with stabilization delay for newly plugged devices.
- **Periodic recovery**: Failed streams are automatically cleared and re-registered when the USB device is still present.
- **Test coverage**: ~87% internal package average with table-driven tests, race detector clean.
- **Audit trail**: Three previous audit rounds with all issues resolved and documented.
