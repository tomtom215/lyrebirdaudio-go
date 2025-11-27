# LyreBirdAudio-Go

**Production-grade USB audio streaming to RTSP via MediaMTX**

[![CI](https://github.com/tomtom215/lyrebirdaudio-go/actions/workflows/ci.yml/badge.svg)](https://github.com/tomtom215/lyrebirdaudio-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/tomtom215/lyrebirdaudio-go)](https://goreportcard.com/report/github.com/tomtom215/lyrebirdaudio-go)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Overview

LyreBirdAudio-Go is a complete rewrite of [LyreBirdAudio](https://github.com/tomtom215/LyreBirdAudio) in Go, providing a drop-in replacement for the bash implementation with improved reliability, concurrency, and maintainability.

**Status:** 🚧 **ACTIVE DEVELOPMENT** - Not yet production ready. See [Migration from Bash](#migration-from-bash) for timeline.

### Key Features

- **24/7/365 Reliability**: Designed for continuous operation with automatic recovery
- **USB Hotplug Support**: Automatic detection and stream management for USB audio devices
- **Persistent Device Mapping**: udev-based device persistence across reboots
- **Concurrent Stream Management**: Parallel stream startup and monitoring using Go routines
- **Static Binary Deployment**: Single binary with no runtime dependencies
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
- Go 1.21+ (for building from source)
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

# MediaMTX integration
mediamtx:
  api_url: http://localhost:9997
  rtsp_url: rtsp://localhost:8554
  config_path: /etc/mediamtx/mediamtx.yml

# Monitoring
monitor:
  enabled: true
  interval: 5m              # Health check interval
  restart_unhealthy: true   # Auto-restart failed streams
```

### Migration from Bash

**Timeline**: The bash version will be supported until this Go implementation reaches feature parity and completes field testing (estimated: Q2 2025).

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
│   ├── lyrebird/              # Main CLI (replaces orchestrator.sh)
│   ├── lyrebird-stream/       # Stream manager daemon
│   ├── lyrebird-usb/          # USB mapper utility
│   └── lyrebird-install/      # MediaMTX installer
├── internal/                   # Internal packages (not importable)
│   ├── audio/                 # ALSA device detection
│   ├── config/                # Configuration management
│   ├── udev/                  # udev rule generation
│   ├── stream/                # Stream lifecycle management
│   ├── mediamtx/              # MediaMTX integration
│   ├── systemd/               # systemd service management
│   ├── lock/                  # File-based locking
│   └── diagnostics/           # Health checks and diagnostics
├── pkg/                        # Public packages (importable)
│   └── lyrebird/              # Public API
├── scripts/                    # Installation and helper scripts
├── systemd/                    # systemd service templates
├── testdata/                   # Test fixtures and data
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
- Minimum 80% code coverage (aim for 90%+)
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
- ✅ Integration tests (on self-hosted runner with USB devices)
- ✅ Cross-compilation validation (amd64, arm64, armv7, armv6)
- ✅ Code coverage reporting
- ✅ Security scanning (gosec)
- ✅ Dependency vulnerability scanning

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

# Or via environment variable
export LYREBIRD_LOG_LEVEL=debug
sudo -E systemctl restart lyrebird-stream
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

### Benchmarks

See [BENCHMARKS.md](docs/BENCHMARKS.md) for detailed performance comparisons:
- Bash vs Go startup time
- Stream initialization latency
- Memory footprint comparison
- CPU utilization under load

## Contributing

Contributions are welcome! This project follows industrial-grade development practices:

1. **Test-Driven Development**: Write tests first, then implementation
2. **Documentation**: Update README and godoc comments
3. **Code Review**: All changes require review before merge
4. **CI/CD**: All tests must pass, coverage must not decrease

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

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
