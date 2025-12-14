# CLAUDE.md - LyreBirdAudio Go Codebase Guide

**Project**: LyreBirdAudio - USB audio streaming to RTSP (Go port)
**Go Version**: 1.23
**Repository**: github.com/tomtom215/lyrebirdaudio-go

---

## Quick Reference

### Common Commands

```bash
# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# Build binary
go build -o bin/lyrebird ./cmd/lyrebird

# Run linters
go vet ./...
golangci-lint run ./...

# Format code
gofmt -s -w .

# Tidy modules
go mod tidy
```

### Current Test Coverage

| Package | Coverage | Notes |
|---------|----------|-------|
| internal/supervisor | 94.3% | Supervisor tree for service management |
| internal/util | 94.0% | Panic recovery, resource tracking |
| internal/config | 89.1% | YAML config + migration |
| internal/udev | 86.0% | udev rule generation + file writing |
| internal/stream | 85.5% | Stream manager with backoff |
| internal/audio | 84.3% | Device detection |
| internal/lock | 75.7% | File-based locking |
| cmd/lyrebird | 50.5% | CLI (many commands require root/interactive) |
| cmd/lyrebird-stream | 18.3% | Daemon (requires runtime environment) |
| **Internal packages** | **~87%** | Core library code well-tested |

---

## Project Overview

LyreBirdAudio captures audio from USB microphones and streams them via RTSP using FFmpeg and MediaMTX. This is a Go port of the original bash implementation, designed for 24/7 unattended operation.

### Key Features

- Automatic USB audio device detection
- Persistent device identification via USB port mapping
- YAML configuration with bash migration support
- Stream lifecycle management with exponential backoff
- File-based locking to prevent duplicate streams

---

## ⚠️ CRITICAL: Strict Test-Driven Development (TDD)

**This project follows STRICT Test-Driven Development practices. This is NON-NEGOTIABLE.**

### Reliability Requirements

LyreBirdAudio must achieve **industrial control system (RTOS) level reliability** for 24/7/365 unattended operation. The original bash version has been field-tested by real users running continuously for years. This Go port must be **AT LEAST as stable** and should aim for:

- Zero unexpected crashes or panics
- Graceful degradation under all failure scenarios
- Recovery from every conceivable edge case
- Deterministic behavior under all conditions
- Defense against operator error and environmental disruption

### TDD Workflow - MANDATORY

**Every single line of production code MUST have corresponding tests written FIRST.**

1. **Write the test first** - Before any implementation
   - Cover the happy path
   - Cover all error paths
   - Cover boundary conditions
   - Cover edge cases (even "impossible" ones)
   - Cover race conditions and timing issues

2. **Watch it fail** - Verify the test fails for the right reason

3. **Write minimal code** - Just enough to make the test pass

4. **Refactor** - Improve code while keeping tests green

5. **Repeat** - For every feature, bug fix, or change

### Coverage Requirements

- **Minimum**: 80% (enforced by CI)
- **Target**: 95%+ for all packages
- **Critical components** (stream manager, lock, config): 100% of realistic paths
- **Error paths**: Every error return must have a test that triggers it
- **Panic recovery**: Every potential panic must be tested
- **Concurrent code**: Must pass `go test -race` with zero warnings

### What Must Be Tested

1. **Happy paths** - Normal operation
2. **Error paths** - Every `if err != nil` branch
3. **Boundary conditions** - Empty inputs, max values, zero values
4. **Invalid inputs** - Malformed data, wrong types, nil pointers
5. **File system failures** - Missing files, permission denied, disk full, read-only FS
6. **Process failures** - Command not found, command crashes, signals
7. **Network failures** - Connection refused, timeouts, DNS failures
8. **Concurrent access** - Race conditions, deadlocks, data corruption
9. **Resource exhaustion** - Out of memory, file descriptors, disk space
10. **Signal handling** - SIGINT, SIGTERM, SIGHUP during various states
11. **State transitions** - Every valid and invalid state change
12. **Time-based behavior** - Timeouts, delays, backoff, expiration
13. **Platform differences** - Different kernels, filesystems, architectures

### Test Quality Standards

- Use table-driven tests for comprehensive coverage
- Test names must clearly describe the scenario
- Tests must be deterministic (no flaky tests)
- Tests must be fast (< 100ms for unit tests)
- Tests must be isolated (no shared state)
- Tests must clean up resources (files, goroutines)
- Mock external dependencies (ffmpeg, udev, MediaMTX)
- Use `t.TempDir()` for file operations
- Check error messages, not just error existence

### Forbidden Practices

- ❌ Writing code without tests first
- ❌ Skipping tests for "simple" code
- ❌ Testing only happy paths
- ❌ Ignoring race detector warnings
- ❌ Committing code with failing tests
- ❌ Lowering coverage thresholds
- ❌ Using `// TODO: add tests`
- ❌ Mocking time.Sleep() without testing backoff logic

---

## Codebase Structure

```
lyrebirdaudio-go/
├── cmd/
│   ├── lyrebird/           # Main CLI application
│   │   ├── main.go         # Entry point, all CLI commands
│   │   └── main_test.go    # CLI tests
│   └── lyrebird-stream/    # Streaming daemon
│       ├── main.go         # Daemon with supervisor tree
│       └── main_test.go    # Daemon tests
├── internal/
│   ├── audio/              # USB audio device detection
│   │   ├── detector.go     # Scans /proc/asound for devices
│   │   ├── sanitize.go     # Device name sanitization
│   │   └── *_test.go
│   ├── config/             # Configuration management
│   │   ├── config.go       # YAML loading/saving/validation
│   │   ├── migrate.go      # Bash → YAML migration
│   │   └── *_test.go
│   ├── lock/               # File-based locking
│   │   ├── filelock.go     # flock(2) based locking
│   │   └── *_test.go
│   ├── stream/             # Core stream manager
│   │   ├── manager.go      # FFmpeg lifecycle management
│   │   ├── backoff.go      # Exponential backoff
│   │   └── *_test.go
│   ├── supervisor/         # Service supervision (NEW)
│   │   ├── supervisor.go   # Erlang-style supervisor tree
│   │   └── *_test.go
│   ├── udev/               # udev rule generation
│   │   ├── mapper.go       # USB port path detection
│   │   ├── rules.go        # Rule generation + file writing
│   │   └── *_test.go
│   └── util/               # Utility functions
│       ├── panic.go        # Panic recovery (SafeGo)
│       ├── resources.go    # Resource tracking
│       └── *_test.go
├── systemd/                # Systemd service templates (NEW)
│   └── lyrebird-stream.service
├── testdata/               # Test fixtures
│   └── config/
├── Makefile               # Build automation
├── go.mod
├── go.sum
└── .github/workflows/ci.yml
```

---

## Component Guide

### 1. Audio Detection (`internal/audio`)

Scans `/proc/asound/card*` for USB audio devices.

**Key Types:**
```go
type Device struct {
    CardNumber int    // ALSA card number (0-31)
    Name       string // Device name
    USBID      string // vendor:product (e.g., "0d8c:0014")
    VendorID   string
    ProductID  string
    DeviceID   string // From /dev/snd/by-id/ (optional)
}
```

**Key Functions:**
- `DetectDevices(asoundPath)` - Returns all USB audio devices
- `GetDeviceInfo(asoundPath, cardNum)` - Gets info for specific card
- `ParseUSBID(usbID)` - Splits "VVVV:PPPP" into vendor/product
- `SanitizeDeviceName(name)` - Converts to safe identifier

**Testing:** Uses temp directories with mock /proc/asound structure.

### 2. Configuration (`internal/config`)

YAML-based configuration with device-specific overrides.

**Config Structure:**
```yaml
devices:
  blue_yeti:
    sample_rate: 48000
    channels: 2
    bitrate: 192k
    codec: opus

default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus

stream:
  initial_restart_delay: 10s
  max_restart_delay: 300s
  max_restart_attempts: 50

mediamtx:
  api_url: http://localhost:9997
  rtsp_url: rtsp://localhost:8554
```

**Key Functions:**
- `LoadConfig(path)` - Loads and validates YAML
- `MigrateFromBash(path)` - Converts bash env vars to YAML
- `GetDeviceConfig(name)` - Returns device config with defaults merged
- `DefaultConfig()` - Returns production defaults

### 3. Stream Manager (`internal/stream`)

The core component managing FFmpeg process lifecycle.

**State Machine:**
```
idle → starting → running ⟲
                    ↓
                  failed → (backoff) → starting
                    ↓
                  stopped
```

**Key Types:**
```go
type ManagerConfig struct {
    DeviceName  string        // e.g., "blue_yeti"
    ALSADevice  string        // e.g., "hw:0,0"
    SampleRate  int           // e.g., 48000
    Channels    int           // e.g., 2
    Bitrate     string        // e.g., "128k"
    Codec       string        // "opus" or "aac"
    RTSPURL     string        // Output URL
    LockDir     string        // For lock files
    FFmpegPath  string        // Path to ffmpeg binary
    Backoff     *Backoff      // Backoff policy
}

type State int
const (
    StateIdle State = iota
    StateStarting
    StateRunning
    StateStopping
    StateFailed
    StateStopped
)
```

**Key Functions:**
- `NewManager(cfg)` - Creates new manager
- `Run(ctx)` - Main loop (blocks until context cancelled)
- `State()` - Returns current state (thread-safe)
- `Metrics()` - Returns uptime, attempts, failures

**Backoff Algorithm:**
- Initial delay: 10s
- Max delay: 300s (5 min)
- Doubles on each failure
- Resets after 300s of successful running

### 4. File Locking (`internal/lock`)

Prevents multiple managers for the same device.

```go
lock, _ := NewFileLock("/var/run/lyrebird/device.lock")
lock.Acquire(30 * time.Second)  // Blocks up to 30s
defer lock.Release()
```

### 5. udev Rules (`internal/udev`)

Generates persistent device symlinks based on USB port.

**Rule Format (byte-for-byte bash compatible):**
```
SUBSYSTEM=="sound", KERNEL=="controlC[0-9]*", ATTRS{busnum}=="1", ATTRS{devnum}=="5", SYMLINK+="snd/by-usb-port/1-1.4"
```

**Key Functions:**
- `GetUSBPhysicalPort(sysfsPath, busNum, devNum)` - Maps bus/dev to port path
- `GenerateRule(portPath, busNum, devNum)` - Creates single rule
- `GenerateRulesFile(devices)` - Creates complete rules file

---

## CLI Commands

### Main CLI (lyrebird)
```
lyrebird help              # Show usage
lyrebird version           # Show version info
lyrebird devices           # List detected USB audio devices
lyrebird detect            # Detect capabilities and recommend settings
lyrebird usb-map           # Create udev rules (requires root)
lyrebird migrate           # Convert bash config to YAML
lyrebird validate          # Validate configuration file
lyrebird status            # Show stream status and RTSP URLs
lyrebird setup             # Interactive setup wizard (requires root)
lyrebird install-mediamtx  # Install MediaMTX RTSP server (requires root)
lyrebird diagnose          # Run system diagnostics
lyrebird check-system      # Check system compatibility
lyrebird test              # Test config (stub - not yet implemented)
```

### Streaming Daemon (lyrebird-stream)
```
lyrebird-stream                          # Run with default config
lyrebird-stream --config=/path/to/yaml   # Custom config path
lyrebird-stream --lock-dir=/var/run/x    # Custom lock directory
lyrebird-stream --log-level=debug        # Enable debug logging
```

**Common Flags:**
```bash
--config=/path/to/config.yaml    # Config file path
--from=/path/to/bash.conf        # Source for migration
--to=/path/to/output.yaml        # Destination for migration
--force                          # Overwrite existing files
--dry-run                        # Preview without writing
--output=/path/to/rules          # Output path for udev rules
--auto, -y                       # Non-interactive mode (setup)
--version=vX.Y.Z                 # MediaMTX version (install-mediamtx)
--no-service                     # Skip systemd service (install-mediamtx)
```

---

## Development Workflow

### Adding New Features

1. **Write tests first** - Create `*_test.go` with test cases
2. **Implement minimum code** - Make tests pass
3. **Add edge cases** - Cover error paths
4. **Run full test suite** - `go test -race ./...`
5. **Check coverage** - `go test -cover ./...`
6. **Run linters** - `go vet ./... && golangci-lint run`

### Test Patterns

**Table-driven tests:**
```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {"valid", "input", "output", false},
        {"invalid", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Something(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.expected {
                t.Errorf("got %q, want %q", got, tt.expected)
            }
        })
    }
}
```

**Temp directories for file tests:**
```go
func TestFileOperation(t *testing.T) {
    tmpDir := t.TempDir()  // Auto-cleaned up
    // Create test fixtures in tmpDir
}
```

### Code Conventions

1. **Error handling**: Always wrap errors with context
   ```go
   if err != nil {
       return fmt.Errorf("failed to do X: %w", err)
   }
   ```

2. **Context propagation**: Pass `context.Context` through async operations
   ```go
   func Run(ctx context.Context) error
   ```

3. **Thread safety**: Use `atomic.Value` for state, `sync.RWMutex` for complex types
   ```go
   state atomic.Value  // For State enum
   mu    sync.RWMutex  // For pointers
   ```

4. **Validation**: Validate config at load time, not use time
   ```go
   cfg, err := LoadConfig(path)  // Validates internally
   ```

---

## CI/CD Pipeline

GitHub Actions workflow (`.github/workflows/ci.yml`):

1. **Quality** - Format check, go vet, golangci-lint
2. **Security** - gosec, govulncheck
3. **Test** - Race detector, coverage threshold (80%)
4. **Build** - Cross-compile for linux/amd64, arm64, arm/v7, arm/v6

### Coverage Threshold

CI enforces 80% minimum coverage. Internal packages average **~87%**.

Coverage notes:
- CLI commands (cmd/lyrebird) have lower coverage due to root/interactive requirements
- Streaming daemon (cmd/lyrebird-stream) requires runtime environment for full testing
- Internal packages are well-tested with comprehensive unit tests

---

## Key Design Decisions

### 1. Bash Compatibility

udev rules are byte-for-byte compatible with the bash version. This is validated with character-by-character comparison tests.

### 2. Exponential Backoff

Streams that crash are restarted with exponential backoff (10s → 20s → 40s → ... → 300s max). After 300s of successful running, backoff resets.

### 3. File-Based Locking

Each device gets a lock file (`/var/run/lyrebird/{device}.lock`) using `flock(2)`. Prevents duplicate stream managers.

### 4. Context Cancellation

All long-running operations respect `context.Context` for graceful shutdown. Signal handlers cancel context on SIGINT/SIGTERM.

### 5. State Machine

Manager uses explicit 6-state machine for predictable lifecycle management. State changes are atomic.

---

## Troubleshooting

### Tests failing with "ffmpeg not found"

Stream integration tests require ffmpeg:
```bash
sudo apt-get install ffmpeg
```

### Coverage below 80%

The stream manager's `startFFmpeg()` and `Run()` require ffmpeg. Unit tests cover validation logic; integration tests cover process management.

### Race conditions

Always run tests with race detector:
```bash
go test -race ./...
```

### Linter errors

```bash
# Fix formatting
gofmt -s -w .

# Check for issues
golangci-lint run ./...
```

---

## Future Work

### Completed (Phase 1-3)
- ✅ Supervisor tree for service management (`internal/supervisor`)
- ✅ Streaming daemon (`cmd/lyrebird-stream`)
- ✅ udev rules file writing and reloading
- ✅ Systemd service template (`systemd/lyrebird-stream.service`)
- ✅ CLI commands: status, diagnose, check-system, setup, install-mediamtx

### Remaining
- `lyrebird test` command - Test config without modifying system
- MediaMTX API client for runtime stream management
- Prometheus metrics endpoint
- Hot-reload configuration via SIGHUP

---

## External Dependencies

- **gopkg.in/yaml.v3** - YAML parsing
- **FFmpeg** - Audio encoding (runtime dependency)
- **MediaMTX** - RTSP server (runtime dependency)

---

## References

- [Original Bash Implementation](https://github.com/tomtom215/LyreBirdAudio)
- [MediaMTX Documentation](https://github.com/bluenviron/mediamtx)
- [FFmpeg Documentation](https://ffmpeg.org/documentation.html)
- [ALSA Documentation](https://www.alsa-project.org/wiki/Main_Page)

---

*Last updated: 2025-12-14*
