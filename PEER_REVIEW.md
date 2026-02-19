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

## Summary

The project demonstrates strong engineering foundations: structured logging, context propagation, table-driven tests, atomic state management, and an Erlang-style supervisor tree. The internal packages are well-tested (85–94% coverage). However, the review identified **48 discrete issues** across five severity tiers, including critical concurrency bugs and a logic error in the lock subsystem that will cause lock theft for any stream running longer than five minutes — the primary operational scenario.

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

The stale-lock check considers a lock **stale if the lock file's `ModTime` is older than `DefaultStaleThreshold` (300 s), even when the process that owns the lock is confirmed to be running** (signal 0 succeeds). The check runs signal-0 to verify the process is alive, and then falls through to the age check regardless:

```go
err = process.Signal(syscall.Signal(0))
if err != nil {
    return true, nil  // process dead → stale (correct)
}

// Age check runs even when the process is alive:
age := time.Since(info.ModTime())
if age > threshold {
    return true, nil  // WRONG: running process treated as stale
}
```

Because `ModTime` is only updated when the PID is written at acquisition time, any stream running for longer than five minutes will have its lock file incorrectly treated as stale. A second process (e.g., a supervisor restart or a daemon reload) will then:

1. Call `os.Remove(fl.path)` — succeeds (removes the directory entry).
2. Open a new file at the same path, acquire `flock` on the new inode (no contention).
3. Both processes now believe they hold exclusive access to the same device.

This is the primary failure mode for the tool's core purpose: 24/7 continuous streaming. The fix requires the age check to only apply when the process is confirmed to be **dead** (the age check is a heuristic for zombie PIDs on systems where signal 0 does not always fail for dead processes).

---

### C-2 — Data Race: `registeredServices` Map Accessed from Multiple Goroutines

**File:** `cmd/lyrebird-stream/main.go:130–274`

`registeredServices` is a plain `map[string]bool` written and read from three goroutines concurrently:

- Goroutine 1 (poll loop, line 211): reads `koanfCfg.Load()` then calls `registerDevices(newCfg)` which reads and writes the map.
- Goroutine 2 (reload handler, line 238): calls `registerDevices(newCfg)` which reads and writes the map, and also iterates `registeredServices` for logging (line 262).
- Main goroutine (line 201): calls `registerDevices(cfg)` at startup.

Although the poll loop and reload handler never execute simultaneously in practice for the initial implementation, `go test -race` does not catch this because tests do not exercise concurrent SIGHUP + ticker scenarios. The map needs a `sync.RWMutex` or conversion to `sync.Map`.

---

### C-3 — Nil Pointer Dereference: `koanfCfg` Can Be Nil

**File:** `cmd/lyrebird-stream/main.go:221`

`loadConfigurationKoanf` returns `(nil, config.DefaultConfig(), nil)` when the config file is absent and `NewKoanfConfig` fails (lines 338–341). The caller stores the first return value as `koanfCfg`:

```go
koanfCfg, cfg, err := loadConfigurationKoanf(*configPath)
```

The poll loop then calls:

```go
newCfg, err := koanfCfg.Load()  // panic: nil pointer dereference
```

While `NewKoanfConfig` with only `WithEnvPrefix` is unlikely to fail today, the code is structurally unsafe. The nil case must be guarded or the API contract must be changed so `koanfCfg` is never nil on success.

---

### C-4 — Race Condition in `serviceWrapper.Serve` / `Stop`

**File:** `internal/supervisor/supervisor.go:161–208`

`serviceWrapper` stores `ctx` and `cancel` as plain fields. `Serve()` sets them (`w.ctx, w.cancel = context.WithCancel(ctx)`) while `Stop()` reads `w.cancel` — both can execute concurrently when suture calls `Stop()` shortly after `Serve()` starts:

```go
// Serve sets (no lock):
w.ctx, w.cancel = context.WithCancel(ctx)

// Stop reads (no lock):
func (w *serviceWrapper) Stop() {
    if w.cancel != nil {
        w.cancel()
    }
}
```

If `Stop()` is called between when `Serve()` is entered and when `w.cancel` is assigned, the service will not be stopped. This is a data race that `go test -race` cannot reliably catch without a stress test.

---

### C-5 — `m.cmd` Assigned Before `cmd.Start()` Succeeds

**File:** `internal/stream/manager.go:487–497`

```go
m.mu.Lock()
m.cmd = cmd        // assigned here
m.startTime = time.Now()
m.mu.Unlock()

if err := cmd.Start(); err != nil {
    return fmt.Errorf("failed to start ffmpeg: %w", err)
    // m.cmd is never cleared on error
}
```

If `cmd.Start()` fails, `m.cmd` points to an unstarted command. A concurrent call to `stop()` — which reads `m.cmd` — will attempt to signal a process that was never started, and the `Process` field will be nil, causing a nil pointer dereference in `proc.Signal(os.Interrupt)`.

---

## MAJOR Issues

### M-1 — Error Comparison Using `==` Instead of `errors.Is`

**Files:**
- `internal/stream/manager.go:290` — `if err == context.Canceled`
- `cmd/lyrebird-stream/main.go:279` — `if err := sup.Run(ctx); err != nil && err != context.Canceled`
- `cmd/lyrebird-stream/main.go:300` — `if err != nil && err != context.Canceled`

Comparing with `==` misses wrapped errors (e.g., `fmt.Errorf("...: %w", context.Canceled)`). Every one of these should use `errors.Is(err, context.Canceled)`. This is a correctness bug: in some code paths the wrapped context error will not be recognized as a normal shutdown, causing spurious error logs or incorrect behavior.

---

### M-2 — Watchdog Signal Never Sent (`WatchdogSec` Without `sd_notify`)

**File:** `systemd/lyrebird-stream.service:67`

The service file sets `WatchdogSec=60`. systemd requires the service to call `sd_notify(0, "WATCHDOG=1")` at least once every 60 seconds. The daemon has no such call. As a result, systemd will kill and restart the service every 60 seconds on any system that has `libsystemd` watchdog enforcement active (which is the default for modern systemd). This means the daemon cannot run continuously in production as deployed via the provided service file.

Either remove `WatchdogSec` or implement the watchdog keepalive.

---

### M-3 — Health Endpoint Is Implemented But Never Started

**Files:** `internal/health/health.go`, `cmd/lyrebird-stream/main.go`

`internal/health` provides `ListenAndServe` and a complete `Handler`. The daemon never starts it. The README and systemd comments advertise health monitoring. Users relying on `/healthz` for liveness probes will find no server listening. This is a feature that is documented but does not exist at runtime.

---

### M-4 — Device Polling Only Triggers When No Services Are Registered

**File:** `cmd/lyrebird-stream/main.go:219`

```go
if sup.ServiceCount() == 0 {
    // Only re-detect here
}
```

If the daemon starts with one USB microphone and a second is later plugged in, the second device is never registered. The poll loop only runs device detection when the service count is zero. This contradicts the "USB Hotplug Support" feature listed in the README.

---

### M-5 — `Manager.Close()` Never Called; Log File Handles Leak

**Files:** `internal/stream/manager.go:417`, `cmd/lyrebird-stream/main.go:297–305`

`Manager.Close()` releases the rotating log writer. `streamService.Run()` calls `m.manager.Run(ctx)` but never calls `m.manager.Close()`. Every manager restart (due to supervisor re-launching `streamService`) accumulates an open but unclosed log file handle. Over time this will exhaust the process's file descriptor limit.

---

### M-6 — `registeredServices` Polling and Hot-Reload Cannot Register Existing Devices With New Config

**File:** `cmd/lyrebird-stream/main.go:134–198`

On SIGHUP, `registerDevices` skips all devices already in `registeredServices`. If a device's configuration changes (different sample rate, codec, etc.) and the user sends SIGHUP, the already-running stream continues with the old configuration. The README claims "future enhancements will restart only affected streams," but there is no mechanism to restart streams with stale config even in the future — the architecture prevents it because the map has no config hash comparison.

---

### M-7 — Misleading `isLockStale` Comment on Age Check

**File:** `internal/lock/filelock.go:339–343`

```go
age := time.Since(info.ModTime())
if age > threshold {
    // Lock is old - likely stale
    return true, nil
}
```

The comment "likely stale" signals awareness of the heuristic nature, but there is no acknowledgement that this triggers incorrectly for healthy long-running processes. This is directly related to C-1.

---

### M-8 — `koanf.go` env Transform Comment Is Misleading

**File:** `internal/config/koanf.go:152–154`

```go
TransformFunc: func(k, v string) (string, any) {
    // Remove prefix (already handled by Prefix option)
    k = strings.TrimPrefix(k, kc.envPrefix+"_")
```

The comment claims the prefix removal is "already handled by Prefix option." In koanf v2's `env` provider, the `Prefix` field filters which env vars are loaded but does **not** strip the prefix before calling `TransformFunc`. The prefix stripping here is required, not redundant. The misleading comment will cause future maintainers to delete the line, breaking all environment variable overrides.

---

### M-9 — `Watch()` Does Not Stop the File Watcher on Context Cancellation

**File:** `internal/config/koanf.go:239–271`

`fp.Watch(...)` starts an internal `fsnotify` goroutine. The function then blocks on `<-ctx.Done()` and returns — without stopping the watcher. The `fsnotify` watcher and its goroutine continue running after the function returns, leaking file descriptor and goroutine resources. The file provider offers no explicit `Stop()` in koanf v2's API, but the goroutine leak is still a defect that accumulates on every config reload cycle.

---

### M-10 — `runDetect` Outputs Hardcoded Settings, Ignores Actual Device Capabilities

**File:** `cmd/lyrebird/main.go:218–246` (`runDetectWithPath`)

The `detect` command is supposed to inspect a device and recommend optimal settings. The implementation outputs hardcoded recommendations (`48000 Hz`, `stereo`, `opus`, `128k`) regardless of the actual device. `internal/audio/capabilities.go` implements `DetectCapabilities` with full PCM info parsing and is 100% covered — but is never called by the CLI. The `detect` command is a misleading stub.

---

### M-11 — `allPassed` Logic Broken in `runTest`

**File:** `cmd/lyrebird/main.go:1152` (`runTest`)

`allPassed` is only set to `false` when FFmpeg is not found (test 3). Tests 2 (config syntax check warnings), 4 (MediaMTX reachability), and 5 (RTSP port) emit "WARNING" output but do not update `allPassed`. The final output can read "All tests passed!" even when MediaMTX is unreachable and RTSP is inaccessible. This is especially dangerous as a pre-flight check.

---

### M-12 — `installLyreBirdService` Writes a Stripped-Down Service File

**File:** `cmd/lyrebird/main.go:910` (`installLyreBirdService`)

The service written by the `setup` command lacks all security hardening present in `systemd/lyrebird-stream.service`: `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `MemoryDenyWriteExecute`, `RestrictSUIDSGID`, `SystemCallFilter`, `DevicePolicy`, etc. Users who install via `lyrebird setup` receive a significantly less secure service than those who manually copy from the repository. These two service definitions must be kept in sync or the embedded one must be removed in favor of the versioned file.

---

### M-13 — Self-Update Has No Checksum Verification

**File:** `internal/updater/updater.go:266` (`Download`)

The updater downloads a binary from GitHub releases but does not verify the downloaded content against `checksums.txt` (which is published in releases). An attacker who compromises the CDN or performs a MITM attack receives arbitrary code execution on the target system. The updater must verify the SHA256 checksum before replacing the binary.

---

## MEDIUM Issues

### ME-1 — `Backoff.RecordFailure` Doubles Delay on First Call

**File:** `internal/stream/backoff.go:90–103`

At construction, `currentDelay = initialDelay`. `RecordFailure()` immediately doubles it (`currentDelay *= 2`). The first restart therefore waits `2 × initialDelay`, not `initialDelay` as documented and configured. With the default 10 s initial delay, the first restart waits 20 s. The CLAUDE.md, README, and comments all state "Initial delay: 10s."

---

### ME-2 — `logf` Always Uses `Info` Level and Loses Structured Logging Benefits

**Files:**
- `internal/stream/manager.go:214–217`
- `internal/supervisor/supervisor.go:250–253`

Both `logf` wrappers call `logger.Info(fmt.Sprintf(format, args...))` for all messages including errors and warnings. This:

1. Loses all `slog` structured fields except the single injected "device"/"component" key.
2. Uses Info level for error messages (e.g., "FFmpeg failed").
3. Defeats the purpose of using `log/slog`.

Error-level events should call `logger.Error(msg, kvpairs...)` and warnings should call `logger.Warn(...)`.

---

### ME-3 — `stop()` Spawns an Unkillable 2-Second Goroutine

**File:** `internal/stream/manager.go:564–570`

```go
go func() {
    time.Sleep(2 * time.Second)
    _ = proc.Kill()
}()
```

This goroutine has no context or cancellation mechanism. If FFmpeg exits within the 2-second window (the common case), the goroutine continues sleeping and then issues a `Kill()` on an already-reaped process. If the manager is stopped and immediately restarted, the stale goroutine may overlap with a new process's lifecycle. Under high restart frequency this adds latency and creates ambiguous process state.

---

### ME-4 — `findDeviceIDPath` Hardcodes `/dev/snd/by-id`, Untestable

**File:** `internal/audio/detector.go:226–258`

The path `/dev/snd/by-id` is hardcoded and cannot be injected for testing, resulting in 27.8% coverage. The function should accept the `byIDDir` as a parameter (same pattern used for `asoundPath` and `sysfsPath` throughout the package).

---

### ME-5 — `udev.WriteRulesFile` and `udev.ReloadUdevRules` Have 0% Coverage

**File:** `internal/udev/rules.go:167, 217`

These two functions write to `/etc/udev/rules.d/` and call `udevadm`. They are system-modifying functions with zero test coverage. `WriteRulesFileToPath` (80% covered) shows that testable variants can be written. `ReloadUdevRules` should accept an injectable command runner for testing.

---

### ME-6 — `config.Save()` Coverage Is 68%; Atomic-Write Error Paths Untested

**File:** `internal/config/config.go:116` (`Save`)

`Save` implements a careful atomic write (temp file → sync → chmod → rename). The `Chmod`, `Sync`, and second `Close` error branches are not covered. For a function that modifies a production configuration file, all error paths must be tested.

---

### ME-7 — `logf` in `supervisor.go` Uses `fmt.Sprintf` Then `slog.Info`

**File:** `internal/supervisor/supervisor.go:250–253`

Same structural anti-pattern as ME-2. `logf` formats a message string and passes it to `slog.Info` as a single positional argument, losing all structured-logging benefits.

---

### ME-8 — Package-Level Flag Variables Impede Testability

**File:** `cmd/lyrebird-stream/main.go:60–65`

```go
var (
    configPath = flag.String("config", ...)
    lockDir    = flag.String("lock-dir", ...)
    ...
)
```

Package-level `flag` variables are parsed once at startup. Tests that call `flag.Parse()` multiple times (or that run in parallel) will see stale values. The standard Go idiom for testable CLIs is to pass a `*flag.FlagSet` or accept configuration via function parameters.

---

### ME-9 — `internal/health` HTTP Server Missing `ReadTimeout` and `WriteTimeout`

**File:** `internal/health/health.go:92–96`

```go
srv := &http.Server{
    Addr:              addr,
    Handler:           handler,
    ReadHeaderTimeout: 5 * time.Second,
    // ReadTimeout and WriteTimeout missing
}
```

Without `ReadTimeout` and `WriteTimeout`, the health server is vulnerable to slow-client attacks (Slowloris). `ReadHeaderTimeout` protects only the header phase.

---

### ME-10 — env Transform Is Brittle: New `DeviceConfig` Fields Break env Overrides

**File:** `internal/config/koanf.go:176–186`

The transform uses a hardcoded list to identify DeviceConfig field suffixes:

```go
knownFields := []string{"_sample_rate", "_channels", "_bitrate", "_codec", "_thread_queue"}
```

Any new field added to `DeviceConfig` must be added here as well. The CLAUDE.md documents this as an env-var override feature. Missing this list update is a silent failure (the env var is loaded but mapped to the wrong key). A more robust approach is to use double-underscore delimiters (`LYREBIRD_DEVICES__BLUE_YETI__SAMPLE_RATE`) to avoid the guessing logic entirely.

---

### ME-11 — `ValidatePartial` Allows `SampleRate < 0` to Pass

**File:** `internal/config/config.go:276`

```go
func (d *DeviceConfig) ValidatePartial() error {
    if d.SampleRate < 0 {
        return fmt.Errorf("sample_rate must be positive")
    }
```

`SampleRate == 0` passes validation. If a device config sets `sample_rate: 0` explicitly (possibly from a misconfigured env var), `GetDeviceConfig` will propagate 0 because `if devCfg.SampleRate != 0` is false, leaving the default in place. This is the intended fallback behavior but the validation error message ("must be positive") is misleading for value 0.

---

### ME-12 — `getUSBBusDevFromCard` Has 16% Coverage and Fragile Loop Logic

**File:** `cmd/lyrebird/main.go:360` (`getUSBBusDevFromCard`)

This function traverses sysfs to find USB bus/device numbers. It has only 16% coverage and contains a loop that uses `continue` after a failed `Sscanf` without resetting partial state. Additionally, it is not used by any test that exercises the sysfs traversal path.

---

## LOW Issues

### L-1 — `supervisor.serviceWrapper.Stop()` Has 0% Coverage

**File:** `internal/supervisor/supervisor.go:204`

`Stop()` is called by suture when removing a service gracefully. It has zero test coverage. This is the cancellation path for every supervised stream. It should be exercised by a test that calls `Remove()` while a service is running.

---

### L-2 — `Manager.Close()` Has 0% Coverage

**File:** `internal/stream/manager.go:417`

The function responsible for releasing log file handles is never called in tests. Because it is also never called in the daemon (M-5), coverage reflects the real-world omission.

---

### L-3 — `menu.RunCommand` Has 0% Coverage

**File:** `internal/menu/menu.go:409`

`RunCommand` executes a shell command and writes output to the menu's writer. It is completely untested. The function uses `exec.Command` which is injectable via `io.Writer` and mockable.

---

### L-4 — `downloadFile` and `installLyreBirdService` Have 0% Coverage

**File:** `cmd/lyrebird/main.go:1091, 910`

Both functions are reachable in normal use but have zero tests. `downloadFile` shelling out to curl/wget can be tested with a local HTTP test server.

---

### L-5 — `menu.Display` Has 5.6% Coverage; `createDeviceMenu` 36.4%

**File:** `internal/menu/menu.go:104, 492`

The primary public entry point for the menu package has near-zero coverage. The scanner-based fallback path (`displayWithScanner` at 83.3%) is tested; the `huh`-based path is not. At minimum the `WithInput` / `WithOutput` injection pattern should be used in tests to exercise the full display path.

---

### L-6 — `SafeGoWithRecover` Closes Channel After Sending Error

**File:** `internal/util/panic.go:88–96`

```go
if errCh != nil {
    if err != nil {
        errCh <- err
    }
    close(errCh)
}
```

If `err != nil`, the channel receives an error and is then closed. The caller must handle both a received error and a subsequent zero-value read from the closed channel, or use a `for range` loop. The documented usage example only reads once (`<-errCh`), which will correctly return the error — but the close creates ambiguity. The pattern should use a single send-and-close pattern or a done channel.

---

### L-7 — `stop()` Ignores SIGINT Return Value for Wrong Reasons

**File:** `internal/stream/manager.go:560`

```go
_ = proc.Signal(os.Interrupt)
```

The discard is documented in `#nosec G204` comments elsewhere but there is no comment here explaining why the signal error is intentionally discarded. On a process that has already exited (between the nil check and the signal call), the error would be ESRCH. The comment should clarify this is the known-acceptable race.

---

### L-8 — Makefile `test` Timeout (30 s) Diverges from CI Timeout (2 min)

**File:** `Makefile:83`, `ci.yml:106`

```makefile
# Makefile
go test -v -race -timeout 30s ./...

# ci.yml
go test -race -timeout 2m ./...
```

The `internal/lock` tests take 3.6 s under race detection (8.5 s without). A slower CI runner or additional tests could push the local `make test` over 30 s. The Makefile should match CI or be documented as intentionally stricter.

---

### L-9 — `golangci-lint` Version Mismatch Between CI and Makefile

**File:** `ci.yml:49`, `Makefile:119`

CI pins `golangci-lint@v1.62.2`. The Makefile installs `@latest`. Different lint rule versions may produce different results, making local `make lint` unreliable as a CI gate verification.

---

### L-10 — `go.mod` Contains Two YAML Parsers

**File:** `go.mod:9,52`

```
gopkg.in/yaml.v3 v3.0.1
go.yaml.in/yaml/v3 v3.0.4 // indirect
```

Two separate YAML parsing libraries are in the dependency tree (the second is pulled by koanf). This is not a bug but adds binary size and cognitive overhead. It is worth tracking whether koanf's direct dependency on `go.yaml.in/yaml/v3` can be resolved to the same `gopkg.in/yaml.v3`.

---

### L-11 — `stretchr/testify` Listed as Indirect Dependency

**File:** `go.mod:50`

```
github.com/stretchr/testify v1.11.1 // indirect
```

If any test file uses `testify` assertions directly it should be a direct dependency. If it is truly only pulled transitively, it should not appear at this version in `go.mod` after `go mod tidy`.

---

### L-12 — `logrotate.go` Feature Not Wired in Daemon

**File:** `internal/stream/logrotate.go`, `cmd/lyrebird-stream/main.go`

`RotatingWriter` is implemented and tested (85% coverage) but the daemon never sets `ManagerConfig.LogDir`, so log rotation never activates. The FFmpeg log output is silently discarded. The feature should either be wired up or the `LogDir` field should be documented as intentionally unused.

---

### L-13 — Platform Build Constraints Missing

**Files:** `internal/lock/filelock.go`, `internal/stream/monitor.go`

Both files use Linux-specific syscalls (`syscall.Flock`, `/proc` filesystem). There are no `//go:build linux` constraints. Cross-compiling for non-Linux targets (even accidentally) will produce binaries that fail at runtime. The CI only cross-compiles for Linux targets, so this is a latent risk rather than an active defect.

---

### L-14 — `registeredServices` Read Without Lock in Reload Goroutine (Logging)

**File:** `cmd/lyrebird-stream/main.go:262–269`

```go
for devName := range registeredServices {
    devCfg := newCfg.GetDeviceConfig(devName)
    logger.Info("device config after reload", ...)
}
```

This range loop runs in the reload goroutine while the poll goroutine may concurrently write to `registeredServices`. Even read-only iteration is a data race if another goroutine is writing concurrently.

---

## Documentation Issues

### D-1 — README: Migration Timeline Is Stale

**File:** `README.md:239`

> "estimated: Q2 2025"

The current date is February 2026. The timeline has passed. Either update with a new estimate or remove the date entirely.

---

### D-2 — README: "No Runtime Dependencies" Is Inaccurate

**File:** `README.md:21`

> "Static Binary Deployment: Single binary with no runtime dependencies"

The binary shells out to `ffmpeg`, `curl` or `wget`, `udevadm`, `systemctl`, `tar`, and `install`. These are runtime dependencies. The correct claim is "no shared library dependencies" (CGO_ENABLED=0 produces a statically linked Go binary).

---

### D-3 — README: Integration Tests Claim Is Inaccurate

**File:** `README.md:344`

> "Integration tests (on self-hosted runner with USB devices)"

The CI uses `runs-on: ubuntu-latest`, not a self-hosted runner. The integration test step has a comment: `# Note: This will be skipped if no integration tests exist yet`. No integration tests exist. The README overstates CI capabilities.

---

### D-4 — CLAUDE.md: "Future Work" Section Is Outdated

**File:** `CLAUDE.md` — "Future Work / Remaining"

Lists "Hot-reload configuration via SIGHUP" as remaining work. This feature is implemented and documented in both README.md and the systemd service file. The section must be updated.

---

### D-5 — CLAUDE.md: Code Example Omits Error Handling

**File:** `CLAUDE.md` — koanf example

```go
cfg, err := kc.Load()
devCfg := cfg.GetDeviceConfig("blue_yeti")
```

`err` is declared but never checked. Documentation code examples that ignore errors teach the wrong pattern, especially in a project with a "zero compromises" reliability mandate.

---

### D-6 — Developer Artifacts at Repository Root

**Files:** `AUDIT_REPORT.md`, `FINDINGS.md`, `IMPLEMENTATION_PLAN.md`, `IMPROVEMENTS_SUMMARY.md`

These implementation planning and audit documents belong in a `/docs` directory, a wiki, or pull request descriptions — not at the repository root. They will confuse users browsing the repository and pollute the project structure. They also reference a previous review branch (`claude/complete-codebase-audit-YNcmO`) which contains information that is now partially superseded.

---

### D-7 — AUDIT_REPORT.md Contains Inaccurate Bug Descriptions

**File:** `AUDIT_REPORT.md:30–43`

Issue 1.1 claims `m.state.Load()` can return nil. The code in `manager.go:189` calls `mgr.state.Store(StateIdle)` in the constructor, and `State()` (line 361) explicitly handles `v == nil`. Issue 1.2 claims `Backoff.Reset` has no nil check — the code at `backoff.go:216` has an explicit nil check. The existing audit report has not been updated after fixes were applied.

---

### D-8 — README: Performance Numbers Are Duplicated With Different Wording

**File:** `README.md:413–433`

"Resource Usage" (lines 417–420) and "Benchmarks" (lines 429–433) both list the same figures for CPU and memory with slightly different phrasing. One section should be removed or the two should be merged.

---

### D-9 — CLAUDE.md Coverage Table Formatting Is Inconsistent

**File:** `CLAUDE.md` — "Current Test Coverage" table

Column widths in the Markdown table are not padded consistently. This renders correctly in most parsers but is a style inconsistency in a document that specifies "every piece of documentation must be perfect."

---

### D-10 — README `Debug Mode` Section Is Misleading

**File:** `README.md:395`

```bash
export LYREBIRD_LOG_LEVEL=debug
sudo -E systemctl restart lyrebird-stream
```

`sudo -E` does not inject environment variables into systemd-managed services in most configurations. The correct mechanism is to set `LYREBIRD_LOG_LEVEL=debug` in `/etc/lyrebird/environment` (referenced by the `EnvironmentFile=` directive in the service file), then restart. The documented approach will silently have no effect.

---

## CI/CD Issues

### CI-1 — CI Tests Only One Go Version

**File:** `ci.yml:85`

The test matrix has a single entry: `['1.24.13']`. The project declares `go 1.24.2` as the minimum in `go.mod`. Testing against both the minimum and latest would confirm backward compatibility.

---

### CI-2 — Release Job Does Not Create a GitHub Release

**File:** `ci.yml:233–266`

The `release` job downloads artifacts and uploads them again as a single artifact but never calls `gh release create` or any equivalent. On tag pushes, no actual GitHub Release is created. The job is effectively a no-op for the release process.

---

### CI-3 — `codecov/codecov-action` Uses `v4` Without SHA Pin

**File:** `ci.yml:111`

```yaml
uses: codecov/codecov-action@v4
```

Unpinned floating tags are a supply-chain risk. All third-party GitHub Actions should be pinned to a full commit SHA.

---

### CI-4 — `gosec` and `govulncheck` Installed at `@latest` Without Version Pin

**File:** `ci.yml:69, 76`

```yaml
run: go install github.com/securego/gosec/v2/cmd/gosec@latest
run: go install golang.org/x/vuln/cmd/govulncheck@latest
```

Installing security tools at `@latest` in CI means tool updates can silently break builds or, conversely, miss newly detected issues if the `@latest` resolution is cached. Pin these to specific versions.

---

### CI-5 — Integration Test Step Runs on `ubuntu-latest` Without Hardware

**File:** `ci.yml:212`

The integration step is conditioned on `main` branch or `workflow_dispatch`. It runs on `ubuntu-latest` which has no USB devices. The step will silently succeed (no integration-tagged tests exist). This gives a false sense of integration test coverage.

---

## Checklist Summary

| ID | Severity | File | Issue |
|----|----------|------|-------|
| C-1 | CRITICAL | `internal/lock/filelock.go:296` | Lock theft for running processes after 300 s |
| C-2 | CRITICAL | `cmd/lyrebird-stream/main.go:130` | `registeredServices` map race |
| C-3 | CRITICAL | `cmd/lyrebird-stream/main.go:221` | Nil pointer dereference on `koanfCfg.Load()` |
| C-4 | CRITICAL | `internal/supervisor/supervisor.go:161` | Race in `Serve`/`Stop` on `cancel` field |
| C-5 | CRITICAL | `internal/stream/manager.go:487` | `m.cmd` set before `cmd.Start()` succeeds |
| M-1 | MAJOR | `manager.go:290`, `main.go:279,300` | `==` instead of `errors.Is` for context errors |
| M-2 | MAJOR | `systemd/lyrebird-stream.service:67` | `WatchdogSec` without `sd_notify` |
| M-3 | MAJOR | `internal/health/health.go` | Health endpoint implemented but never started |
| M-4 | MAJOR | `cmd/lyrebird-stream/main.go:219` | Hotplug only works when no services exist |
| M-5 | MAJOR | `internal/stream/manager.go:417` | `Manager.Close()` never called; fd leak |
| M-6 | MAJOR | `cmd/lyrebird-stream/main.go:134` | Config changes on SIGHUP not applied to running streams |
| M-7 | MAJOR | `internal/lock/filelock.go:339` | Age-check comment obscures C-1 logic flaw |
| M-8 | MAJOR | `internal/config/koanf.go:152` | Misleading "already handled" comment |
| M-9 | MAJOR | `internal/config/koanf.go:248` | File watcher goroutine leaked on ctx cancel |
| M-10 | MAJOR | `cmd/lyrebird/main.go:218` | `detect` uses hardcoded settings, not capabilities |
| M-11 | MAJOR | `cmd/lyrebird/main.go:1152` | `allPassed` not updated by warning tests |
| M-12 | MAJOR | `cmd/lyrebird/main.go:910` | `installLyreBirdService` lacks security hardening |
| M-13 | MAJOR | `internal/updater/updater.go:266` | Self-update has no checksum verification |
| ME-1 | MEDIUM | `internal/stream/backoff.go:90` | First restart waits 2× initial delay |
| ME-2 | MEDIUM | `manager.go:214`, `supervisor.go:250` | `logf` always Info level, loses slog structure |
| ME-3 | MEDIUM | `internal/stream/manager.go:564` | Unkillable 2-s goroutine in `stop()` |
| ME-4 | MEDIUM | `internal/audio/detector.go:226` | `findDeviceIDPath` hardcodes path, untestable |
| ME-5 | MEDIUM | `internal/udev/rules.go:167,217` | `WriteRulesFile`, `ReloadUdevRules` 0% coverage |
| ME-6 | MEDIUM | `internal/config/config.go:116` | `Save()` error paths untested |
| ME-7 | MEDIUM | `internal/supervisor/supervisor.go:250` | Same `logf` anti-pattern as ME-2 |
| ME-8 | MEDIUM | `cmd/lyrebird-stream/main.go:60` | Package-level flags impede testability |
| ME-9 | MEDIUM | `internal/health/health.go:92` | Missing `ReadTimeout`/`WriteTimeout` |
| ME-10 | MEDIUM | `internal/config/koanf.go:176` | Brittle hardcoded field suffix list for env transform |
| ME-11 | MEDIUM | `internal/config/config.go:276` | `ValidatePartial` misleading for `SampleRate == 0` |
| ME-12 | MEDIUM | `cmd/lyrebird/main.go:360` | `getUSBBusDevFromCard` 16% coverage, fragile loop |
| L-1 | LOW | `internal/supervisor/supervisor.go:204` | `Stop()` 0% coverage |
| L-2 | LOW | `internal/stream/manager.go:417` | `Close()` 0% coverage |
| L-3 | LOW | `internal/menu/menu.go:409` | `RunCommand` 0% coverage |
| L-4 | LOW | `cmd/lyrebird/main.go:1091,910` | `downloadFile`, `installLyreBirdService` 0% coverage |
| L-5 | LOW | `internal/menu/menu.go:104` | `Display` 5.6% coverage |
| L-6 | LOW | `internal/util/panic.go:88` | `SafeGoWithRecover` close-after-send ambiguity |
| L-7 | LOW | `internal/stream/manager.go:560` | Undocumented intentional signal discard |
| L-8 | LOW | `Makefile:83` vs `ci.yml:106` | Test timeout mismatch (30 s vs 2 min) |
| L-9 | LOW | `ci.yml:49` vs `Makefile:119` | `golangci-lint` version mismatch |
| L-10 | LOW | `go.mod:9,52` | Two YAML parsers in dependency tree |
| L-11 | LOW | `go.mod:50` | `testify` listed as indirect |
| L-12 | LOW | `internal/stream/logrotate.go` | Log rotation implemented but never activated |
| L-13 | LOW | `lock/filelock.go`, `stream/monitor.go` | No `//go:build linux` constraints |
| L-14 | LOW | `cmd/lyrebird-stream/main.go:262` | Map range in reload goroutine is also a data race |
| D-1 | DOC | `README.md:239` | Stale Q2 2025 timeline |
| D-2 | DOC | `README.md:21` | "No runtime dependencies" is incorrect |
| D-3 | DOC | `README.md:344` | Integration test CI claim is inaccurate |
| D-4 | DOC | `CLAUDE.md` | SIGHUP hot-reload still listed as future work |
| D-5 | DOC | `CLAUDE.md` | Code example ignores error return |
| D-6 | DOC | Root dir | Developer artifacts (`AUDIT_REPORT.md`, `FINDINGS.md`, etc.) at root |
| D-7 | DOC | `AUDIT_REPORT.md` | Existing report describes bugs that were already fixed |
| D-8 | DOC | `README.md:413` | Performance numbers duplicated |
| D-9 | DOC | `CLAUDE.md` | Coverage table column widths inconsistent |
| D-10 | DOC | `README.md:395` | Debug mode via `sudo -E` does not work with systemd |
| CI-1 | CI/CD | `ci.yml:85` | Single Go version in test matrix |
| CI-2 | CI/CD | `ci.yml:233` | Release job does not create a GitHub Release |
| CI-3 | CI/CD | `ci.yml:111` | `codecov-action` not SHA-pinned |
| CI-4 | CI/CD | `ci.yml:69,76` | Security tools installed at `@latest` |
| CI-5 | CI/CD | `ci.yml:212` | Integration step runs on ubuntu-latest without hardware |

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
