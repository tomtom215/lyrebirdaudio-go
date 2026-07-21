# Project Chronology

Development timeline for LyreBirdAudio-Go, tracking major milestones, audit results, and quality improvements.

---

## Phase 1-2: Core Implementation

**Period**: Initial development through 2026-02-19

- Core audio detection (`internal/audio`) — 97.6% coverage
- Configuration system (`internal/config`) — koanf + YAML + env vars
- Stream manager (`internal/stream`) — FFmpeg lifecycle with backoff
- File locking (`internal/lock`) — flock(2) based
- udev rule generation (`internal/udev`) — byte-for-byte bash compatible
- Supervisor tree (`internal/supervisor`) — Erlang-style using suture v4
- CLI commands: devices, detect, usb-map, migrate, validate, status, setup, install-mediamtx, diagnose, check-system
- Streaming daemon (`cmd/lyrebird-stream`) — production-ready with SIGHUP reload
- systemd service template with 18 security hardening directives

---

## Phase 3: Sonnet 4.6 Peer Review

**Date**: 2026-02-19
**Branch**: `claude/code-review-audit-ceC8S`
**Reviewer**: Claude Sonnet 4.6

### Results
- **59 issues identified** across 6 tiers (CRITICAL through CI/CD)
- **59/59 fixed** in 3 implementation phases
- Coverage: 71.8% → 73.7%

### Critical Fixes (C-1 through C-5)
| ID | Issue | Impact |
|----|-------|--------|
| C-1 | Lock theft for streams running > 5 minutes | Data corruption: two managers on one device |
| C-2 | registeredServices map race condition | Concurrent map read/write panic |
| C-3 | nil koanfCfg dereference in poll goroutine | Daemon crash on config load failure |
| C-4 | Supervisor cancel race | Goroutine leak on shutdown |
| C-5 | cmd.Start() failure not checked | Zombie FFmpeg processes |

### Major Fixes (M-1 through M-6)
- M-1: `errors.Is` for wrapped context errors
- M-2: WatchdogSec removed (not implementable)
- M-3: Health endpoint added at :9998
- M-4: Device polling unconditional (hotplug support)
- M-5: manager.Close() in defer (fd leak)
- M-6: Config hash for SIGHUP change detection

### Documentation
- Full report: `docs/PEER_REVIEW.md`

---

## Phase 4: Opus 4.6 Deep Audit (Session 1)

**Date**: 2026-02-20
**Branch**: `claude/peer-review-audit-Y9PPD`
**Auditor**: Claude Opus 4.6

### Bugs Found and Fixed
| ID | File | Issue | Severity |
|----|------|-------|----------|
| BUG-1 | updater.go:761 | `FormatUpdateInfo` inverted `IsZero()` — printed "Published: 0001-01-01" for zero times | MAJOR |
| BUG-2 | updater.go:376-387 | `Update()` rollback defer captured wrong `err` scope — backup rollback was dead code | MAJOR |
| BUG-3 | diagnostics.go:591 | `checkMediaMTXAPI` ignored context parameter | LOW |

### Documentation Fixes
- CLAUDE.md coverage table: all 14 entries updated (up to 7.5% stale)
- Coverage threshold text: "70%" corrected to "65%"

### Verification
- All 59 prior peer-review fixes confirmed correctly implemented
- Full report: `docs/OPUS_AUDIT_REPORT.md`

---

## Phase 5: Opus 4.6 Security Audit (Session 2)

**Date**: 2026-02-20
**Branch**: `claude/peer-review-audit-Y9PPD`
**Auditor**: Claude Opus 4.6

### Focus: Permissions, Ownership, Least Privilege

### Security Fixes
| ID | File(s) | Issue | Fix |
|----|---------|-------|-----|
| SEC-1 | cmd/lyrebird-stream/main.go | Health endpoint on all interfaces | Bind to `127.0.0.1:9998` |
| SEC-2 | internal/lock/filelock.go | Lock dir `0755`, files `0644` | Dir `0750`, files `0640` |
| SEC-3 | internal/config/config.go | Config save `0644` | Save `0640` |
| SEC-4 | internal/config/backup.go | Backup dir `0755`, restore `0644` | Dir `0750`, restore `0640` |
| SEC-5 | cmd/lyrebird/main.go | MediaMTX version not validated | Regex validation added |

### Tests Added: 10
- Lock permission tests (3)
- Config save permission test (1 updated)
- Backup/restore permission tests (4)
- Version validation tests (2)

### Documentation Created
- `docs/SECURITY_AUDIT.md` — full security audit report
- `docs/CHRONOLOGY.md` — this file
- `docs/LESSONS_LEARNED.md` — patterns and anti-patterns
- `docs/SESSION_SETUP_INSTRUCTIONS.md` — AI session guide
- CLAUDE.md reorganized with table of contents

### Full Report: `docs/SECURITY_AUDIT.md`

---

## Phase 6: Production Readiness Audit (Session 3)

**Date**: 2026-02-20
**Branch**: `claude/verify-previous-findings-JZo7J`
**Auditor**: Claude Opus 4.6

### Focus: Verify 15 production-readiness findings, fix all verified issues

### Verification Results
| Finding | Claim | Verified |
|---------|-------|----------|
| P-1 | MediaMTX API client is dead code | **TRUE** |
| P-2 | No silent stream detection | **TRUE** (code exists in dead client) |
| P-3 | Max restart = permanent death | **TRUE** (50 failures → StateFailed forever) |
| P-4 | Health endpoint nil provider | **TRUE** (always returns 503) |
| P-5 | Config backup never called | **TRUE** (zero production callers) |
| P-6 | No config validation before reload | **FALSE** (Load() calls Validate()) |
| P-7 | USB stabilization delay unused | **TRUE** (defined but never referenced) |
| P-8 | No Prometheus metrics | **TRUE** |
| P-9 | No syslog/remote logging | **TRUE** |
| P-10 | No resource limits | **PARTIAL** (LimitNOFILE exists, no MemoryMax) |
| P-11 | No diagnostic bundle export | **TRUE** |
| P-12 | No field technician runbook | **TRUE** |
| P-13 | Menu not populated | **FALSE** (7 main + 13 submenu items wired) |
| P-14 | No checksum verification | **TRUE** |
| P-15 | Daemon test coverage 32.7% | **TRUE** |

### Fixes Applied (7 code changes)
| Fix | Finding(s) | Description |
|-----|-----------|-------------|
| P-1/P-2 fix | P-1, P-2 | Wire MediaMTX client into daemon with stream health check loop (stall detection via bytes_received) |
| P-3 fix | P-3 | Add periodic recovery goroutine — clears failed stream registrations so device polling re-registers with fresh backoff |
| P-4 fix | P-4 | Implement `supervisorStatusProvider` that queries supervisor for live service states, replaces nil provider |
| P-5 fix | P-5 | Wire `BackupConfig()` into CLI migrate and setup commands before config save |
| P-7 fix | P-7 | Apply `USBStabilizationDelay` wait in `registerDevices` before creating stream managers |
| P-10 fix | P-10 | Add `MemoryHigh=384M` and `MemoryMax=512M` to systemd service file |
| P-14 fix | P-14 | Add SHA256 hash computation and verification against official `checksums.sha256` from MediaMTX GitHub releases |

### Tests Added: 10
- `TestSupervisorStatusProvider_NoServices` — P-4 empty supervisor
- `TestSupervisorStatusProvider_WithServices` — P-4 with registered services
- `TestSupervisorStatusProvider_HealthyMapping` — P-4 running→healthy mapping
- `TestSupervisorStatusProvider_FailedService` — P-4 failed→unhealthy+error
- `TestSupervisorStatusProvider_ImplementsInterface` — P-4 compile-time check
- `TestVerifyDownloadIntegrity` valid/empty/nonexistent — P-14 (3 subtests)
- `TestVerifyChecksumFile` match/mismatch/missing/nonexistent/case — P-14 (5 subtests)

### Findings Not Fixed (with rationale)
| Finding | Rationale |
|---------|-----------|
| P-6 | Claim was **FALSE** — validation already exists |
| P-8 | Prometheus metrics: significant new feature, not a bug fix |
| P-9 | Remote logging: operational concern; systemd journal captures stderr |
| P-11 | Diagnostic bundle export: nice-to-have, not critical |
| P-12 | Field technician runbook: documentation task, not code |
| P-13 | Claim was **FALSE** — menu IS fully wired with 20 items |
| P-15 | Daemon test coverage: integration tests need real FFmpeg/MediaMTX |

### Coverage Impact
- cmd/lyrebird: 48.5% → 49.2% (improved)
- cmd/lyrebird-stream: 32.7% → 26.3% (decreased due to new untestable goroutine code)
- All internal packages: unchanged (87%+ average)

### Full Report: Inline in session transcript

---

## Phase 7: Opus 4.8 Field-Reliability Hardening (Session 4)

**Date**: 2026-07-19
**Branch**: `claude/bioacoustics-hardening-6j7t86`
**Focus**: Latent 24/7/365 field-reliability bugs, dependency currency, expanded E2E.

Deep audit of the full daemon/stream reliability path (manual review + two parallel
adversarial surveys). The codebase was already unusually well-hardened; the prime
suspects (backoff overflow, timer/goroutine/fd leaks, config validate-before-swap,
MediaMTX request timeouts, flock stale detection) were all verified sound. Fixes
landed for the genuine gaps:

### Fixed
- **CRITICAL — local recording never worked (missing `-map` on the tee).** The
  `local_record_dir` feature builds an ffmpeg `-f tee` output feeding both the RTSP
  publish and the segment recorder, but never passed `-map`. The tee muxer does not
  do ffmpeg's automatic stream selection, so ffmpeg mapped zero streams and aborted
  with "Output file does not contain any stream" before either slave opened — every
  start failed, taking down BOTH the recording and the live stream in a crash/backoff
  loop. Latent because nothing drove the real tee command end-to-end; the new
  `TestE2E_LocalRecordingTee` (real ffmpeg + MediaMTX) exposed it. Fixed by mapping
  the audio stream explicitly (`-map 0:a`) before `-f tee`. (`internal/stream/process.go`)
- **HIGH — RTSP published over UDP; tee RTSP slave carried invalid options.** Driving
  the real tee end-to-end (once `-map` let it run) surfaced two more issues on the
  same path: both RTSP publish paths used ffmpeg's default UDP transport (RTSP-over-UDP
  can silently drop/reorder RTP even on localhost, corrupting research audio and
  leaving MediaMTX not marking the publisher "ready"), and the tee's RTSP slave carried
  `reconnect*` protocol options in a muxer-option position where they are meaningless
  and perturb muxer setup. Fixed: publish over TCP (`rtsp_transport=tcp`) on both paths
  for lossless in-order delivery, and drop the bogus reconnect options from the tee
  slave (recovery there is the manager's backoff restart). A `RealtimeInput` opt-in was
  also added so a synthetic (lavfi) source can be `-re`-paced for a healthy live publish
  without affecting hardware ALSA capture. (`internal/stream/process.go`, `manager.go`)
- **HIGH — default recording codec/container pairing was invalid.** Verified
  directly against ffmpeg 7.x that, among the supported segment formats, opus muxes
  ONLY into ogg and aac ONLY into wav (opus+wav, opus+flac, aac+ogg, aac+flac all
  fail ffmpeg's container check). The default was `opus` codec + `wav` segment format
  — an impossible pairing that, with the new `onfail=ignore`, would silently record
  nothing. Fixed the default to `ogg` (matching the default opus codec) and added
  load-time validation that rejects an incompatible codec×segment_format when local
  recording is enabled, with an error naming the required format — so a misconfig
  fails loudly at startup instead of losing research recordings silently.
  (`internal/config/config.go`)
- **HIGH — USB re-enumeration strands a device for hours.** The daemon pinned
  `hw:<card>,0` at registration and keyed the registry on device *name*, so a mic
  that returned on a different ALSA card number (unplug/replug, hub reset, USB bus
  reset) kept the manager driving the stale card until ~50 backoff attempts (hours)
  plus the 5-minute failed-stream recovery rebuilt it — and could stream a different
  device under the old name. The poller now tracks each stream's card number and
  restarts on change within one poll (~seconds). (`cmd/lyrebird-stream/main.go`)
- **HIGH — a full recording disk killed the live RTSP stream.** FFmpeg's `tee`
  muxer defaults to `onfail=abort`, so a failing segment write aborted the whole
  process, dropping the monitored live stream. Added `onfail=ignore` to the segment
  slave; the RTSP slave keeps `onfail=abort` for fast restart on publish failure.
  (`internal/stream/process.go`)
- **MEDIUM — a panic in any daemon background loop crashed the whole process**,
  dropping every stream. Added `runSupervised`, a recover-and-restart wrapper, and
  wired the six long-lived loops (poller, reload, stall detector, failed-stream
  recovery, segment retention, disk monitor) through it. (`cmd/lyrebird-stream`)
- **LOW — ResourceMonitor leaked one map entry per FFmpeg PID** (dormant: monitoring
  is off by default). `MonitorProcess` now prunes its pid on exit.
  (`internal/stream/monitor.go`)
- **LOW — ffmpeg log-rotation failures were silently discarded**; the manager now
  wires `WithRotateLogger` so a full log disk is visible. (`internal/stream/manager.go`)
- **Test robustness** — `TestRunCheckSystemSmoke` asserted an environment-dependent
  outcome; now asserts the error correlates with actual ffmpeg presence.
- **Dependencies** — `go-isatty` v0.0.23, `x/sys` v0.47.0, `x/text` v0.40.0; govulncheck clean.

### Expanded tests
- New E2E `TestE2E_LocalRecordingTee` drives the real `stream.Manager` (tee →
  live MediaMTX + local ogg segments), the regression guard proving the `onfail=ignore`
  tee syntax is valid ffmpeg and that segments are written while publishing.
- New unit tests for the card-number-change restart, `runSupervised` panic recovery,
  ResourceMonitor pruning, and the tee `onfail` guard.

### Flagged for verification (not changed)
- **Default `segment_format: wav` with the default `opus` codec is a likely
  incompatible codec/container pairing** (opus muxes into ogg, not WAV/FLAC). Left
  unchanged pending ffmpeg verification; the `onfail=ignore` fix now prevents such a
  segment-init failure from crash-looping the live stream. Recommend confirming the
  codec×segment_format matrix and validating it at config load.

### Verification
`gofmt`, `go vet`, `golangci-lint` (0 issues), `gosec`, `govulncheck` (no vulns), and
`go test -race ./...` all clean. Coverage flat-to-improved (cmd/lyrebird-stream
78.7%→82.5%).

---

## Phase 8: Fable 5 Adversarial Reliability Audit (Session 5)

**Date**: 2026-07-21
**Branch**: `claude/lyrebird-reliability-audit-92322q`
**Focus**: Long-horizon field chaos — identity stability across polls, clock
discontinuities, resource exhaustion over restart cycles, fault-injection E2E,
32-bit smoke testing. All fixes TDD (failing test first).

### Fixed
- **HIGH — unstable device identity registered unbounded streams.**
  `SanitizeDeviceName` falls back to a TIMESTAMPED `unknown_device_<unix>`
  name (bash-compatible) for raw names that sanitize to nothing — e.g. a fully
  non-ASCII or symbols-only USB product string. The daemon keyed its stream
  registry on that name, so every 10 s poll computed a NEW identity for the
  same physical device and registered another stream service: unbounded growth
  of managers, lock files, goroutines and failing FFmpeg processes, plus a
  config/MediaMTX path that changed every poll. Added
  `audio.Device.StableName()` (substitutes deterministic
  `usb_<vendor>_<product>` when sanitization hit the timestamped fallback);
  daemon registration now uses it. (`internal/audio/detector.go`,
  `cmd/lyrebird-stream/main.go`)
- **MEDIUM — FFmpeg stderr logging silently lost after a supervisor re-run.**
  `streamService` closes the manager after every `Run` return, but suture
  re-runs the SAME service after an error (e.g. a lock-acquire timeout while a
  competing process briefly holds the device lock). The re-run streamed with a
  nil log writer forever — months of unattended operation with empty FFmpeg
  logs. `Manager.Run` now reopens the rotating writer; a reopen failure is
  logged and tolerated so a log-disk problem can never take down the stream.
  (`internal/stream/manager.go`)
- **MEDIUM — retention deleted pre-clock-sync recordings.** A no-RTC Pi boots
  near the Unix epoch; segments recorded before NTP sync carry ~1970 mtimes.
  After NTP steps the clock forward, the hourly retention pass computed a
  "55-year" age and deleted the only copy of audio captured while offline.
  Segments stamped before a 2020-01-01 sanity floor are now exempt from
  AGE-based deletion (still first in line for the SIZE budget).
  (`cmd/lyrebird-stream/maintenance.go`)

### New test artifacts
- **Fuzz**: `FuzzParseUSBID`, `FuzzParseCPUJiffies` (+ sibling stat parsers on
  the same corpus), `FuzzStableNameDeterministic` — ~1M execs each, clean.
- **Fault-injection E2E** (real ffmpeg + MediaMTX): `TestE2E_FFmpegKillRecovery`
  (SIGKILL ffmpeg mid-publish twice; assert backoff re-publish + fd baseline
  after shutdown), `TestE2E_MediaMTXRestartRecovery` (kill/restart the server
  on the same ports mid-stream; assert recovery).
- **Soak**: `TestRegisterRemoveCyclesNoResourceLeak` — 12 register→run→remove
  cycles through the real supervisor/manager/flock/log-writer wiring; asserts
  fds and goroutines return to baseline (runs untagged, -race clean).

### Verified sound (no change needed)
- Stall-detector 404-vs-stall semantics (a departed publisher makes ffmpeg's
  TCP publish fail → exit → manager backoff restart covers it); flock
  single-gate acquire; koanf validate-before-swap; MediaMTX client timeouts;
  backoff jitter math; RotatingWriter close/rotate interleaving (unreachable
  in manager usage — writes are sequenced before Close).
- **32-bit**: full test suite passes compiled as GOARCH=386 (32-bit `int`);
  cross-compiles clean for linux/amd64, arm64, arm GOARM=7, arm GOARM=6.

### Residual risks (documented, accepted)
- A fake-hwclock offset of hours–days at boot can still mis-age recent
  segments (only the epoch-class discontinuity is guarded); a backwards NTP
  step larger than one segment duration could overwrite a segment via
  strftime filename collision.
- Two devices with identical names — or identical USB IDs and unusable
  names — collapse to one stream identity (pre-existing limitation).
- Stall-detector removal + poller re-registration resets a persistently
  failing device's backoff (~3 min cadence) — intended aggressive recovery,
  bounded.

### Verification
`gofmt -s`, `go vet`, `golangci-lint` (0 issues), `gosec`, `govulncheck` all
clean; `go test -race ./...` green; e2e green against real ffmpeg + MediaMTX
v1.19.2 including the new fault-injection tests; GOARCH=386 suite green;
`go mod tidy` a no-op. Coverage flat-to-improved (audio 97.6%→98.1%).

---

## Quality Metrics Over Time

| Metric | Phase 3 Start | Phase 3 End | Phase 5 End | Phase 6 End |
|--------|--------------|-------------|-------------|-------------|
| Total coverage | 71.8% | 73.7% | 73.7%+ | ~73% |
| Internal pkg avg | ~85% | ~87% | ~87% | ~87% |
| Open issues | 59 | 0 | 0 | 0 |
| Security findings | — | — | 0 (5 fixed) | 0 |
| Production gaps | — | — | — | 7 fixed |
| Race conditions | 0 | 0 | 0 | 0 |
| `go vet` warnings | 0 | 0 | 0 | 0 |

---

*Last updated: 2026-02-20*
