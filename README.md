# LyreBirdAudio-Go

**Production-grade USB audio streaming to RTSP via MediaMTX**

[![CI](https://github.com/tomtom215/lyrebirdaudio-go/actions/workflows/ci.yml/badge.svg)](https://github.com/tomtom215/lyrebirdaudio-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/tomtom215/lyrebirdaudio-go)](https://goreportcard.com/report/github.com/tomtom215/lyrebirdaudio-go)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Overview

LyreBirdAudio-Go is a complete rewrite of [LyreBirdAudio](https://github.com/tomtom215/LyreBirdAudio) in Go, providing a drop-in replacement for the bash implementation with improved reliability, concurrency, and maintainability.

**Status:** ✅ **Production Ready** - Validated for supervised deployments. See [Field Deployment](#local-recording-safety-net) before first unattended remote deployment.

### Key Features

- **24/7/365 Reliability**: Designed for continuous operation with automatic recovery
- **USB Hotplug Support**: Automatic detection and stream management for USB audio devices
- **Persistent Device Mapping**: udev-based device persistence across reboots
- **Concurrent Stream Management**: Parallel stream startup and monitoring using Go routines
- **Static Binary Deployment**: Single binary with no shared library dependencies (requires ffmpeg, udevadm, systemctl at runtime)
- **Cross-Platform**: Supports x86_64, ARM64, ARMv7, ARMv6 (Raspberry Pi)
- **Test-Driven Development**: Comprehensive test coverage for production reliability

### Architecture

```
┌─────────────┐
│ USB Devices │
└──────┬──────┘
       │
       ├─── udev rules (/etc/udev/rules.d/99-usb-soundcards.rules)
       │    └─── Persistent device mapping by physical USB port
       │
       ├─── ALSA (/proc/asound/card{N}/)
       │    └─── Device detection and capability scanning
       │
       ├─── FFmpeg (per-device process)
       │    └─── ALSA capture → Opus/AAC encoding
       │
       ├─── MediaMTX (RTSP server)
       │    └─── Stream distribution (rtsp://host:8554/device_name)
       │
       └─── lyrebird-stream (Go daemon)
            ├─── Device monitoring
            ├─── Stream lifecycle management
            ├─── Exponential backoff on failures
            └─── Health monitoring and recovery
```

## Installation

### Prerequisites

- Linux kernel 4.4+ (Ubuntu 20.04+, Debian 11+, Raspberry Pi OS)
- Go 1.24+ (for building from source; required for koanf hot-reload features)
- FFmpeg with ALSA support
- systemd (for service management)
- udev (for device persistence)

### Quick Start (Pre-built Binary)

```bash
# Download latest release
curl -L https://github.com/tomtom215/lyrebirdaudio-go/releases/latest/download/lyrebird-linux-amd64 -o lyrebird
chmod +x lyrebird
sudo mv lyrebird /usr/local/bin/

# Interactive setup
sudo lyrebird setup

# Or non-interactive
sudo lyrebird setup --auto
```

### Build from Source

```bash
# Clone repository
git clone https://github.com/tomtom215/lyrebirdaudio-go.git
cd lyrebirdaudio-go

# Build all binaries
make build

# Install to /usr/local/bin
sudo make install

# Run tests
make test

# Run tests with coverage
make test-coverage
```

## Usage

### Setup and Configuration

```bash
# 1. Install MediaMTX
sudo lyrebird install-mediamtx

# 2. Map USB devices (creates udev rules)
sudo lyrebird usb-map
# Note: Requires reboot for udev rules to take effect

# 3. Detect device capabilities
sudo lyrebird detect

# 4. Start streaming
sudo systemctl start lyrebird-stream
sudo systemctl enable lyrebird-stream  # Enable on boot
```

### Managing Streams

```bash
# Check stream status
lyrebird status

# List detected devices
lyrebird devices

# View logs
journalctl -u lyrebird-stream -f

# Restart all streams
sudo systemctl restart lyrebird-stream

# Stop streaming
sudo systemctl stop lyrebird-stream
```

### Configuration

Configuration is stored in `/etc/lyrebird/config.yaml`:

```yaml
# Device-specific configuration
devices:
  blue_yeti:
    sample_rate: 48000
    channels: 2
    bitrate: 192k
    codec: opus
    thread_queue: 8192

  usb_audio_1:
    sample_rate: 44100
    channels: 1
    bitrate: 128k
    codec: opus

# Global defaults (used when device-specific config not found)
default:
  sample_rate: 48000
  channels: 2
  bitrate: 128k
  codec: opus
  thread_queue: 8192

# Stream manager settings
stream:
  initial_restart_delay: 10s    # First restart delay
  max_restart_delay: 300s       # Maximum backoff delay
  max_restart_attempts: 50      # Max attempts before giving up
  usb_stabilization_delay: 5s   # Wait after USB changes

  # LOCAL RECORDING SAFETY NET (STRONGLY RECOMMENDED for unattended deployment)
  # Without local_record_dir, a MediaMTX crash at 3 AM loses audio with no recovery.
  # Set this to enable simultaneous local recording alongside RTSP streaming (tee muxer).
  # Segments are named: <device>_YYYYMMDD_HHMMSS.<segment_format>
  local_record_dir: /var/lib/lyrebird/recordings  # Comment out to disable
  segment_duration: 3600     # Segment length in seconds (default: 1 hour)
  segment_format: wav        # wav, flac, or ogg (default: wav = lossless)
  segment_max_age: 168h      # Delete segments older than 7 days (0 = no limit)
  segment_max_total_bytes: 0 # Delete oldest segments when dir exceeds this size (0 = no limit)
  # At 48kHz/stereo/WAV ≈ 660 MB/hour per stream. A 64 GB Pi holds ~48 hours per stream.

# MediaMTX integration
mediamtx:
  api_url: http://localhost:9997
  rtsp_url: rtsp://localhost:8554
  config_path: /etc/mediamtx/mediamtx.yml

# Monitoring
monitor:
  enabled: true
  interval: 5m                    # Health check / recovery interval
  stall_check_interval: 60s       # How often to check for stalled streams
  max_stall_checks: 3             # Stall checks before restart (3 × 60s = 3 min)
  restart_unhealthy: true         # Auto-restart failed streams
  health_addr: 127.0.0.1:9998    # Health endpoint address (GAP-8: now configurable)
  disk_low_threshold_mb: 1024     # Warn when free disk < 1 GB (0 = disabled)
```

#### Local Recording Safety Net

> **Important for unattended field deployment**: Without `local_record_dir`, a
> MediaMTX crash or network outage at 3 AM silently loses all audio. There is
> no recovery. For wildlife monitoring stations, bioacoustics research, or any
> deployment where recordings cannot be repeated, **always set `local_record_dir`**.

```bash
# Create recording directory with correct permissions
sudo mkdir -p /var/lib/lyrebird/recordings
sudo chown lyrebird:audio /var/lib/lyrebird/recordings
sudo chmod 750 /var/lib/lyrebird/recordings

# Add to config
sudo lyrebird setup   # Interactive setup includes local_record_dir prompt
```

#### Environment Variable Overrides

Configuration values can be overridden using environment variables with the `LYREBIRD_` prefix:

```bash
# Override default settings
export LYREBIRD_DEFAULT_SAMPLE_RATE=44100
export LYREBIRD_DEFAULT_CODEC=aac
export LYREBIRD_DEFAULT_BITRATE=256k

# Override device-specific settings
export LYREBIRD_DEVICES_BLUE_YETI_SAMPLE_RATE=96000
export LYREBIRD_DEVICES_BLUE_YETI_CODEC=aac

# Override stream settings
export LYREBIRD_STREAM_MAX_RESTART_DELAY=600s

# Override MediaMTX settings
export LYREBIRD_MEDIAMTX_API_URL=http://custom-host:9997
```

**Precedence Order** (highest to lowest):
1. Environment variables (`LYREBIRD_*`)
2. YAML configuration file (`/etc/lyrebird/config.yaml`)
3. Built-in defaults

This makes LyreBird a true [12-factor app](https://12factor.net/config), perfect for Docker/Kubernetes deployments:

```yaml
# Kubernetes example
env:
  - name: LYREBIRD_DEFAULT_BITRATE
    value: "256k"
  - name: LYREBIRD_MEDIAMTX_API_URL
    value: "http://mediamtx-service:9997"
```

#### Hot-Reload Configuration

The daemon supports configuration hot-reload without downtime via SIGHUP:

```bash
# Edit configuration
sudo vim /etc/lyrebird/config.yaml

# Reload configuration without stopping streams
sudo systemctl reload lyrebird-stream

# Or send SIGHUP directly
sudo pkill -HUP lyrebird-stream
```

Configuration changes are reloaded immediately. Future enhancements will restart only affected streams.

### Migration from Bash

**Timeline**: The Go implementation is now production-ready for supervised deployments. The bash version remains available for reference. See the [RUNBOOK](docs/RUNBOOK.md) for field operator procedures.

**Migration Tool**:
```bash
# Automatic config migration
lyrebird migrate --from=/etc/mediamtx/audio-devices.conf --to=/etc/lyrebird/config.yaml

# Validate migration
lyrebird validate --config=/etc/lyrebird/config.yaml

# Test run (doesn't modify system)
lyrebird test --config=/etc/lyrebird/config.yaml
```

**Compatibility**:
- udev rules format: 100% identical (byte-for-byte match)
- systemd service behavior: Preserved (restart policies, dependencies)
- Stream naming: Identical to bash version
- CLI interface: Backward-compatible wrappers available

## Development

### Project Structure

```
lyrebirdaudio-go/
├── cmd/                        # Command-line applications
│   ├── lyrebird/              # Main CLI (all commands)
│   └── lyrebird-stream/       # Stream manager daemon
├── internal/                   # Internal packages (not importable)
│   ├── audio/                 # ALSA device detection & capabilities
│   ├── config/                # Configuration management (koanf)
│   ├── diagnostics/           # System health checks (24 checks)
│   ├── lock/                  # File-based locking (flock)
│   ├── mediamtx/              # MediaMTX REST API client
│   ├── menu/                  # Interactive TUI menus (huh)
│   ├── stream/                # Stream lifecycle & backoff
│   ├── supervisor/            # Erlang-style supervisor trees (suture)
│   ├── udev/                  # udev rule generation
│   ├── updater/               # Self-update from GitHub releases
│   └── util/                  # Panic recovery, resources
├── systemd/                    # systemd service templates
├── testdata/                   # Test fixtures and mock data
├── .github/workflows/          # CI/CD pipelines
├── Makefile                    # Build automation
├── go.mod                      # Go module definition
└── README.md                   # This file
```

### Testing

We follow **strict test-driven development**:

```bash
# Run all tests
make test

# Run tests with race detection
make test-race

# Run tests with coverage
make test-coverage

# Run integration tests (requires USB hardware)
make test-integration

# Run benchmarks
make bench

# Generate coverage report
make coverage-html
```

**Testing Requirements**:
- Every new function requires corresponding tests
- Critical paths require table-driven tests with edge cases
- Minimum 65% overall code coverage (enforced by CI)
- Internal packages: ~87% coverage (aim for 90%+)
- CLI packages: Lower coverage acceptable (root/interactive requirements)
- Integration tests for USB/udev/systemd components
- Race detection enabled in CI/CD

### Code Quality

```bash
# Run linters
make lint

# Format code
make fmt

# Vet code
make vet

# Run all checks (fmt, vet, lint, test)
make check
```

### CI/CD Pipeline

GitHub Actions workflow (`.github/workflows/ci.yml`):
- ✅ Go fmt verification
- ✅ Go vet checks
- ✅ golangci-lint (comprehensive linting)
- ✅ Unit tests with race detection
- ✅ Integration tests (ubuntu-latest; hardware-specific paths are skipped in CI)
- ✅ Cross-compilation validation (amd64, arm64, armv7, armv6)
- ✅ Code coverage reporting
- ✅ Security scanning (gosec)
- ✅ Dependency vulnerability scanning

## Health Endpoint & Monitoring

The daemon exposes a health endpoint at `127.0.0.1:9998` (configurable via `monitor.health_addr`).

### Endpoints

| Path | Format | Description |
|------|--------|-------------|
| `/healthz` | JSON | Service health, disk space, NTP sync status |
| `/metrics` | Prometheus text | Per-stream uptime, restarts, failures, disk gauges |

```bash
# Check daemon health
curl -s http://127.0.0.1:9998/healthz | jq .

# Scrape Prometheus metrics
curl -s http://127.0.0.1:9998/metrics
```

### Example /healthz Response

```json
{
  "status": "healthy",
  "timestamp": "2026-03-02T10:00:00Z",
  "services": [
    {"name": "blue_yeti", "state": "running", "healthy": true, "uptime_ns": 3600000000000, "restarts": 0}
  ],
  "system": {
    "disk_free_bytes": 42949672960,
    "disk_total_bytes": 64424509440,
    "disk_low_warning": false,
    "ntp_synced": true
  }
}
```

### Prometheus / Grafana Integration

Point Prometheus at `http://<pi-ip>:9998/metrics` (update `health_addr` to `0.0.0.0:9998` if scraping remotely — keep behind firewall). See `docs/monitoring-timer.sh` for a minimal alerting script using `systemd-timer`.

## Troubleshooting

### Common Issues

**Streams not starting**:
```bash
# Check device detection
lyrebird devices

# Verify FFmpeg can access ALSA
arecord -l

# Check MediaMTX status
systemctl status mediamtx

# View detailed logs
journalctl -u lyrebird-stream -f --no-pager
```

**Device not detected after hotplug**:
```bash
# Verify udev rules loaded
sudo udevadm control --reload-rules
sudo udevadm trigger

# Check for symlinks
ls -la /dev/snd/by-usb-port/
```

**Port conflicts (8554/9997)**:
```bash
# Check what's using the ports
sudo netstat -tulpn | grep -E '8554|9997'

# Stop conflicting service
sudo systemctl stop <service-name>
```

### Debug Mode

```bash
# Enable debug logging
sudo lyrebird-stream --log-level=debug

# Or via EnvironmentFile (for systemd-managed service)
# Add to /etc/lyrebird/environment:
# LYREBIRD_LOG_LEVEL=debug
sudo systemctl restart lyrebird-stream
```

### Health Checks

```bash
# Run diagnostics
lyrebird diagnose

# Check system compatibility
lyrebird check-system

# Validate configuration
lyrebird validate --config=/etc/lyrebird/config.yaml
```

## Performance

### Resource Usage

Typical resource consumption (per stream):
- CPU: 1-3% (idle), 5-15% (active encoding)
- Memory: 10-20 MB per stream
- Network: Depends on bitrate (128kbps default = ~16KB/s)
- Startup time: ~50ms (vs ~500ms for bash version)
- Stream initialization: <100ms per device

### Benchmarks

Run benchmarks locally with:

```bash
make bench
```

## Contributing

Contributions are welcome! This project follows industrial-grade development practices:

### Development Workflow

1. **Fork and clone** the repository
2. **Create a feature branch**: `git checkout -b feature/your-feature`
3. **Write tests first** (TDD is mandatory)
4. **Implement the feature** to make tests pass
5. **Run all checks**: `make check`
6. **Commit with clear messages** following [Conventional Commits](https://www.conventionalcommits.org/)
7. **Open a pull request** with description of changes

### Code Standards

- **Test-Driven Development**: Write tests first, then implementation
- **Documentation**: Update README and godoc comments for public APIs
- **Code Review**: All changes require review before merge
- **CI/CD**: All tests must pass, coverage must not decrease
- **Minimum coverage**: 65% overall, 90%+ for internal packages
- **Race detection**: Code must pass `go test -race`

### Commit Message Format

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Credits

- Original bash implementation: [LyreBirdAudio](https://github.com/tomtom215/LyreBirdAudio)
- MediaMTX: [bluenviron/mediamtx](https://github.com/bluenviron/mediamtx)
- FFmpeg: [FFmpeg project](https://ffmpeg.org/)

## Support

- **Issues**: [GitHub Issues](https://github.com/tomtom215/lyrebirdaudio-go/issues)
- **Discussions**: [GitHub Discussions](https://github.com/tomtom215/lyrebirdaudio-go/discussions)
- **Original Bash Version**: [LyreBirdAudio](https://github.com/tomtom215/LyreBirdAudio)

---

**Project Status**: Active Development | **Production Ready**: No | **Field Testing**: Pending
