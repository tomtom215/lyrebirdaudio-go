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

## Quality Metrics Over Time

| Metric | Phase 3 Start | Phase 3 End | Phase 5 End |
|--------|--------------|-------------|-------------|
| Total coverage | 71.8% | 73.7% | 73.7%+ |
| Internal pkg avg | ~85% | ~87% | ~87% |
| Open issues | 59 | 0 | 0 |
| Security findings | — | — | 0 (5 fixed) |
| Race conditions | 0 | 0 | 0 |
| `go vet` warnings | 0 | 0 | 0 |

---

*Last updated: 2026-02-20*
