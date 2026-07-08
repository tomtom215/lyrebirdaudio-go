# Engineering Excellence Review — July 2026

> **Scope**: Complete line-by-line review of every package and every test, full
> static-analysis + E2E toolchain, and modernization against the latest releases
> of Go, MediaMTX, and all dependencies (as of 2026-07-08).
>
> **Method**: Eight parallel deep-review passes (one per package cluster) over
> all 55 non-test and 250 test files; the full analyzer suite (`go vet`,
> `staticcheck`, `gosec`, `govulncheck`, `golangci-lint` v2, `shellcheck`,
> `bash -n`); and a **real** end-to-end harness that runs an actual MediaMTX
> v1.19.2 server plus a real ffmpeg publisher — no USB hardware. Every fix below
> was verified against real behavior, not mocks. Findings were cross-checked
> against authoritative sources (MediaMTX source via the Go module proxy, live
> API captures, systemd/Go release notes) rather than assumed.

---

## Headline: the tool did not actually work in several fundamental ways

Despite ~90% test coverage and clean analyzers, the suite mocked away reality —
API responses were built from the client's own structs, locks were tested only
in-process, and no test ran real ffmpeg or MediaMTX. That masked a set of
production-breaking defects. The single most important:

### CRITICAL — MediaMTX `tracks` decoded as objects, breaking all monitoring

`internal/mediamtx/client.go` modeled a path's `tracks` field as `[]Track`
(objects). The real MediaMTX v3 API emits it as an array of **codec-label
strings** (e.g. `["Opus"]`), verified against a live v1.19.2 server. So
`json.Decode` returned an `UnmarshalTypeError` on **every path that carried a
track**, and `GetPath` — and therefore `ListPaths`, `IsStreamHealthy`,
`GetStreamStats`, `WaitForStream`, `HealthCheck` — failed the moment a stream
started producing audio. The daemon's stall detection and auto-restart, the core
24/7 self-healing feature, silently never fired. The existing tests passed only
because their mock servers `json.Encode`d the client's own structs, so the
objects round-tripped and never exercised the real string wire format.

**Fixed** (`f22e349`): model `tracks` as `[]string`; replace the
struct-round-trip mocks with real captured wire-format JSON; add a regression
test built from a live-server capture.

---

## HIGH findings (all fixed, all with regression tests)

| # | Area | Defect | Commit |
|---|------|--------|--------|
| H1 | systemd | `DeviceAllow=/dev/snd/* rw` — systemd does not glob, so with `DevicePolicy=closed` **all audio devices were denied** (no capture) under the hardened unit. Now `char-alsa`. | `81f356c` |
| H2 | systemd | `ReadWritePaths=/var/run/lyrebird` referenced a tmpfs path wiped on boot → unit failed `226/NAMESPACE` after a reboot. Now `RuntimeDirectory`/`StateDirectory`. | `81f356c` |
| H3 | updater | Self-update copied over the running binary in place → `ETXTBSY`, so `lyrebird update` could never work; also non-atomic. Now stage + atomic rename. | `acbdd9a` |
| H4 | lock | Stale-lock handling unlinked and recreated the file, so two acquirers could `flock` two different inodes at one path and both "hold" it — two managers on one device. Now `flock` is the sole gate. | `d44cb3e` |
| H5 | stream | ffmpeg was built with `exec.CommandContext(ctx)`, so os/exec **SIGKILLed** it on shutdown, truncating the in-progress recording segment (no container trailer) on every shutdown/reload/restart. Now a single graceful SIGINT → `StopTimeout` → SIGKILL. | `8f6e840` |
| H6 | stream | Successful runs counted toward the max-attempts ceiling, which never reset on success → a healthy stream that merely restarted ~50 times over its life was permanently abandoned. Now a successful run resets the counter. | `9ff0da2` |
| H7 | config | Stream restart/backoff timing was unvalidated: `max_restart_attempts: 0` (or an omitted `stream:` section) made every stream fail before FFmpeg launched, and a bare `initial_restart_delay: 45` was decoded as 45 nanoseconds. Now validated with unit-aware errors. | `f42b601` |
| H8 | config | Both loaders unmarshaled into a zero `Config`, so any omitted field became the Go zero value instead of its documented default (root cause of H7's first case). Now both unmarshal on top of `DefaultConfig()`. | `f42b601` |

---

## Modernization (latest versions, verified)

- **Go toolchain 1.24.7 → 1.25.12** (latest stable): resolves **22 stdlib CVEs**
  (`GO-2025-4007/4008/4009` in crypto/x509, crypto/tls, encoding/pem) that
  `govulncheck` reported; it now reports zero. (`21ddc34`)
- **MediaMTX default install 1.17.1 → v1.19.2** and **auto-enable the control
  API** on install — the stock config ships `api: false`, so lyrebird's status,
  monitoring and session management were silently non-functional out of the box.
  (`eea58d8`)
- **Dependencies to latest**: `huh v0.8.0 → v1.0.0`, `koanf → v2.3.5`, fsnotify,
  mapstructure, `golang.org/x/*`, etc. (`21ddc34`)
- **golangci-lint unified on v2** (was three different pinned versions across CI,
  Makefile, and reality) with a committed `.golangci.yml` and the real
  `errorlint`/wrapped-error issues fixed. (`544d907`)
- **CI**: Go 1.25.12 everywhere, a real min-version floor (`GOTOOLCHAIN=local`),
  a **shellcheck + bash -n** job, an **E2E job** that downloads MediaMTX and runs
  the harness, and an anchored coverage grep. (`469768e`, `de4b83a`)
- **Makefile**: `build-all` now builds the daemon too, `fmt` uses `gofmt -s`.

## Hardware-free E2E harness (`test/e2e`, `de4b83a`)

A new `-tags e2e` harness starts a real MediaMTX server (API enabled) and a real
ffmpeg publisher generating a synthetic sine tone — no USB microphone — and
drives the full client surface against real wire-format responses. This is the
class of test that would have caught the CRITICAL. Run with `make test-e2e`.

---

## MEDIUM / LOW findings — all resolved

> **Update (2026-07-08, follow-up session):** every MEDIUM and LOW item below
> has now been fixed, each with regression tests, under `go test -race`,
> `golangci-lint` (default + `e2e,integration` tags), and `govulncheck`, and the
> real MediaMTX v1.19.2 + ffmpeg E2E harness. Resolution commits are noted inline.

### MEDIUM (resolved)

Resolutions: mediamtx pagination `3398aba`; health NTP-soft + provider TTL cache
`8cfdb75`; daemon stall-state prune + watchdog liveness gating + manager fd leak
`149a6a2`; diagnostics per-check timeout + disk dir `b3bb611`; updater
arm/arm64 exact-match + fail-closed checksum `aef2372`; stream backoff jitter +
dead-config removal `0c17d97`; audio channel recommendation + capture-only caps
`5f7db9d`; cli exit codes `c1b1e1b` + status EPERM/bundle perms `f0f90b2`; menu
isatty + editor stdin `9e88fb7`; config validate-before-swap + fsnotify unwatch +
de-flaked watch test `a0f8a50` + sub-second backups & atomic restore `4cc14fb`.

Original finding list (for traceability):
- `mediamtx/sessions.go`: `ListRTSPSessions` fetches only the first page; add
  auto-pagination so stall-recovery can find readers on page 2+.
- `health/health.go`: NTP desync returns HTTP 503 despite being a soft warning →
  a routine chrony re-sync can flap the endpoint / trigger restarts.
- `health/health.go`: providers run `exec("timedatectl")` + `Statfs` inline in
  the HTTP handler on every scrape; a hung `timedatectl` leaks handler
  goroutines. Add a context/TTL cache off the request path.
- `cmd/lyrebird-stream/monitors.go`: stall-detector per-device state
  (`stallCount`/`prevBytes`) is not reset when a device is removed by
  reload/failed-recovery → spurious restart/warnings after a `systemctl reload`.
- `cmd/lyrebird-stream/maintenance.go`: the systemd watchdog is an unconditional
  ticker (fed even if the daemon is logically wedged) — gate it on a real health
  probe.
- `cmd/lyrebird-stream/main.go`: a `Manager` (with an open log fd) is leaked when
  `sup.Add` fails on a duplicate during a poller/reload race.
- `diagnostics`: external-command checks have no per-check timeout; the disk
  check only inspects `/`, missing a full `/var`.
- `updater`: `arm` asset selection uses `strings.Contains`, matching `arm64`
  (wrong binary on 32-bit Pi); missing-checksum installs unverified.
- `stream`: supervisor backoff config fields are dead; per-stream backoff has no
  jitter (thundering herd on correlated failures).
- `audio`: `RecommendSettings` can recommend an unsupported channel count;
  `parseStreamFile` may collect playback (OUT) formats as capture caps
  (advisory `detect` path only).
- `cmd/lyrebird`: `test`/`diagnose`/`check-system` return exit 0 even on failure
  (breaks the advertised scripting use); `status` misreports a live root-owned
  stream as "stale" when run non-root (EPERM treated as not-running); the
  diagnostic bundle is world-readable (0644) despite containing config + logs.
- `menu`: TTY detection uses `input != os.Stdin` instead of `isatty`; `RunCommand`
  leaves stdin nil so the "Edit Config" editor can't run.
- `config`: `TestKoanfConfig_Watch` is timing-flaky under full-suite load;
  `Watch()` leaks the fsnotify watcher on ctx cancel (the "no Stop()" comment is
  wrong — `Unwatch()` exists); millisecond-collision backups are invisible to
  `ListBackups`/`CleanOldBackups`; `RestoreBackup` writes non-atomically.

### LOW (resolved)

- Unbounded `io.ReadAll` on HTTP/download bodies → bounded with `io.LimitReader`
  (mediamtx error bodies, updater download cap, bundle reads) `2d75078`.
- `url.PathEscape` parity gap in `mediamtx.GetPath` → escaped `2d75078`.
- Resource monitor never computed CPU% → delta-based CPU% via jiffies,
  USER_HZ=100, with a pure unit-tested helper `7f42290`.
- Environment-coupled / assertion-free tests → `TestRun` split into
  deterministic routing + env-independent dispatch checks; `--bundle` and
  diagnose/check-system "output" tests now assert real behavior `224de53`.
- CI actions not SHA-pinned → pinned to commit SHAs with version comments
  `086d29d`.

A full per-file list with file:line references is available in the review
working notes.

---

## Verification status

`go build`, `go vet`, `gofmt -s`, `staticcheck`, `gosec`, `golangci-lint` v2
(incl. `-tags integration,e2e`), and `govulncheck` are all clean;
`go test -race ./...` passes; and the `-tags e2e` harness passes against real
MediaMTX v1.19.2 + ffmpeg.
