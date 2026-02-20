# CLAUDE.md — AI Session Guide for LyreBirdAudio-Go

> **Last updated**: 2026-02-20
> **Go Version**: 1.24+
> **Repository**: github.com/tomtom215/lyrebirdaudio-go

---

## Table of Contents

1. [Quick Reference](#quick-reference)
2. [Project Overview](#project-overview)
3. [Codebase Map](#codebase-map)
4. [Mandatory Standards](#mandatory-standards)
5. [Security Posture](#security-posture)
6. [Test Coverage Dashboard](#test-coverage-dashboard)
7. [Modular Documentation Index](#modular-documentation-index)
8. [Session Checklist](#session-checklist)

---

## Quick Reference

### Essential Commands
```bash
go test -race ./...              # Run all tests (MUST pass before commit)
go test -cover ./...             # Coverage report
go vet ./...                     # Static analysis
go build -o bin/lyrebird ./cmd/lyrebird  # Build CLI
go build -o bin/lyrebird-stream ./cmd/lyrebird-stream  # Build daemon
gofmt -s -w .                    # Format code
go mod tidy                      # Tidy modules
```

### Critical Rules (Non-Negotiable)
- **gofmt Before Commit**: Run `gofmt -s -w .` before every commit. CI will reject unformatted code. NEVER hand-align comments or struct fields with extra spaces — `gofmt` owns all whitespace decisions.
- **TDD Required**: Write tests FIRST, then implementation
- **Race-Free**: `go test -race ./...` must pass with zero warnings
- **Coverage Floor**: 65% minimum (CI-enforced), 90%+ target for internal packages
- **No TODO Comments**: Every test and fix must be complete
- **Error Wrapping**: Always `fmt.Errorf("context: %w", err)`
- **Context Propagation**: All async operations take `context.Context`

---

## Project Overview

LyreBirdAudio captures audio from USB microphones and streams via RTSP using FFmpeg and MediaMTX. Go port of the original bash implementation. Designed for 24/7/365 unattended operation at industrial reliability levels.

**Key architectural decisions**: See [docs/CHRONOLOGY.md](docs/CHRONOLOGY.md)
**Lessons from past sessions**: See [docs/LESSONS_LEARNED.md](docs/LESSONS_LEARNED.md)

---

## Codebase Map

```
cmd/lyrebird/          → CLI: devices, detect, usb-map, migrate, validate, status, setup, install-mediamtx, diagnose, check-system, update, menu
cmd/lyrebird-stream/   → Daemon: supervisor tree, signal handling (SIGINT/SIGTERM/SIGHUP), device polling, config hot-reload
internal/audio/        → USB audio detection via /proc/asound + device name sanitization (97.6%)
internal/config/       → YAML + koanf + env vars + hot-reload + backup/restore (92.0%)
internal/diagnostics/  → 24 system health checks (65.2%)
internal/health/       → HTTP health endpoint at 127.0.0.1:9998 (94.1%)
internal/lock/         → flock(2)-based file locking with stale detection (77.3%)
internal/mediamtx/     → MediaMTX REST API client (92.4%)
internal/menu/         → Interactive TUI menus via charmbracelet/huh (61.5%)
internal/stream/       → FFmpeg lifecycle + exponential backoff + state machine (87.1%)
internal/supervisor/   → Erlang-style supervisor tree via suture v4 (96.4%)
internal/udev/         → udev rule generation, byte-for-byte bash compatible (92.9%)
internal/updater/      → Self-update from GitHub releases + semver (89.5%)
internal/util/         → SafeGo panic recovery + resource tracking (94.1%)
systemd/               → lyrebird-stream.service (18 security hardening directives)
```

---

## Mandatory Standards

### TDD Workflow
1. Write test → 2. Watch it fail → 3. Write minimal code → 4. Refactor → 5. Repeat

### What Must Be Tested
| Category | Examples |
|----------|---------|
| Happy paths | Normal operation flows |
| Error paths | Every `if err != nil` branch |
| Boundary conditions | Empty, zero, max values |
| File system failures | Missing files, permission denied |
| Process failures | Command not found, crashes, signals |
| Network failures | Connection refused, timeouts |
| Concurrent access | Race conditions, deadlocks |
| Signal handling | SIGINT/SIGTERM/SIGHUP during operations |
| State transitions | Every valid and invalid state change |

### Code Conventions
- Table-driven tests with descriptive names
- `t.TempDir()` for all file operations in tests
- `atomic.Value` for state, `sync.RWMutex` for complex types
- Validate config at load time, not use time
- Mock external dependencies (ffmpeg, udev, MediaMTX)

---

## Security Posture

### File Permissions (Least Privilege)
| Resource | Mode | Rationale |
|----------|------|-----------|
| Lock directory | `0750` | Owner+group only; prevents PID enumeration |
| Lock files | `0640` | Owner+group read/write for service coordination |
| Config files | `0640` | May contain API URLs; no world-read |
| Config directories | `0750` | Restrict traversal to owner+group |
| Backup files | `0600` | Owner-only; contains full config data |
| Backup directories | `0750` | Restrict traversal to owner+group |
| systemd service files | `0644` | Standard (must be system-readable) |
| Log directories | `0750` | May contain sensitive device/URL info |

### Network Security
- Health endpoint binds to `127.0.0.1:9998` (localhost only)
- MediaMTX API defaults to `http://localhost:9997` (localhost only)
- Version strings validated with regex before URL construction

### Systemd Hardening (18 directives)
`NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true`, `PrivateTmp=true`,
`ProtectKernelTunables=true`, `ProtectKernelModules=true`, `ProtectControlGroups=true`,
`RestrictSUIDSGID=yes`, `RestrictNamespaces=yes`, `LockPersonality=yes`,
`MemoryDenyWriteExecute=yes`, `RestrictRealtime=yes`, `SystemCallFilter=@system-service`,
`SystemCallArchitectures=native`, `DevicePolicy=closed`, `DeviceAllow=/dev/snd/* rw`,
`ReadWritePaths=/var/run/lyrebird`, `ReadOnlyPaths=/etc/lyrebird /proc/asound`

### Audit Trail
- **Initial peer review**: [docs/PEER_REVIEW.md](docs/PEER_REVIEW.md) — 59 issues, all resolved
- **Opus deep audit**: [docs/OPUS_AUDIT_REPORT.md](docs/OPUS_AUDIT_REPORT.md) — 3 bugs found and fixed
- **Security audit**: [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) — Permissions hardening (SEC-1 through SEC-5)

---

## Test Coverage Dashboard

| Package | Coverage | Status |
|---------|----------|--------|
| internal/audio | 97.6% | ✅ Excellent |
| internal/supervisor | 96.4% | ✅ Excellent |
| internal/health | 94.1% | ✅ Excellent |
| internal/util | 94.1% | ✅ Excellent |
| internal/udev | 92.9% | ✅ Excellent |
| internal/mediamtx | 92.4% | ✅ Excellent |
| internal/config | 92.0% | ✅ Excellent |
| internal/updater | 89.5% | ✅ Good |
| internal/stream | 87.1% | ✅ Good |
| internal/lock | 77.3% | ⬜ Acceptable |
| internal/diagnostics | 65.2% | ⬜ Acceptable |
| internal/menu | 61.5% | ⬜ Acceptable (requires terminal) |
| cmd/lyrebird | 48.5% | ⬜ CLI (root/interactive) |
| cmd/lyrebird-stream | 32.7% | ⬜ Daemon (runtime env) |
| **Internal avg** | **~87%** | **Target: 90%+** |
| **Overall** | **~74%** | **CI min: 65%** |

---

## Modular Documentation Index

| Document | Purpose | Audience |
|----------|---------|----------|
| [README.md](README.md) | Project overview, installation, usage | End users, contributors |
| [docs/PEER_REVIEW.md](docs/PEER_REVIEW.md) | Initial code review (59 issues) | Reviewers, auditors |
| [docs/AUDIT_REPORT.md](docs/AUDIT_REPORT.md) | Pre-release assessment (100+ items) | Reviewers, auditors |
| [docs/OPUS_AUDIT_REPORT.md](docs/OPUS_AUDIT_REPORT.md) | Deep audit by Opus 4.6 (3 bugs fixed) | Reviewers, auditors |
| [docs/SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) | Permissions/privilege security audit | Security reviewers |
| [docs/CHRONOLOGY.md](docs/CHRONOLOGY.md) | Timeline of all changes and decisions | AI sessions, contributors |
| [docs/LESSONS_LEARNED.md](docs/LESSONS_LEARNED.md) | What worked, what didn't, patterns | AI sessions, contributors |
| [docs/SESSION_SETUP_INSTRUCTIONS.md](docs/SESSION_SETUP_INSTRUCTIONS.md) | How to start an effective session | AI assistants |

---

## Session Checklist

### Before Starting Work
- [ ] Read this file (CLAUDE.md)
- [ ] Check `docs/LESSONS_LEARNED.md` for relevant patterns
- [ ] Run `go test -race ./...` to verify clean baseline
- [ ] Check `git log --oneline -10` for recent context

### Before Committing
- [ ] `gofmt -s -l .` returns no files (run `gofmt -s -w .` to fix)
- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` clean
- [ ] Coverage not decreased (`go test -cover ./...`)
- [ ] No `// TODO` comments added
- [ ] Error messages include context
- [ ] New public functions have godoc comments
- [ ] Security: file permissions follow least-privilege table above

### After Session
- [ ] Update `docs/LESSONS_LEARNED.md` if new patterns discovered
- [ ] Update coverage table if numbers changed significantly
- [ ] Update `docs/CHRONOLOGY.md` with session summary

---

*This file is optimized for AI coding assistants. For human developers, see [README.md](README.md).*
