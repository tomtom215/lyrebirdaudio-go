# LyreBirdAudio-Go Feature Parity Implementation Plan

**Goal**: Achieve 100% feature parity with the original bash implementation
**Created**: 2025-12-14

---

## Phase 1: Core Audio Capabilities (Priority: HIGH)

### 1.1 Audio Capability Detection (`internal/audio/capabilities.go`)

Port the `lyrebird-mic-check.sh` functionality for non-invasive hardware detection.

**New Types:**
```go
type Capabilities struct {
    Formats     []string  // S16_LE, S24_LE, S32_LE, etc.
    SampleRates []int     // 8000, 16000, 44100, 48000, 96000, etc.
    Channels    []int     // 1, 2, 4, 6, 8
    BitDepths   []int     // 16, 24, 32
    MinRate     int
    MaxRate     int
    IsBusy      bool
}

type QualityTier string
const (
    QualityLow    QualityTier = "low"
    QualityNormal QualityTier = "normal"
    QualityHigh   QualityTier = "high"
)

type RecommendedSettings struct {
    SampleRate int
    Channels   int
    Codec      string
    Bitrate    string
    Format     string
}
```

**Key Functions:**
- `DetectCapabilities(asoundPath, cardNum)` - Parse `/proc/asound/cardX/stream0`
- `ParseALSAFormats(streamData)` - Extract supported formats
- `ParseSampleRates(streamData)` - Extract rate range
- `IsDeviceBusy(cardNum)` - Check without opening device
- `RecommendSettings(caps, tier)` - Generate optimal config per quality tier
- `CapabilitiesToJSON(caps)` - JSON output for scripting

**Files to parse:**
- `/proc/asound/cardX/stream0` - Format/rate capabilities
- `/proc/asound/cardX/pcm0c/sub0/hw_params` - Busy state detection
- `/proc/asound/cardX/pcm0c/sub0/status` - Current device status

### 1.2 Quality Tiers (`internal/config/quality.go`)

**Tier Definitions:**
```go
var QualityPresets = map[QualityTier]DeviceConfig{
    QualityLow: {
        SampleRate: 16000,
        Channels:   1,
        Codec:      "opus",
        Bitrate:    "24k",
    },
    QualityNormal: {
        SampleRate: 48000,
        Channels:   2,
        Codec:      "opus",
        Bitrate:    "128k",
    },
    QualityHigh: {
        SampleRate: 48000,
        Channels:   2,
        Codec:      "opus",
        Bitrate:    "256k",
    },
}
```

---

## Phase 2: Configuration Management (Priority: HIGH)

### 2.1 Config Backup/Restore (`internal/config/backup.go`)

**Functions:**
- `BackupConfig(configPath)` - Create timestamped backup
- `ListBackups(backupDir)` - List available backups
- `RestoreBackup(backupPath, configPath)` - Restore from backup
- `CleanOldBackups(backupDir, keepCount)` - Prune old backups

**Backup Format:**
```
/etc/lyrebird/backups/
├── config.yaml.2025-12-14T10-30-00.bak
├── config.yaml.2025-12-14T11-45-00.bak
└── config.yaml.2025-12-14T12-00-00.bak
```

### 2.2 SIGHUP Hot-Reload (`cmd/lyrebird-stream/`)

Modify daemon to reload config on SIGHUP:
- Parse new config file
- Validate before applying
- Gracefully restart affected streams
- Log reload success/failure

---

## Phase 3: Stream Manager Enhancements (Priority: HIGH)

### 3.1 Resource Monitoring (`internal/stream/monitor.go`)

**New Types:**
```go
type ResourceMetrics struct {
    FileDescriptors int
    CPUPercent      float64
    MemoryBytes     int64
    ThreadCount     int
    Timestamp       time.Time
}

type ResourceThresholds struct {
    FDWarning      int     // 500
    FDCritical     int     // 1000
    CPUWarning     float64 // 20.0
    CPUCritical    float64 // 40.0
    MemoryWarning  int64   // 512MB
    MemoryCritical int64   // 1GB
}
```

**Functions:**
- `MonitorResources(pid)` - Get current metrics
- `CheckThresholds(metrics, thresholds)` - Return warnings/critical alerts
- `StartResourceMonitor(ctx, interval)` - Background monitoring goroutine

**Data Sources:**
- `/proc/{pid}/fd/` - File descriptor count
- `/proc/{pid}/stat` - CPU time
- `/proc/{pid}/statm` - Memory usage

### 3.2 FFmpeg Log Rotation (`internal/stream/logrotate.go`)

**Functions:**
- `NewRotatingWriter(path, maxSize, maxFiles)` - Create rotating log writer
- `Rotate()` - Rotate current log file
- `Cleanup()` - Remove old log files

**Config:**
```yaml
stream:
  log_rotation:
    enabled: true
    max_size: 10MB
    max_files: 5
    compress: true
```

---

## Phase 4: MediaMTX Integration (Priority: MEDIUM)

### 4.1 MediaMTX API Client (`internal/mediamtx/client.go`)

**API Endpoints:**
- `GET /v3/paths/list` - List all paths
- `GET /v3/paths/get/{name}` - Get path details
- `POST /v3/paths/add/{name}` - Add path
- `DELETE /v3/paths/delete/{name}` - Remove path

**Types:**
```go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

type Path struct {
    Name        string `json:"name"`
    Source      string `json:"source"`
    Ready       bool   `json:"ready"`
    ReadyTime   string `json:"readyTime"`
    Tracks      []Track `json:"tracks"`
    BytesReceived int64 `json:"bytesReceived"`
}

type Track struct {
    Type  string `json:"type"`
    Codec string `json:"codec"`
}
```

**Functions:**
- `NewClient(apiURL)` - Create client
- `ListPaths(ctx)` - Get all streams
- `GetPath(ctx, name)` - Get stream details
- `IsStreamHealthy(ctx, name)` - Health check
- `WaitForStream(ctx, name, timeout)` - Wait for stream ready

---

## Phase 5: Comprehensive Diagnostics (Priority: MEDIUM)

### 5.1 Extended Diagnostics (`internal/diagnostics/`)

Implement all 24 checks from bash version:

**Package Structure:**
```
internal/diagnostics/
├── diagnostics.go      # Main runner
├── system.go           # OS, kernel, memory, CPU
├── audio.go            # ALSA, devices, conflicts
├── network.go          # Ports, TCP, connectivity
├── resources.go        # FDs, limits, disk
├── services.go         # Systemd, MediaMTX
├── logs.go             # Log analysis
└── report.go           # Output formatting
```

**Check Categories:**

1. **Prerequisites** - Bash version, required/optional tools
2. **Project Info** - Script versions, git status
3. **System Info** - OS, kernel, arch, CPU, memory, uptime
4. **USB Audio** - ALSA, cards, udev rules
5. **Audio Caps** - Formats, codecs, mixer
6. **MediaMTX** - Binary, config, ports, paths
7. **Stream Health** - Errors, warnings, patterns
8. **RTSP** - Connectivity, multicast
9. **Resources** - Memory, CPU, FDs per process
10. **Logs** - Size, errors, 24h patterns
11. **Limits** - FD limits, ports, processes
12. **Disk** - Usage per mount point
13. **Config** - YAML syntax, streams
14. **Time** - NTP, timezone
15. **Services** - Systemd states
16. **Permissions** - File access, ownership
17. **Stability** - Restarts, uptime, crashes
18. **Constraints** - inotify, TCP backlog, memory
19. **FD Leaks** - Usage percentages
20. **Audio Conflicts** - PulseAudio, locks
21. **inotify/Entropy** - Watch limits, pool
22. **Network** - Ephemeral ports, TIME-WAIT
23. **Process Health** - Per-stream metrics
24. **Summary** - Overall score, recommendations

**Output Modes:**
- `--mode=quick` - Essential checks only
- `--mode=full` - All checks (default)
- `--mode=debug` - Verbose with raw data
- `--format=text|json` - Output format
- `--export=PATH` - Save report

---

## Phase 6: Test Command (Priority: MEDIUM)

### 6.1 Config Test (`cmd/lyrebird/test.go`)

**Functions:**
- Validate config syntax
- Check device availability
- Test FFmpeg command (dry-run)
- Verify MediaMTX connectivity
- Test RTSP URL accessibility

**Output:**
```
Testing configuration: /etc/lyrebird/config.yaml

[✓] Config syntax valid
[✓] Device 'blue_yeti' available (hw:1,0)
[✓] FFmpeg command builds successfully
[✓] MediaMTX API reachable
[✓] RTSP port 8554 accessible

All tests passed!
```

---

## Phase 7: Version Management (Priority: LOW)

### 7.1 Updater (`cmd/lyrebird/update.go`)

**Commands:**
```
lyrebird update              # Update to latest stable
lyrebird update --check      # Check for updates
lyrebird update --list       # List available versions
lyrebird update --version=X  # Update to specific version
lyrebird update --rollback   # Rollback to previous
```

**Functions:**
- `CheckForUpdates()` - Compare with remote
- `ListVersions()` - Fetch tags/releases
- `Update(version)` - Download and install
- `Rollback()` - Restore previous version
- `BackupBinary()` - Before update

**Note:** For Go, this would download pre-built binaries from GitHub releases rather than git-based updates.

---

## Phase 8: Interactive Menu (Priority: LOW)

### 8.1 Orchestrator TUI (`cmd/lyrebird/menu.go`)

Simple terminal menu using raw terminal control (no external deps):

```
╔══════════════════════════════════════════╗
║     LyreBirdAudio Management Menu        ║
╠══════════════════════════════════════════╣
║  1. Quick Setup Wizard                   ║
║  2. Device Management                    ║
║  3. Stream Control                       ║
║  4. System Diagnostics                   ║
║  5. View Logs                            ║
║  6. Configuration                        ║
║  7. About / Version                      ║
║  0. Exit                                 ║
╚══════════════════════════════════════════╝
Select option:
```

**Submenus:**
- Device Management: List, detect caps, create rules
- Stream Control: Start, stop, restart, status
- Diagnostics: Quick, full, export
- Logs: MediaMTX, FFmpeg, orchestrator
- Configuration: Edit, validate, backup, restore

---

## Implementation Order

1. **Phase 1.1**: Audio capability detection (HIGH - core feature)
2. **Phase 1.2**: Quality tiers (HIGH - depends on 1.1)
3. **Phase 2.1**: Config backup/restore (HIGH - safety feature)
4. **Phase 3.1**: Resource monitoring (HIGH - reliability)
5. **Phase 3.2**: Log rotation (HIGH - disk safety)
6. **Phase 4.1**: MediaMTX API client (MEDIUM - health checks)
7. **Phase 6.1**: Test command (MEDIUM - user request)
8. **Phase 5.1**: Extended diagnostics (MEDIUM - debugging)
9. **Phase 2.2**: SIGHUP hot-reload (MEDIUM - convenience)
10. **Phase 7.1**: Updater (LOW - nice to have)
11. **Phase 8.1**: Interactive menu (LOW - nice to have)

---

## Testing Requirements

Each new component must have:
- Unit tests with >80% coverage
- Table-driven test cases
- Error path testing
- Mock dependencies (filesystem, network)
- Race condition tests where applicable

---

## Files to Create

```
internal/
├── audio/
│   ├── capabilities.go      # NEW
│   └── capabilities_test.go # NEW
├── config/
│   ├── quality.go           # NEW
│   ├── quality_test.go      # NEW
│   ├── backup.go            # NEW
│   └── backup_test.go       # NEW
├── stream/
│   ├── monitor.go           # NEW
│   ├── monitor_test.go      # NEW
│   ├── logrotate.go         # NEW
│   └── logrotate_test.go    # NEW
├── mediamtx/
│   ├── client.go            # NEW
│   └── client_test.go       # NEW
└── diagnostics/
    ├── diagnostics.go       # NEW
    ├── diagnostics_test.go  # NEW
    ├── checks.go            # NEW
    └── report.go            # NEW
```

---

## Estimated Effort

| Phase | Components | Complexity |
|-------|------------|------------|
| 1 | Capabilities, Quality | Medium |
| 2 | Backup, Hot-reload | Medium |
| 3 | Monitor, Log rotation | Medium |
| 4 | MediaMTX client | Low |
| 5 | Diagnostics (24 checks) | High |
| 6 | Test command | Low |
| 7 | Updater | Medium |
| 8 | Menu TUI | Medium |

---

*This plan will be updated as implementation progresses.*
