# Session Setup Instructions

Instructions for AI assistants and contributors to quickly orient and be productive with this codebase.

---

## Quick Start

```bash
# 1. Verify Go version (1.24+ required)
go version

# 2. Run tests to verify environment
go test ./...

# 3. Run with race detector
go test -race ./...

# 4. Check coverage
go test -cover ./...

# 5. Run linters
go vet ./...
```

---

## Mandatory Reading Order

For AI sessions or new contributors, read in this order for fastest ramp-up:

1. **`CLAUDE.md`** — Primary codebase guide (ToC at top, quick reference, TDD requirements)
2. **`docs/LESSONS_LEARNED.md`** — Critical patterns and anti-patterns to avoid
3. **`docs/SECURITY_AUDIT.md`** — Permission matrix and security decisions
4. **`docs/CHRONOLOGY.md`** — What was done when, and by whom

### Only If Needed
- `docs/PEER_REVIEW.md` — Original 59-issue peer review (all closed)
- `docs/OPUS_AUDIT_REPORT.md` — Deep audit bugs (all fixed)
- `docs/AUDIT_REPORT.md` — Initial pre-release assessment

---

## Key Rules

### TDD Is Non-Negotiable
Every production code change requires tests written FIRST. No exceptions.
See CLAUDE.md section "Strict Test-Driven Development (TDD)".

### Permission Standards
All new files must follow the permission matrix in `docs/SECURITY_AUDIT.md`:
- Directories: `0750` (not `0755`)
- Config/lock files: `0640` (not `0644`)
- Backup files: `0600`
- Network listeners: `127.0.0.1` by default (not `0.0.0.0`)

### Always Verify After Changes
```bash
go test -race ./...   # Must pass with 0 races
go vet ./...           # Must be clean
go test -cover ./...   # Coverage must not decrease
```

---

## Common Tasks

### Adding a New Internal Package
1. Create `internal/newpkg/newpkg.go`
2. Create `internal/newpkg/newpkg_test.go` with tests FIRST
3. Implement minimum code to pass tests
4. Target 90%+ coverage
5. Run `go test -race ./internal/newpkg/...`

### Fixing a Bug
1. Write a failing test that reproduces the bug
2. Verify the test fails for the right reason
3. Fix the bug with minimal code change
4. Verify the test passes
5. Run full suite: `go test -race ./...`

### Adding a CLI Command
1. Add case to `run()` switch in `cmd/lyrebird/main.go`
2. Implement handler function
3. Add test cases to `cmd/lyrebird/main_test.go`
4. If requires root: add `os.Geteuid()` check

### Updating Coverage Table
After significant test changes, run:
```bash
go test -cover ./... 2>&1 | grep -E "^ok" | sort
```
Update the coverage table in CLAUDE.md with actual values.

---

## Architecture Quick Reference

```
cmd/lyrebird/        → CLI tool (setup, detect, validate, etc.)
cmd/lyrebird-stream/ → Daemon (runs 24/7, manages FFmpeg streams)
internal/audio/      → USB device detection (/proc/asound)
internal/config/     → YAML + koanf + env vars + hot-reload
internal/lock/       → flock(2) file locking
internal/stream/     → FFmpeg process lifecycle + backoff
internal/supervisor/ → Erlang-style supervisor tree (suture)
internal/udev/       → udev rule generation
internal/health/     → HTTP health endpoint
internal/mediamtx/   → MediaMTX API client
internal/updater/    → Self-update from GitHub releases
internal/util/       → Panic recovery, resource tracking
```

---

## Environment Requirements

| Dependency | Required | Purpose |
|------------|----------|---------|
| Go 1.24+ | Build | koanf file watching via fsnotify |
| FFmpeg | Runtime | Audio encoding |
| MediaMTX | Runtime | RTSP server |
| Linux 4.4+ | Runtime | ALSA, udev, flock(2) |
| systemd | Optional | Service management |

---

## Session Checklist

Before ending any development session, verify:

- [ ] All tests pass: `go test -race ./...`
- [ ] No vet warnings: `go vet ./...`
- [ ] Coverage not decreased: `go test -cover ./...`
- [ ] New files use correct permissions (see SECURITY_AUDIT.md)
- [ ] New network listeners bind to localhost
- [ ] CLAUDE.md coverage table updated if coverage changed
- [ ] Changes committed with descriptive message

---

*Last updated: 2026-02-20*
