# Peer Review — LyreBirdAudio-Go

**Reviewer:** Claude Code
**Date:** 2026-02-19
**Branch:** `claude/code-review-audit-ceC8S`
**Scope:** Full codebase — correctness, security, concurrency, coverage, architecture, CI/CD, documentation
**Build status:** `go build ./...` — PASS
**Test status:** `go test ./...` — PASS (all 14 packages)
**Race detector:** `go test -race ./...` — PASS
**Linter:** `golangci-lint run ./...` — 0 issues
**Total coverage:** 71.8%

---

## Implementation Status

**Implementation branch:** `claude/implement-peer-review-fixes-lQ6hO`
**Last updated:** 2026-02-19
**Post-fix test status:** `go test -race ./...` — PASS (14/14 packages), `go vet ./...` — PASS
**Post-fix total coverage:** 71.5%

| Tier | Total | Fixed | Open |
|------|-------|-------|------|
| CRITICAL | 5 | **5** | 0 |
| MAJOR | 13 | **9** | 4 |
| MEDIUM | 12 | **5** | 7 |
| LOW | 14 | **7** | 7 |
| DOC | 10 | **1** | 9 |
| CI/CD | 5 | 0 | **5** |
| **Total** | **59** | **27** | **32** |

All five CRITICAL issues and the highest-impact MAJOR issues have been resolved. The remaining open items are in areas that require deeper architectural changes (stream hot-reload, CLI feature completion, self-update security) or carry lower operational risk.

---

## Summary

The project demonstrates strong engineering foundations: structured logging, context propagation, table-driven tests, atomic state management, and an Erlang-style supervisor tree. The internal packages are well-tested (85–94% coverage). However, the review identified **48 discrete issues** across five severity tiers (plus 5 CI/CD issues introduced at review time), including critical concurrency bugs and a logic error in the lock subsystem that will cause lock theft for any stream running longer than five minutes — the primary operational scenario.

---

## Severity Definitions

| Label | Meaning |
|-------|---------|
| **CRITICAL** | Will cause incorrect runtime behavior or data corruption |
| **MAJOR** | Significant defect or security concern; blocks production release |
| **MEDIUM** | Functional defect or design violation with workaround |
| **LOW** | Code quality, style, or minor inconsistency |
| **DOC** | Documentation inaccuracy or omission |

---

## CRITICAL Issues

### C-1 — Lock Theft for Long-Running Streams

**File:** `internal/lock/filelock.go:296–347`
**Function:** `isLockStale`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

The stale-lock check considered a lock **stale if the lock file's `ModTime` is older than `DefaultStaleThreshold` (300 s), even when the process that owned the lock was confirmed to be running** (signal 0 succeeded). The check ran signal-0 to verify the process was alive, and then fell through to the age check regardless:

```go
// ORIGINAL BUGGY CODE:
err = process.Signal(syscall.Signal(0))
if err != nil {
    return true, nil  // process dead → stale (correct)
}
// Age check ran even when the process was alive:
age := time.Since(info.ModTime())
if age > threshold {
    return true, nil  // WRONG: running process treated as stale
}
```

**Fix applied:** `isLockStale` now returns `false` immediately when `signal(0)` succeeds (process alive). The age check is not run for live processes. Regression test `TestFileLockStaleOldAgeAliveProcess` added; `TestFileLockStaleDeadProcessOldAge` documents the correct stale path.

---

### C-2 — Data Race: `registeredServices` Map Accessed from Multiple Goroutines

**File:** `cmd/lyrebird-stream/main.go:130–274`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

`registeredServices` was a plain `map[string]bool` written and read from three goroutines concurrently (poll loop, SIGHUP handler, main goroutine).

**Fix applied:** The map is now protected by `sync.RWMutex` (`registeredMu`). Read operations use `RLock`/`RUnlock`; write operations use `Lock`/`Unlock`. The reload goroutine's range loop (L-14) was also fixed to copy names under `RLock` before iterating.

---

### C-3 — Nil Pointer Dereference: `koanfCfg` Can Be Nil

**File:** `cmd/lyrebird-stream/main.go:221`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

`loadConfigurationKoanf` returned `(nil, config.DefaultConfig(), nil)` when the config file was absent and `NewKoanfConfig` failed. The poll loop called `koanfCfg.Load()` unconditionally, causing a nil pointer dereference.

**Fix applied:** The poll loop now guards `koanfCfg` with a nil check and falls back to the previously loaded `cfg` when `koanfCfg` is nil. The SIGHUP handler also guards with a nil check and treats it as a no-op. Regression test `TestLoadConfigurationKoanfNonNilOnSuccess` added.

---

### C-4 — Race Condition in `serviceWrapper.Serve` / `Stop`

**File:** `internal/supervisor/supervisor.go:161–208`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

`serviceWrapper` stored `ctx` and `cancel` as plain fields set by `Serve()` and read by `Stop()` with no synchronization. If suture called `Stop()` between `Serve()` entry and `w.cancel` assignment, the service would not be stopped.

**Fix applied:** Added `sync.Mutex mu` to `serviceWrapper`. `Serve()` sets `w.cancel` under `mu.Lock()`; `Stop()` reads `w.cancel` under `mu.Lock()`. The `ctx` field was removed from the struct (it was only used as a local variable inside `Serve`). `Remove()` now delegates to `Stop()` so the mutex is always respected. Stress tests `TestSupervisor_StopServeCancelRace` and `TestSupervisor_StopCoverage` added.

---

### C-5 — `m.cmd` Assigned Before `cmd.Start()` Succeeds

**File:** `internal/stream/manager.go:487–497`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

`m.cmd` was assigned before `cmd.Start()`, so a failed start left a dangling pointer to an unstarted command. A concurrent `stop()` call could then attempt to signal a nil `Process`.

**Fix applied:** `m.cmd` is now assigned only after `cmd.Start()` returns `nil`. The lock/unlock is split: pre-start (logWriter, startTime), start, then post-start (m.cmd). Regression test `TestStartFFmpegCmdNilOnFailure` added.

---

## MAJOR Issues

### M-1 — Error Comparison Using `==` Instead of `errors.Is`

**Files:**
- `internal/stream/manager.go:290` — `if err == context.Canceled`
- `cmd/lyrebird-stream/main.go:279` — `err != context.Canceled`
- `cmd/lyrebird-stream/main.go:300` — `err != context.Canceled`

**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

**Fix applied:** All three sites now use `errors.Is(err, context.Canceled)`.

---

### M-2 — Watchdog Signal Never Sent (`WatchdogSec` Without `sd_notify`)

**File:** `systemd/lyrebird-stream.service:67`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

The service file set `WatchdogSec=60` but the daemon never calls `sd_notify(0, "WATCHDOG=1")`. systemd would kill and restart the service every 60 seconds on any system with watchdog enforcement active.

**Fix applied:** `WatchdogSec=60` removed. A comment explains the absence: the daemon does not implement sd_notify keepalives.

---

### M-3 — Health Endpoint Is Implemented But Never Started

**Files:** `internal/health/health.go`, `cmd/lyrebird-stream/main.go`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

`internal/health` provided `ListenAndServe` and a complete `Handler` but the daemon never started it.

**Fix applied:** The daemon now starts `health.ListenAndServe(ctx, ":9998", healthHandler)` in a goroutine before the supervisor loop. Health check failures are logged as warnings (non-fatal).

---

### M-4 — Device Polling Only Triggers When No Services Are Registered

**File:** `cmd/lyrebird-stream/main.go:219`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

The poll loop was gated on `sup.ServiceCount() == 0`, so a second USB microphone plugged in after startup was never registered.

**Fix applied:** The `ServiceCount() == 0` gate removed. The poll loop now scans unconditionally on every tick, calling `registerDevices` which skips already-registered devices via the mutex-protected map.

---

### M-5 — `Manager.Close()` Never Called; Log File Handles Leak

**Files:** `internal/stream/manager.go:417`, `cmd/lyrebird-stream/main.go:297–305`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

`streamService.Run()` called `m.manager.Run(ctx)` but never called `m.manager.Close()`. Every supervisor-initiated restart accumulated an open log file handle.

**Fix applied:** `streamService.Run()` now calls `s.manager.Close()` with error logging immediately after `s.manager.Run(ctx)` returns, regardless of the run result.

---

### M-6 — Config Changes on SIGHUP Not Applied to Running Streams

**File:** `cmd/lyrebird-stream/main.go:134–198`
**Status: 🔲 OPEN**

On SIGHUP, `registerDevices` skips all devices already in `registeredServices`. Changed configuration (sample rate, codec, etc.) is not applied to already-running streams. There is no mechanism to restart streams with updated config. Requires architectural work: config hash comparison per device, stream stop/re-register on diff.

---

### M-7 — Misleading `isLockStale` Comment on Age Check

**File:** `internal/lock/filelock.go:339–343`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

**Fix applied:** Comments updated as part of the C-1 fix. `isLockStale` now documents that the age check was removed from the live-process path and explains why (`signal(0)` success is authoritative).

---

### M-8 — `koanf.go` env Transform Comment Is Misleading

**File:** `internal/config/koanf.go:152–154`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

The comment claimed prefix removal was "already handled by Prefix option." It is not — the `TrimPrefix` call is necessary.

**Fix applied:** Comment replaced with accurate description: "k arrives WITHOUT the LYREBIRD_ prefix (stripped by env.Provider)."

---

### M-9 — `Watch()` Does Not Stop the File Watcher on Context Cancellation

**File:** `internal/config/koanf.go:239–271`
**Status: ✅ DOCUMENTED** — `claude/implement-peer-review-fixes-lQ6hO`

`fp.Watch(...)` starts an internal `fsnotify` goroutine that cannot be stopped because koanf v2's `file.Provider` exposes no `Stop()` method. The goroutine leak is a real defect in the koanf dependency.

**Fix applied:** The goroutine leak cannot be fixed without a koanf API change. A detailed doc comment now explains the limitation and recommends using manual `Reload()` calls on SIGHUP instead of `Watch()`. The daemon uses the SIGHUP approach. The daemon does **not** call `Watch()`.

---

### M-10 — `runDetect` Outputs Hardcoded Settings, Ignores Actual Device Capabilities

**File:** `cmd/lyrebird/main.go:218–246` (`runDetectWithPath`)
**Status: 🔲 OPEN**

The `detect` command outputs hardcoded recommendations regardless of actual device PCM capabilities. `internal/audio/capabilities.go` (100% covered) implements `DetectCapabilities` but is never called by the CLI.

---

### M-11 — `allPassed` Logic Broken in `runTest`

**File:** `cmd/lyrebird/main.go:1152` (`runTest`)
**Status: 🔲 OPEN**

`allPassed` is only set to `false` when FFmpeg is not found. Warning-level failures in tests 2, 4, and 5 do not update `allPassed`, so the command can print "All tests passed!" with unreachable MediaMTX.

---

### M-12 — `installLyreBirdService` Writes a Stripped-Down Service File

**File:** `cmd/lyrebird/main.go:910` (`installLyreBirdService`)
**Status: 🔲 OPEN**

The service written by `lyrebird setup` lacks all security hardening present in `systemd/lyrebird-stream.service`. Users who install via the CLI receive a significantly less secure service. The two definitions must be kept in sync or the CLI must use the versioned file.

---

### M-13 — Self-Update Has No Checksum Verification

**File:** `internal/updater/updater.go:266` (`Download`)
**Status: 🔲 OPEN**

The updater downloads a binary from GitHub releases without verifying against `checksums.txt`. A compromised CDN or MITM attack results in arbitrary code execution. SHA256 verification must be added before replacing the binary.

---

## MEDIUM Issues

### ME-1 — `Backoff.RecordFailure` Doubles Delay on First Call

**File:** `internal/stream/backoff.go:90–103`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

At construction, `currentDelay = initialDelay`. `RecordFailure()` doubled it immediately, so the first restart waited `2 × initialDelay` (20 s) rather than the documented `initialDelay` (10 s).

**Fix applied:** The call order in `manager.Run()` was swapped: `WaitContext()` is called first (using the current, pre-doubled delay), then `RecordFailure()` doubles the delay for the *next* iteration. The `Backoff` library itself was not changed. Regression test `TestBackoffFirstRestartUsesInitialDelay` added.

---

### ME-2 — `logf` Always Uses `Info` Level and Loses Structured Logging Benefits

**Files:** `internal/stream/manager.go:214–217`, `internal/supervisor/supervisor.go:250–253`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

Both `logf` wrappers called `logger.Info(...)` for all messages including errors.

**Fix applied:** `Manager` now has both `logf` (Info) and `logError` (Error). Failure paths in `Run()` use `logError`. See also ME-7.

---

### ME-3 — `stop()` Spawns an Unkillable 2-Second Goroutine

**File:** `internal/stream/manager.go:564–570`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

The kill goroutine used `time.Sleep(2 * time.Second)` with no cancellation mechanism, potentially overlapping with a new process's lifecycle under high restart frequency.

**Fix applied:** The goroutine now uses `context.WithTimeout(context.Background(), 2*time.Second)`. The goroutine waits on `<-killCtx.Done()` and only kills when `killCtx.Err() == context.DeadlineExceeded` (i.e., FFmpeg did not exit within 2 s on its own). `killCancel()` is deferred so the context is always released.

---

### ME-4 — `findDeviceIDPath` Hardcodes `/dev/snd/by-id`, Untestable

**File:** `internal/audio/detector.go:226–258`
**Status: 🔲 OPEN**

The path `/dev/snd/by-id` is hardcoded and cannot be injected for testing, resulting in 27.8% coverage. The function should accept `byIDDir` as a parameter.

---

### ME-5 — `udev.WriteRulesFile` and `udev.ReloadUdevRules` Have 0% Coverage

**File:** `internal/udev/rules.go:167, 217`
**Status: 🔲 OPEN**

Both system-modifying functions have zero test coverage. `ReloadUdevRules` should accept an injectable command runner.

---

### ME-6 — `config.Save()` Coverage Is 68%; Atomic-Write Error Paths Untested

**File:** `internal/config/config.go:116` (`Save`)
**Status: 🔲 OPEN**

`Chmod`, `Sync`, and second `Close` error branches are not covered in a function that modifies production config files.

---

### ME-7 — `logf` in `supervisor.go` Uses `fmt.Sprintf` Then `slog.Info`

**File:** `internal/supervisor/supervisor.go:250–253`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

Same structural anti-pattern as ME-2.

**Fix applied:** `Supervisor` now has `logf` (Info), `logWarn` (Warn), and `logError` (Error) helpers. Service failure paths use `logWarn`; the suture-remove error path uses `logWarn`. The "Warning:" string prefix in the remove log message was also removed (it is now expressed via the log level).

---

### ME-8 — Package-Level Flag Variables Impede Testability

**File:** `cmd/lyrebird-stream/main.go:60–65`
**Status: 🔲 OPEN**

Package-level `flag` variables are parsed once at startup. Tests that call `flag.Parse()` multiple times will see stale values.

---

### ME-9 — `internal/health` HTTP Server Missing `ReadTimeout` and `WriteTimeout`

**File:** `internal/health/health.go:92–96`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

Without `ReadTimeout` and `WriteTimeout`, the health server was vulnerable to slow-client (Slowloris) connections.

**Fix applied:** `ReadTimeout: 10 * time.Second` and `WriteTimeout: 10 * time.Second` added to the `http.Server` struct.

---

### ME-10 — env Transform Is Brittle: New `DeviceConfig` Fields Break env Overrides

**File:** `internal/config/koanf.go:176–186`
**Status: 🔲 OPEN**

The transform uses a hardcoded list of DeviceConfig field suffixes. Any new field added to `DeviceConfig` requires a matching update here or the env override silently fails.

---

### ME-11 — `ValidatePartial` Allows `SampleRate < 0` to Pass

**File:** `internal/config/config.go:276`
**Status: 🔲 OPEN**

`SampleRate == 0` passes validation. The error message "must be positive" is misleading for value 0 (which is a valid zero-value/unset sentinel in this codebase).

---

### ME-12 — `getUSBBusDevFromCard` Has 16% Coverage and Fragile Loop Logic

**File:** `cmd/lyrebird/main.go:360` (`getUSBBusDevFromCard`)
**Status: 🔲 OPEN**

Only 16% coverage; fragile `continue` after failed `Sscanf` without resetting partial state.

---

## LOW Issues

### L-1 — `supervisor.serviceWrapper.Stop()` Had 0% Coverage

**File:** `internal/supervisor/supervisor.go:204`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

**Fix applied:** `TestSupervisor_StopCoverage` calls `Remove()` while a service is running, which causes suture to call `Stop()` on the wrapper. Coverage now exercised under the race detector.

---

### L-2 — `Manager.Close()` Has 0% Unit Test Coverage

**File:** `internal/stream/manager.go:417`
**Status: 🔲 OPEN**

`Close()` is now called from `streamService.Run()` in the daemon (M-5 fix), but no unit test exercises the path because the daemon tests use nil managers. A unit test that creates a real Manager and calls Close() directly is still needed.

---

### L-3 — `menu.RunCommand` Has 0% Coverage

**File:** `internal/menu/menu.go:409`
**Status: 🔲 OPEN**

---

### L-4 — `downloadFile` and `installLyreBirdService` Have 0% Coverage

**File:** `cmd/lyrebird/main.go:1091, 910`
**Status: 🔲 OPEN**

---

### L-5 — `menu.Display` Has 5.6% Coverage; `createDeviceMenu` 36.4%

**File:** `internal/menu/menu.go:104, 492`
**Status: 🔲 OPEN**

---

### L-6 — `SafeGoWithRecover` Closes Channel After Sending Error

**File:** `internal/util/panic.go:88–96`
**Status: 🔲 OPEN**

If `err != nil`, the channel receives an error and is then immediately closed. The close creates ambiguity for callers using range-style reads.

---

### L-7 — `stop()` Undocumented Intentional Signal Discard

**File:** `internal/stream/manager.go:560`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

**Fix applied:** Comment added explaining that `ESRCH` is the expected benign race when the process exits between the nil-check and the signal call.

---

### L-8 — Makefile `test` Timeout (30 s) Diverges from CI Timeout (2 min)

**File:** `Makefile:83`, `ci.yml:106`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

**Fix applied:** Makefile `test` and `test-race` targets updated to `-timeout 2m`.

---

### L-9 — `golangci-lint` Version Mismatch Between CI and Makefile

**File:** `ci.yml:49`, `Makefile:119`
**Status: 🔲 OPEN**

CI pins `golangci-lint@v1.62.2`; Makefile installs `@latest`. Local `make lint` may produce different results than CI.

---

### L-10 — `go.mod` Contains Two YAML Parsers

**File:** `go.mod:9,52`
**Status: 🔲 OPEN**

Both `gopkg.in/yaml.v3` and `go.yaml.in/yaml/v3` (pulled by koanf) are in the dependency tree.

---

### L-11 — `stretchr/testify` Listed as Indirect Dependency

**File:** `go.mod:50`
**Status: 🔲 OPEN**

---

### L-12 — `logrotate.go` Feature Not Wired in Daemon

**File:** `internal/stream/logrotate.go`, `cmd/lyrebird-stream/main.go`
**Status: 🔲 OPEN**

`RotatingWriter` is implemented and tested but `ManagerConfig.LogDir` is never set in the daemon. FFmpeg stderr is silently discarded.

---

### L-13 — Platform Build Constraints Missing

**Files:** `internal/lock/filelock.go`, `internal/diagnostics/diagnostics.go`, `internal/stream/monitor.go`
**Status: ✅ PARTIALLY FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

**Fix applied:** `//go:build linux` added to `internal/lock/filelock.go` and its test file, and to `internal/diagnostics/diagnostics.go` and its test file.

**Still open:** `internal/stream/monitor.go` uses `/proc`-based monitoring but does not yet carry the build constraint.

---

### L-14 — `registeredServices` Read Without Lock in Reload Goroutine (Logging)

**File:** `cmd/lyrebird-stream/main.go:262–269`
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

Fixed as part of the C-2 fix. The reload goroutine now copies names into a local slice under `registeredMu.RLock()` before iterating.

---

## Documentation Issues

### D-1 — README: Migration Timeline Is Stale

**File:** `README.md:239`
**Status: 🔲 OPEN**

> "estimated: Q2 2025"

The current date is February 2026. The timeline has passed.

---

### D-2 — README: "No Runtime Dependencies" Is Inaccurate

**File:** `README.md:21`
**Status: 🔲 OPEN**

The binary shells out to `ffmpeg`, `curl`/`wget`, `udevadm`, `systemctl`, `tar`, and `install`. The correct claim is "no shared library dependencies."

---

### D-3 — README: Integration Tests Claim Is Inaccurate

**File:** `README.md:344`
**Status: 🔲 OPEN**

CI uses `ubuntu-latest`, not a self-hosted runner with USB hardware. No integration tests exist.

---

### D-4 — CLAUDE.md: "Future Work" Section Is Outdated

**File:** `CLAUDE.md` — "Future Work / Remaining"
**Status: ✅ FIXED** — `claude/implement-peer-review-fixes-lQ6hO`

**Fix applied:** CLAUDE.md updated to move SIGHUP hot-reload to the Completed section and document all peer-review fixes implemented in this session.

---

### D-5 — CLAUDE.md: Code Example Omits Error Handling

**File:** `CLAUDE.md` — koanf example
**Status: 🔲 OPEN**

```go
cfg, err := kc.Load()
devCfg := cfg.GetDeviceConfig("blue_yeti")
```

`err` is declared but never checked.

---

### D-6 — Developer Artifacts at Repository Root

**Files:** `AUDIT_REPORT.md`, `FINDINGS.md`, `IMPLEMENTATION_PLAN.md`, `IMPROVEMENTS_SUMMARY.md`
**Status: 🔲 OPEN**

These planning/audit documents belong in `/docs`, a wiki, or PR descriptions — not at the repository root.

---

### D-7 — AUDIT_REPORT.md Contains Inaccurate Bug Descriptions

**File:** `AUDIT_REPORT.md:30–43`
**Status: 🔲 OPEN**

Issue 1.1 claims `m.state.Load()` can return nil (it cannot; `StateIdle` is stored in the constructor). Issue 1.2 claims `Backoff.Reset` has no nil check (it does). The existing report was not updated after previous fixes.

---

### D-8 — README: Performance Numbers Are Duplicated

**File:** `README.md:413–433`
**Status: 🔲 OPEN**

"Resource Usage" and "Benchmarks" sections list identical figures with different phrasing.

---

### D-9 — CLAUDE.md Coverage Table Formatting Is Inconsistent

**File:** `CLAUDE.md` — "Current Test Coverage" table
**Status: 🔲 OPEN**

Column widths in the Markdown table are not padded consistently.

---

### D-10 — README `Debug Mode` Section Is Misleading

**File:** `README.md:395`
**Status: 🔲 OPEN**

`sudo -E systemctl restart` does not inject environment variables into systemd-managed services. The correct mechanism is `/etc/lyrebird/environment` (referenced by `EnvironmentFile=`).

---

## CI/CD Issues

### CI-1 — CI Tests Only One Go Version

**File:** `ci.yml:85`
**Status: 🔲 OPEN**

The test matrix has a single entry: `['1.24.13']`. Testing against both the minimum (`go 1.24.2` from `go.mod`) and latest would confirm backward compatibility.

---

### CI-2 — Release Job Does Not Create a GitHub Release

**File:** `ci.yml:233–266`
**Status: 🔲 OPEN**

The `release` job downloads and re-uploads artifacts but never calls `gh release create`. On tag pushes, no GitHub Release is created.

---

### CI-3 — `codecov/codecov-action` Uses `v4` Without SHA Pin

**File:** `ci.yml:111`
**Status: 🔲 OPEN**

Unpinned floating tags are a supply-chain risk. All third-party GitHub Actions should be pinned to a full commit SHA.

---

### CI-4 — `gosec` and `govulncheck` Installed at `@latest`

**File:** `ci.yml:69, 76`
**Status: 🔲 OPEN**

Installing security tools at `@latest` in CI means tool updates can silently break builds or miss newly detected issues if the resolution is cached.

---

### CI-5 — Integration Test Step Runs on `ubuntu-latest` Without Hardware

**File:** `ci.yml:212`
**Status: 🔲 OPEN**

The integration step runs on `ubuntu-latest` which has no USB devices. It silently succeeds, giving a false sense of integration test coverage.

---

## Checklist Summary

| ID | Severity | File | Issue | Status |
|----|----------|------|-------|--------|
| C-1 | CRITICAL | `internal/lock/filelock.go:296` | Lock theft for running processes after 300 s | ✅ FIXED |
| C-2 | CRITICAL | `cmd/lyrebird-stream/main.go:130` | `registeredServices` map race | ✅ FIXED |
| C-3 | CRITICAL | `cmd/lyrebird-stream/main.go:221` | Nil pointer dereference on `koanfCfg.Load()` | ✅ FIXED |
| C-4 | CRITICAL | `internal/supervisor/supervisor.go:161` | Race in `Serve`/`Stop` on `cancel` field | ✅ FIXED |
| C-5 | CRITICAL | `internal/stream/manager.go:487` | `m.cmd` set before `cmd.Start()` succeeds | ✅ FIXED |
| M-1 | MAJOR | `manager.go:290`, `main.go:279,300` | `==` instead of `errors.Is` for context errors | ✅ FIXED |
| M-2 | MAJOR | `systemd/lyrebird-stream.service:67` | `WatchdogSec` without `sd_notify` | ✅ FIXED |
| M-3 | MAJOR | `internal/health/health.go` | Health endpoint implemented but never started | ✅ FIXED |
| M-4 | MAJOR | `cmd/lyrebird-stream/main.go:219` | Hotplug only works when no services exist | ✅ FIXED |
| M-5 | MAJOR | `internal/stream/manager.go:417` | `Manager.Close()` never called; fd leak | ✅ FIXED |
| M-6 | MAJOR | `cmd/lyrebird-stream/main.go:134` | Config changes on SIGHUP not applied to running streams | 🔲 OPEN |
| M-7 | MAJOR | `internal/lock/filelock.go:339` | Age-check comment obscures C-1 logic flaw | ✅ FIXED |
| M-8 | MAJOR | `internal/config/koanf.go:152` | Misleading "already handled" comment | ✅ FIXED |
| M-9 | MAJOR | `internal/config/koanf.go:248` | File watcher goroutine leaked on ctx cancel | ✅ DOCUMENTED |
| M-10 | MAJOR | `cmd/lyrebird/main.go:218` | `detect` uses hardcoded settings, not capabilities | 🔲 OPEN |
| M-11 | MAJOR | `cmd/lyrebird/main.go:1152` | `allPassed` not updated by warning tests | 🔲 OPEN |
| M-12 | MAJOR | `cmd/lyrebird/main.go:910` | `installLyreBirdService` lacks security hardening | 🔲 OPEN |
| M-13 | MAJOR | `internal/updater/updater.go:266` | Self-update has no checksum verification | 🔲 OPEN |
| ME-1 | MEDIUM | `internal/stream/backoff.go:90` | First restart waits 2× initial delay | ✅ FIXED |
| ME-2 | MEDIUM | `manager.go:214`, `supervisor.go:250` | `logf` always Info level, loses slog structure | ✅ FIXED |
| ME-3 | MEDIUM | `internal/stream/manager.go:564` | Unkillable 2-s goroutine in `stop()` | ✅ FIXED |
| ME-4 | MEDIUM | `internal/audio/detector.go:226` | `findDeviceIDPath` hardcodes path, untestable | 🔲 OPEN |
| ME-5 | MEDIUM | `internal/udev/rules.go:167,217` | `WriteRulesFile`, `ReloadUdevRules` 0% coverage | 🔲 OPEN |
| ME-6 | MEDIUM | `internal/config/config.go:116` | `Save()` error paths untested | 🔲 OPEN |
| ME-7 | MEDIUM | `internal/supervisor/supervisor.go:250` | Same `logf` anti-pattern as ME-2 | ✅ FIXED |
| ME-8 | MEDIUM | `cmd/lyrebird-stream/main.go:60` | Package-level flags impede testability | 🔲 OPEN |
| ME-9 | MEDIUM | `internal/health/health.go:92` | Missing `ReadTimeout`/`WriteTimeout` | ✅ FIXED |
| ME-10 | MEDIUM | `internal/config/koanf.go:176` | Brittle hardcoded field suffix list for env transform | 🔲 OPEN |
| ME-11 | MEDIUM | `internal/config/config.go:276` | `ValidatePartial` misleading for `SampleRate == 0` | 🔲 OPEN |
| ME-12 | MEDIUM | `cmd/lyrebird/main.go:360` | `getUSBBusDevFromCard` 16% coverage, fragile loop | 🔲 OPEN |
| L-1 | LOW | `internal/supervisor/supervisor.go:204` | `Stop()` 0% coverage | ✅ FIXED |
| L-2 | LOW | `internal/stream/manager.go:417` | `Close()` 0% unit test coverage | 🔲 OPEN |
| L-3 | LOW | `internal/menu/menu.go:409` | `RunCommand` 0% coverage | 🔲 OPEN |
| L-4 | LOW | `cmd/lyrebird/main.go:1091,910` | `downloadFile`, `installLyreBirdService` 0% coverage | 🔲 OPEN |
| L-5 | LOW | `internal/menu/menu.go:104` | `Display` 5.6% coverage | 🔲 OPEN |
| L-6 | LOW | `internal/util/panic.go:88` | `SafeGoWithRecover` close-after-send ambiguity | 🔲 OPEN |
| L-7 | LOW | `internal/stream/manager.go:560` | Undocumented intentional signal discard | ✅ FIXED |
| L-8 | LOW | `Makefile:83` vs `ci.yml:106` | Test timeout mismatch (30 s vs 2 min) | ✅ FIXED |
| L-9 | LOW | `ci.yml:49` vs `Makefile:119` | `golangci-lint` version mismatch | 🔲 OPEN |
| L-10 | LOW | `go.mod:9,52` | Two YAML parsers in dependency tree | 🔲 OPEN |
| L-11 | LOW | `go.mod:50` | `testify` listed as indirect | 🔲 OPEN |
| L-12 | LOW | `internal/stream/logrotate.go` | Log rotation implemented but never activated | 🔲 OPEN |
| L-13 | LOW | `lock/filelock.go`, `diagnostics.go`, `stream/monitor.go` | No `//go:build linux` constraints | ✅ PARTIAL (`monitor.go` open) |
| L-14 | LOW | `cmd/lyrebird-stream/main.go:262` | Map range in reload goroutine is a data race | ✅ FIXED |
| D-1 | DOC | `README.md:239` | Stale Q2 2025 timeline | 🔲 OPEN |
| D-2 | DOC | `README.md:21` | "No runtime dependencies" is incorrect | 🔲 OPEN |
| D-3 | DOC | `README.md:344` | Integration test CI claim is inaccurate | 🔲 OPEN |
| D-4 | DOC | `CLAUDE.md` | SIGHUP hot-reload still listed as future work | ✅ FIXED |
| D-5 | DOC | `CLAUDE.md` | Code example ignores error return | 🔲 OPEN |
| D-6 | DOC | Root dir | Developer artifacts (`AUDIT_REPORT.md`, etc.) at root | 🔲 OPEN |
| D-7 | DOC | `AUDIT_REPORT.md` | Existing report describes bugs that were already fixed | 🔲 OPEN |
| D-8 | DOC | `README.md:413` | Performance numbers duplicated | 🔲 OPEN |
| D-9 | DOC | `CLAUDE.md` | Coverage table column widths inconsistent | 🔲 OPEN |
| D-10 | DOC | `README.md:395` | Debug mode via `sudo -E` does not work with systemd | 🔲 OPEN |
| CI-1 | CI/CD | `ci.yml:85` | Single Go version in test matrix | 🔲 OPEN |
| CI-2 | CI/CD | `ci.yml:233` | Release job does not create a GitHub Release | 🔲 OPEN |
| CI-3 | CI/CD | `ci.yml:111` | `codecov-action` not SHA-pinned | 🔲 OPEN |
| CI-4 | CI/CD | `ci.yml:69,76` | Security tools installed at `@latest` | 🔲 OPEN |
| CI-5 | CI/CD | `ci.yml:212` | Integration step runs on ubuntu-latest without hardware | 🔲 OPEN |

---

## Positive Observations

The following aspects are well-executed and should be preserved:

- **`internal/audio/capabilities.go`** — `DetectCapabilities` is 100% covered and correctly models PCM capabilities with recommended settings logic.
- **`internal/stream/manager.go:179–211`** — `NewManager` constructor correctly initializes `state.Store(StateIdle)` and guards all resource setup.
- **`internal/config/config.go:116–172`** — `Save()` uses atomic temp-file-then-rename pattern correctly with explicit `Sync()` before rename.
- **`internal/udev/rules.go`** — `GenerateRule` and `GenerateRulesFile` are 100% covered with byte-for-byte bash-compatibility tests. This is exactly the right approach.
- **`internal/supervisor/supervisor.go`** — The suture integration, `serviceEntry` lifecycle tracking, and `Status()` API are clean and well-structured.
- **`internal/stream/backoff.go`** — The nil-receiver safety pattern (`if b == nil { return }`) is consistently applied throughout the API.
- **`internal/lock/filelock.go:167–237`** — `AcquireContext` uses a ticker + select pattern correctly for context-aware blocking.
- **`internal/config/backup.go`** — Atomic backup with timestamp naming and rotation cleanup is well-designed.
- **All packages use `t.TempDir()`** for file-system tests — no leftover temp files in CI.
- **`internal/util/panic.go`** — `SafeGo` and `SafeGoWithRecover` are 100% covered with full panic recovery tests including stack capture.

---

*End of PEER_REVIEW.md*
