# Lessons Learned

Patterns, anti-patterns, and hard-won knowledge from developing and auditing LyreBirdAudio-Go.
This document is intended to improve session-over-session quality and prevent regression.

---

## Table of Contents

1. [Go-Specific Pitfalls](#go-specific-pitfalls)
2. [Security Patterns](#security-patterns)
3. [Testing Patterns](#testing-patterns)
4. [Concurrency Patterns](#concurrency-patterns)
5. [Architecture Decisions](#architecture-decisions)
6. [What Worked Well](#what-worked-well)
7. [What To Watch For](#what-to-watch-for)

---

## Go-Specific Pitfalls

### LL-0: Always Run `gofmt -s -w .` Before Committing

**CI Failure (Phase 2)**: Hand-aligned inline comments with extra spaces caused CI to reject the commit. `gofmt` has strict rules about comment alignment.

**Pattern**: ALWAYS run `gofmt -s -w .` as the last step before `git add`. Never hand-align comments, struct fields, or table-driven test data — `gofmt` owns all whitespace decisions.

**Anti-pattern**: Aligning comments for readability:
```go
// WRONG — gofmt will reject this
{"v1.9", false},           // missing patch
{"v1", false},             // missing minor+patch
```

```go
// RIGHT — let gofmt handle spacing
{"v1.9", false}, // missing patch
{"v1", false},   // missing minor+patch
```

**Root cause**: AI assistants (including Claude) naturally try to align comments for visual consistency, but Go's formatter uses its own rules. This mismatch is easy to miss without running `gofmt -s -l .` before committing.

---

### LL-1: Closure Variable Capture in `if` Blocks

**BUG-2 (Opus Audit)**: A `defer` inside an `if _, err := ...; err == nil { }` block captured the block-scoped `err` (always nil), not the function's return error. The rollback mechanism was dead code.

**Pattern**: When a `defer` needs to observe a function's return value, always use named returns:
```go
func DoSomething() (retErr error) {
    defer func() {
        if retErr != nil {
            // cleanup
        }
    }()
    // ...
}
```

**Anti-pattern**: Placing `defer` inside an `if` block with short variable declaration:
```go
if _, err := os.Stat(path); err == nil {
    defer func() {
        if err != nil { // BUG: this is always nil!
            restore()
        }
    }()
}
```

---

### LL-2: Inverted Boolean Conditions

**BUG-1 (Opus Audit)**: `info.PublishedAt.IsZero()` when it should be `!info.PublishedAt.IsZero()`. Printed zero date for zero times, omitted real dates.

**Pattern**: Always test both branches of boolean conditions. If a function has `if condition { doA() } else { doB() }`, write a test that triggers each branch.

---

### LL-3: `errors.Is` for Wrapped Context Errors

**M-1 (Peer Review)**: `err == context.Canceled` fails when the error is wrapped. Always use `errors.Is(err, context.Canceled)`.

**Pattern**: Never compare errors with `==`. Always use `errors.Is()` or `errors.As()`.

---

## Security Patterns

### LL-4: File Permissions — Least Privilege by Default

**SEC-2, SEC-3, SEC-4**: Multiple locations used `0755` directories and `0644` files when the data didn't need world access.

**Pattern**: Use this permission table as a baseline:
| Resource | Permission | When |
|----------|-----------|------|
| Directories with sensitive data | `0750` | Config, lock, backup, log dirs |
| Config files | `0640` | May contain URLs, settings |
| Backup files | `0600` | Contain full config copies |
| systemd service files | `0644` | Must be world-readable |
| Lock files | `0640` | Contain PIDs |

**Anti-pattern**: Using `0755` / `0644` as "safe defaults" without analyzing what data is exposed.

---

### LL-5: Network Binding — Localhost by Default

**SEC-1**: Health endpoint bound to `0.0.0.0:9998`, exposing service status to the network.

**Pattern**: Always bind to `127.0.0.1` for monitoring/health endpoints. Make the bind address configurable for environments that need network access.

---

### LL-6: Input Validation Before URL Construction

**SEC-5**: Version string from `--version` flag was used directly in a GitHub download URL without validation.

**Pattern**: Validate all user input against a strict regex before incorporating it into URLs, file paths, or command arguments. Even if the input seems trustworthy (CLI flag), validate it.

```go
var validVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)
```

---

### LL-7: Context Propagation for HTTP Requests

**BUG-3 (Opus Audit)**: `client.Get(url)` ignores the `ctx` parameter. Always use `http.NewRequestWithContext()`.

**Pattern**: Every HTTP request must use `http.NewRequestWithContext(ctx, ...)`:
```go
req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
if err != nil { return err }
resp, err := client.Do(req)
```

---

## Testing Patterns

### LL-8: Test Both Branches of Every Conditional

BUG-1 was not caught because the test only exercised the zero-time branch. The non-zero branch was never tested.

**Pattern**: For every `if/else` or `if condition`, write at least two test cases — one that enters the block and one that skips it.

---

### LL-9: Test File Permissions Explicitly

**Pattern**: After any file creation, verify the exact permission mode:
```go
info, _ := os.Stat(path)
perm := info.Mode().Perm()
if perm != 0640 {
    t.Errorf("permissions = %04o, want 0640", perm)
}
```

---

### LL-10: Test Error Messages, Not Just Error Existence

**Pattern**: `if err == nil { t.Error("expected error") }` is insufficient. Always check the error message contains the expected substring:
```go
if !strings.Contains(err.Error(), "invalid version") {
    t.Errorf("error = %v, want containing 'invalid version'", err)
}
```

---

### LL-11: Table-Driven Tests for Validation Functions

**Pattern**: Validation functions should always use table-driven tests with:
- Valid inputs (multiple forms)
- Invalid inputs (injection patterns, empty, nil, boundary values)
- Edge cases (max values, unicode, special chars)

---

## Concurrency Patterns

### LL-12: Protect Maps with sync.RWMutex

**C-2 (Peer Review)**: `registeredServices` map accessed from multiple goroutines without synchronization caused data race.

**Pattern**: Any map accessed from multiple goroutines MUST be protected. Use `sync.RWMutex` with read-lock for reads and write-lock for writes.

---

### LL-13: Context Cancellation for Graceful Shutdown

**Pattern**: All long-running goroutines must select on `ctx.Done()`:
```go
select {
case <-ticker.C:
    // work
case <-ctx.Done():
    return
}
```

---

### LL-14: Atomic Values for Simple State

**Pattern**: Use `atomic.Value` for simple enum-like state (State machine). Use `sync.RWMutex` for complex types (pointers, maps).

---

## Architecture Decisions

### LL-15: Supervisor Tree for 24/7 Operation

Using suture's Erlang-style supervisor tree provides automatic restart, rate limiting, and clean shutdown for all stream services. This was the right call for a daemon that must run for years unattended.

### LL-16: Atomic Config Save (Temp + Rename)

Writing to a temp file in the same directory, syncing, then renaming prevents partial writes from corrupting config. This pattern is critical for any file that must survive power loss.

### LL-17: Exponential Backoff with Reset

Backoff that resets after N seconds of successful running prevents permanent slowdown after transient failures. The 300s threshold was validated against field experience with the bash version.

---

## What Worked Well

1. **Strict TDD**: Every internal package > 87% coverage, caught real bugs early
2. **Table-driven tests**: Comprehensive, readable, easy to extend
3. **`t.TempDir()`**: Clean test isolation, no leftover artifacts
4. **Injected dependencies**: `atomicCreateTemp` in config made error paths testable
5. **#nosec annotations**: Documented every security suppression with rationale
6. **Multiple audit passes**: Sonnet 4.6 found 59 issues, Opus 4.6 found 3 more bugs + 5 security issues
7. **Byte-for-byte bash compatibility**: Validated with character comparison tests

---

## What To Watch For

1. **Coverage table drift**: CLAUDE.md coverage numbers went stale (up to 7.5% off). Re-measure after every significant change.
2. **Permission consistency**: Permissions were inconsistent across lock, config, and backup packages. Use the permission matrix in SECURITY_AUDIT.md as the source of truth.
3. **New HTTP endpoints**: Any new listener must default to localhost binding.
4. **New file creation**: Any new `os.Create`, `os.WriteFile`, `os.MkdirAll` must use least-privilege permissions.
5. **New `exec.Command` calls**: Validate all arguments, especially those derived from user input.
6. **New `defer` in `if` blocks**: Check what variable the closure captures.

---

## Production Readiness Patterns (Phase 6)

### LL-12: Dead Code Is Invisible Infrastructure Debt

**Pattern**: The MediaMTX API client (92.4% coverage) was never imported by any production code. High test coverage created a false sense of completeness — the code *worked* but was never *used*.

**Lesson**: Coverage alone does not prove integration. After writing a supporting module, search for callers (`grep -r "package/name" --include="*.go"`) outside of tests. If there are none, the module is dead code regardless of its coverage score.

### LL-13: Always Verify Claims Before Acting

**Pattern**: Of 15 production-readiness findings from a prior session, 2 were demonstrably false (P-6: config validation exists; P-13: menu IS fully wired), and 1 was partially incorrect (P-10: some resource limits did exist). Blindly implementing "fixes" for non-issues wastes time and can introduce bugs.

**Lesson**: Before fixing any finding, verify it by reading the actual code. Search for the specific function calls, imports, and code paths mentioned. Trust evidence, not assertions.

### LL-14: Daemon Goroutines Resist Unit Testing

**Pattern**: Adding the P-3 recovery loop, P-1/P-2 health check loop, and P-7 USB stabilization delay increased daemon code by ~100 lines but decreased coverage from 32.7% to 26.3%. These goroutines run in production context and require real MediaMTX, FFmpeg, and USB devices to exercise.

**Lesson**: Extract testable logic into named functions or types (like `supervisorStatusProvider`). Goroutine-based daemon logic should be tested via integration tests or by injecting mock dependencies. Accept that daemon coverage will be lower than library coverage.

### LL-15: Embedded Service Files Need Sync Tests

**Pattern**: Updating `systemd/lyrebird-stream.service` (P-10 memory limits) immediately broke `TestInstallLyreBirdServiceMatchesSystemdFile` because the embedded copy in `main.go` was out of sync. The test caught this instantly.

**Lesson**: When a file has an embedded copy (like service templates), the sync test is essential. Always update both copies simultaneously.

### LL-16: Backup Before Save Is a Caller Responsibility

**Pattern**: The `BackupBeforeSave` function existed but was never called because `Config.Save()` doesn't internally call it. This is correct — Save is a general-purpose method that shouldn't always create backups.

**Lesson**: Backup-before-write is a *caller* concern, not a method concern. CLI commands that modify user config should call `BackupConfig()` before `Save()`. The daemon (which only reads config) has no need for backups.

### LL-17: Pin-Once Device Identifiers Rot Under Re-enumeration

**Pattern**: The daemon resolved each stream's ALSA device to `hw:<card>,0` exactly once at registration and keyed the registry on the sanitized device *name*. When a USB mic returned on a different card number (unplug/replug, hub reset, a USB bus reset from a field power dip), the poller — which skips already-registered names — never re-resolved it, so the manager drove the stale card for hours until backoff exhaustion. The stall detector couldn't help either: a departed publisher makes MediaMTX return 404, which the detector treated as "skip", not "unhealthy".

**Lesson**: Any identifier derived from volatile OS enumeration (card numbers, `/dev` indices, PIDs) must be *re-validated on each poll*, not pinned at first sight. Track it alongside the registration and restart the consumer when it changes. Prefer a registration-owned map that the add-path always overwrites, so the delete-paths cannot desync it into a wrong decision.

### LL-18: `tee` Muxer Couples Output Fates — Decouple With `onfail=ignore`

**Pattern**: A single ffmpeg `tee` feeding both the live RTSP output and the local segment recorder defaults to `onfail=abort`, so a failure in *either* slave (a full recording disk, an RTSP drop) aborted the whole process — the "redundant" local recording became a single point of failure on the critical live path.

**Lesson**: When one process fans out to multiple sinks with independent failure domains, make the non-critical sink `onfail=ignore` so its failure can't take down the critical one. Keep the critical sink at `onfail=abort` so a genuine failure still triggers a fast supervised restart (which also re-establishes the ignored sink). Codec/container pairing still matters: a single encode muxed to both sinks means both must accept that codec (opus → ogg, not wav/flac).

### LL-19: Recover-and-Restart Beats Bare `SafeGo` for Daemon Loops

**Pattern**: `util.SafeGo` recovers a goroutine panic but leaves the goroutine dead. For a long-lived daemon loop (device poller, stall detector) that silently converts "one panic → whole-daemon crash + clean systemd restart" into "one subsystem permanently dead while the daemon looks healthy" — often worse on an unattended device.

**Lesson**: Wrap critical background loops in a recover-*and-restart* supervisor (`runSupervised`) so a panic is contained to its subsystem, logged with a stack, and the loop self-heals — without crashing the process and dropping every stream. Gate the restart with a short delay so a tight panic loop can't spin the CPU, and exit cleanly when the context is cancelled.

### LL-20: Identity Keys Must Be Deterministic — Timestamped Fallbacks Poison Registries

**Pattern**: `SanitizeDeviceName` returns `unknown_device_<unix-timestamp>` for
unusable raw names (bash-compatible). Fine for a one-shot CLI; fatal for the
daemon, which keyed its stream registry on the name across polls — the same
physical device got a NEW identity every second, so every poll registered
another stream (unbounded manager/lock/FFmpeg growth).

**Lesson**: Any value used as a long-lived registry/config key must be a pure
function of stable attributes. If a sanitizer has a time-, random- or
counter-based fallback, wrap it (`Device.StableName()` derives
`usb_<vendor>_<product>`) before using it as identity. Fuzz the invariant:
same input → same key, always.

### LL-21: Resources Closed by a Service Wrapper Must Be Re-Openable

**Pattern**: `streamService.Run` closes the manager after every `Run` return
(correct for fd hygiene), but suture re-Serves the SAME service object after a
failure. The manager's log writer was created once in the constructor, so
every post-re-run FFmpeg lost its stderr logging silently.

**Lesson**: If a supervisor can re-run a service, everything the service's
teardown closes must be lazily re-acquirable at the start of `Run`. Decide the
failure policy explicitly: fail-fast at construction (operator feedback), but
tolerate-and-log on mid-life reopen (keep the stream alive).

### LL-22: Never Trust File mtimes Across a Clock-Sync Discontinuity

**Pattern**: No-RTC devices boot near the epoch; files written before NTP sync
carry ~1970 mtimes. Age-based retention run after the clock steps forward
computed a 55-year age and deleted irreplaceable pre-sync recordings.

**Lesson**: Age-based deletion needs a sanity floor: an mtime before any
plausible deployment date means "age unknowable — skip age policy" (keep
size-based policy, which needs no clock). Applies to any retention/expiry
logic on embedded devices without an RTC.

---

*Last updated: 2026-07-21*
