# Security Audit Report — Permissions, Ownership & Least Privilege

**Date**: 2026-02-20
**Auditor**: Claude Opus 4.6
**Scope**: Full codebase audit for file permissions, ownership, privilege levels, attack surface, network exposure
**Method**: Line-by-line source reading of all Go files + systemd service + CI workflow

---

## Executive Summary

A comprehensive security audit focused on least-privilege principles identified **5 actionable findings** across file permissions, network exposure, and input validation. All have been fixed, tested, and verified. The systemd service and CI workflow received separate targeted audits and scored A- and A respectively.

**Findings Fixed**: 5
**Tests Added**: 10
**Overall Security Posture**: Strong (post-fix)

---

## Findings Fixed

### SEC-1: Health Endpoint Exposed to Network (MEDIUM)

**File**: `cmd/lyrebird-stream/main.go:383`
**Was**: `health.ListenAndServe(ctx, ":9998", healthHandler)`
**Now**: `health.ListenAndServe(ctx, "127.0.0.1:9998", healthHandler)`

**Problem**: The health check endpoint bound to all network interfaces (`:9998`), exposing service status to the network. An attacker could enumerate stream names, device states, and uptime data remotely.

**Fix**: Bind to localhost only (`127.0.0.1:9998`). The health endpoint is designed for local monitoring (systemd, localhost probes) and has no legitimate network access requirement.

**Test**: Existing `TestListenAndServe` already uses `127.0.0.1:0`.

---

### SEC-2: Lock Directory and File Permissions Too Permissive (MEDIUM)

**File**: `internal/lock/filelock.go:64,99,184`
**Was**: Directory `0755`, files `0644`
**Now**: Directory `0750`, files `0640`

**Problem**: Lock directory was world-readable+executable (`0755`) and lock files were world-readable (`0644`). This allowed any user to:
- Enumerate device names from lock file names
- Read PIDs of streaming processes
- Infer system configuration details

**Fix**: Restrict both to owner+group only:
- Directory: `0750` (rwxr-x---)
- Files: `0640` (rw-r-----)

The daemon's lock directory creation at `cmd/lyrebird-stream/main.go:111` already used `0750`, so this change aligns the lock package with the daemon's intent (resolving LIMIT-3 from the prior audit).

**Tests Added**:
- `TestFileLockDirectoryPermissions` — verifies `0750`
- `TestFileLockFilePermissions` — verifies `0640` via `Acquire()`
- `TestFileLockFilePermissionsContext` — verifies `0640` via `AcquireContext()`

---

### SEC-3: Config File Save Permissions Too Permissive (MEDIUM)

**File**: `internal/config/config.go:178`
**Was**: `tmpFile.Chmod(0644)`
**Now**: `tmpFile.Chmod(0640)`

**Problem**: Saved configuration files were world-readable (`0644`). Configuration may contain sensitive information such as MediaMTX API URLs, RTSP server endpoints, and operational details.

**Fix**: Restrict to owner+group (`0640`). The service user and its group can still read the file; other users cannot.

**Test Updated**: `TestSaveConfigAtomicPermissions` now verifies exact `0640` instead of `>= 0644`.

---

### SEC-4: Backup Permissions Inconsistency (MEDIUM)

**File**: `internal/config/backup.go:70,214,220`
**Was**: Backup directory `0755`, config directory `0755`, restored config `0644`
**Now**: Backup directory `0750`, config directory `0750`, restored config `0640`

**Problem**: Inconsistent permissions across backup operations:
- Backup directory was `0755` (world-readable)
- Restored configs were `0644` (world-readable), less restrictive than backup files (`0600`)
- Config parent directory was `0755` when created during restore

**Fix**: Apply consistent least-privilege permissions:
- Backup directory: `0750` (match config directory)
- Config parent directory: `0750` (match daemon convention)
- Restored config files: `0640` (match SEC-3 config save)
- Backup files: Already `0600` (no change needed)

**Tests Added**:
- `TestBackupDirectoryPermissions` — verifies `0750`
- `TestBackupFilePermissions` — verifies `0600` (confirms unchanged)
- `TestRestoreBackupPermissions` — verifies restored config is `0640`
- `TestRestoreConfigDirectoryPermissions` — verifies parent dir is `0750`

---

### SEC-5: MediaMTX Version String Not Validated (MEDIUM)

**File**: `cmd/lyrebird/main.go:1149-1152`
**Was**: No validation on `--version` flag
**Now**: Regex validation requiring `vX.Y.Z` or `X.Y.Z` format

**Problem**: The `--version` flag for `install-mediamtx` was used directly in a download URL:
```go
downloadURL := fmt.Sprintf("https://github.com/.../download/%s/mediamtx_%s_linux_%s.tar.gz", version, version, arch)
```
Malicious input like `v1.9.3/%2e%2e/` could cause URL path traversal.

**Fix**: Added `isValidMediaMTXVersion()` that validates against `^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`. Invalid versions are rejected before any network request.

**Tests Added**:
- `TestIsValidMediaMTXVersion` — 15 cases covering valid versions, injection attempts, path traversal, command injection patterns
- `TestInstallMediaMTXVersionValidation` — integration test verifying rejection propagation

---

## Systemd Service Security Assessment

**File**: `systemd/lyrebird-stream.service`
**Grade**: A- (90/100)

### Hardening Directives Present (18 total)
- `NoNewPrivileges=true` — prevents privilege escalation
- `ProtectSystem=strict` — read-only filesystem except explicit paths
- `ProtectHome=true` — home directories inaccessible
- `PrivateTmp=true` — private /tmp namespace
- `ProtectKernelTunables=true` — hides /proc/sys
- `ProtectKernelModules=true` — prevents module loading
- `ProtectControlGroups=true` — protects cgroup hierarchy
- `RestrictSUIDSGID=yes` — blocks SUID/SGID
- `RestrictNamespaces=yes` — restricts namespace operations
- `LockPersonality=yes` — prevents personality changes
- `MemoryDenyWriteExecute=yes` — blocks W+X memory (anti-shellcode)
- `RestrictRealtime=yes` — prevents RT scheduling abuse
- `SystemCallFilter=@system-service` — syscall allowlist
- `SystemCallArchitectures=native` — blocks foreign arch code
- `DevicePolicy=closed` — denies unlisted devices
- `DeviceAllow=/dev/snd/* rw` — explicit sound device whitelist
- `ReadWritePaths=/var/run/lyrebird` — minimal writable paths
- `ReadOnlyPaths=/etc/lyrebird /proc/asound` — explicit read-only

### Notes
- Runs as root by default; migration path to dedicated user documented in service file
- Resource limits configured: `LimitNOFILE=65536`, `LimitNPROC=4096`
- Rate limiting: max 5 restarts in 300 seconds

---

## CI Workflow Security Assessment

**File**: `.github/workflows/ci.yml`
**Grade**: A (95/100)

### Strengths
- **Least-privilege permissions**: `contents: read`, `pull-requests: read`, `checks: write`
- **Release job elevated only on tags**: `contents: write` gated by `startsWith(github.ref, 'refs/tags/')`
- **Codecov action pinned to SHA** (not tag) — prevents tag spoofing
- **All tools version-pinned**: golangci-lint v1.62.2, gosec v2.21.4, govulncheck v1.1.3
- **No unsafe script injection**: All shell commands use literal strings
- **No secret exposure**: Only `GITHUB_TOKEN`, scoped to release job

---

## Remaining Accepted Risks (Not Bugs)

These items were evaluated and determined to be acceptable trade-offs:

| Item | Risk | Mitigation |
|------|------|-----------|
| MediaMTX API over HTTP | Unencrypted on localhost | Localhost-only by default; network access requires explicit config |
| Hardcoded health port 9998 | Port conflict possible | Future: make configurable via flag/env |
| `@system-service` syscall filter | Broader than necessary | Low risk; `MemoryDenyWriteExecute` mitigates RWX attacks |
| koanf Watch() goroutine leak | Goroutine not stoppable | Daemon uses SIGHUP instead; documented |
| Config stored in plaintext | No encryption at rest | Mitigated by file permissions (0640) + ProtectSystem=strict |

---

## Permission Matrix (Post-Fix)

| Resource | Permission | Rationale |
|----------|-----------|-----------|
| Lock directory | `0750` | Owner+group: daemon and monitoring |
| Lock files | `0640` | Owner+group: prevent PID enumeration |
| Config files (save) | `0640` | Owner+group: may contain sensitive URLs |
| Config files (restore) | `0640` | Consistent with save |
| Backup directory | `0750` | Owner+group: contains config copies |
| Backup files | `0600` | Owner only: maximum restriction for backup data |
| Log directory | `0750` | Owner+group: daemon and log rotation |
| systemd service file | `0644` | Standard: systemd requires world-readable |
| Health endpoint | localhost:9998 | Loopback only: no network exposure |

---

## Verification

| Check | Result |
|-------|--------|
| `go test -race ./...` | ALL PASS (14/14 packages) |
| `go vet ./...` | CLEAN |
| New permission tests | ALL PASS (10 tests) |
| Total coverage | 73.7%+ |
| CI threshold (65%) | MET |

---

*Audit conducted 2026-02-20 by Claude Opus 4.6*
