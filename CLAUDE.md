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
| cmd/lyrebird | 91.8% | CLI entry point |
| internal/audio | 80.4% | Device detection |
| internal/config | 89.1% | YAML config + migration |
| internal/lock | 76.8% | File-based locking |
| internal/stream | 59.7% | Stream manager (integration tests need ffmpeg) |
| internal/udev | 91.0% | udev rule generation |
| **Total** | **79.9%** | Target: 80%+ |

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

## Codebase Structure

```
lyrebirdaudio-go/
├── cmd/
│   └── lyrebird/           # Main CLI application
│       ├── main.go         # Entry point, command routing
│       └── main_test.go    # CLI tests
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
│   └── udev/               # udev rule generation
│       ├── mapper.go       # USB port path detection
│       ├── rules.go        # Rule generation
│       └── *_test.go
├── testdata/               # Test fixtures
│   └── config/
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

```
lyrebird help              # Show usage
lyrebird version           # Show version info
lyrebird devices           # List detected USB audio devices
lyrebird detect            # Detect capabilities and recommend settings
lyrebird usb-map           # Create udev rules (requires root)
lyrebird migrate           # Convert bash config to YAML
lyrebird validate          # Validate configuration file

# Stub commands (not yet implemented):
lyrebird status            # Show stream status
lyrebird setup             # Interactive setup wizard
lyrebird install-mediamtx  # Install MediaMTX
lyrebird test              # Test config without modifying system
lyrebird diagnose          # Run diagnostics
lyrebird check-system      # Check system compatibility
```

**Common Flags:**
```bash
--config=/path/to/config.yaml    # Config file path
--from=/path/to/bash.conf        # Source for migration
--to=/path/to/output.yaml        # Destination for migration
--force                          # Overwrite existing files
--dry-run                        # Preview without writing
--output=/path/to/rules          # Output path for udev rules
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

CI enforces 80% minimum coverage. Current: **79.9%**

To improve coverage:
- Add tests for error paths
- Mock external dependencies (ffmpeg, sysfs)
- Test edge cases in existing functions

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

### In Progress
- MediaMTX API client (`internal/mediamtx`)
- systemd service generation (`internal/systemd`)
- Health monitoring (`internal/diagnostics`)

### Planned
- Complete stub CLI commands (status, setup, diagnose)
- Prometheus metrics endpoint
- Interactive setup wizard

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

*Last updated: 2025-11-28*
