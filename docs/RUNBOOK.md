# LyreBirdAudio-Go — Field Operator Runbook

**Audience**: Field technicians and non-developer operators
**Version**: Production (2026-03-02)
**Health endpoint**: `http://127.0.0.1:9998/healthz`

---

## Table of Contents

1. [Quick Checks](#quick-checks)
2. [No Devices Detected](#no-devices-detected)
3. [Stream is Stalled (bytes not increasing)](#stream-is-stalled)
4. [Low Disk Space](#low-disk-space)
5. [Config Hot-Reload (no downtime)](#config-hot-reload)
6. [Health Endpoint Reference](#health-endpoint-reference)
7. [Reading Structured Logs](#reading-structured-logs)
8. [Device Unreachable (SSH down)](#device-unreachable)
9. [Emergency Restart Procedure](#emergency-restart-procedure)
10. [Update Procedure](#update-procedure)

---

## Quick Checks

```bash
# Is the daemon running?
systemctl status lyrebird-stream

# Are there active streams?
lyrebird status

# What devices are detected?
lyrebird devices

# Full health (JSON)
curl -s http://127.0.0.1:9998/healthz | python3 -m json.tool

# Recent errors (last 50 lines)
journalctl -u lyrebird-stream -n 50 --no-pager

# Disk space
df -h /var/lib/lyrebird/recordings 2>/dev/null || df -h /
```

---

## No Devices Detected

**Symptom**: `lyrebird status` shows no streams, `lyrebird devices` returns empty list.

**Step 1 — Check physical connection**
```bash
# List all USB devices
lsusb

# List ALSA cards
cat /proc/asound/cards
```

**Step 2 — Check udev rules**
```bash
# Are rules installed?
ls -la /etc/udev/rules.d/99-usb-soundcards.rules

# If missing, recreate
sudo lyrebird usb-map
```

**Step 3 — Trigger hotplug detection**
```bash
# Daemon polls every 10s automatically.
# Force immediate re-detection via SIGHUP:
sudo systemctl reload lyrebird-stream
```

**Step 4 — Check daemon logs**
```bash
journalctl -u lyrebird-stream --since "5 min ago" | grep -i "device\|detect\|error"
```

**Step 5 — Replug device**
Physically unplug and replug the USB microphone. The daemon detects the change within 10 seconds.

---

## Stream is Stalled

**Symptom**: `lyrebird status` shows stream as running but RTSP clients get no audio, or bytes counter is not increasing.

**Step 1 — Check health endpoint**
```bash
curl -s http://127.0.0.1:9998/healthz | python3 -m json.tool
# Look for "healthy": false or "state": "failed"
```

**Step 2 — Check stall detection logs**
```bash
# Stall detection runs every 60s, triggers restart after 3 consecutive stalls (3 min)
journalctl -u lyrebird-stream | grep "stall\|stalled\|bytes"
```

**Step 3 — Manual restart**
```bash
# Restart the stream without full daemon restart
sudo systemctl reload lyrebird-stream   # Sends SIGHUP, triggers device re-scan

# If that doesn't work, restart the daemon
sudo systemctl restart lyrebird-stream
```

**Step 4 — Check MediaMTX**
```bash
systemctl status mediamtx
# If MediaMTX is down, streams buffer until it restarts (FFmpeg has reconnect flags)
sudo systemctl restart mediamtx
```

**Step 5 — Check FFmpeg logs**
```bash
ls -la /var/log/lyrebird/
# View the latest FFmpeg log for your device
tail -100 /var/log/lyrebird/<device_name>_*.log
```

---

## Low Disk Space

**Symptom**: `journalctl` shows `LOW DISK SPACE WARNING`, or `/healthz` returns `"disk_low_warning": true`.

**Step 1 — Check current usage**
```bash
df -h /var/lib/lyrebird/recordings
du -sh /var/lib/lyrebird/recordings/*
ls -lt /var/lib/lyrebird/recordings/ | head -20
```

**Step 2 — Delete oldest recordings manually**
```bash
# Delete files older than 7 days
find /var/lib/lyrebird/recordings -mtime +7 -delete
```

**Step 3 — Configure automatic retention**

Edit `/etc/lyrebird/config.yaml`:
```yaml
stream:
  segment_max_age: 168h          # Auto-delete segments older than 7 days
  segment_max_total_bytes: 32000000000  # Cap at 32 GB total
```

Then reload:
```bash
sudo systemctl reload lyrebird-stream
```

**Step 4 — Check for orphaned large files**
```bash
find /var/lib/lyrebird -size +1G -ls
```

---

## Config Hot-Reload

Reload configuration **without stopping streams** (streams continue uninterrupted unless their parameters changed):

```bash
# Edit config
sudo nano /etc/lyrebird/config.yaml

# Reload (no downtime)
sudo systemctl reload lyrebird-stream
# or: sudo kill -HUP $(systemctl show -p MainPID lyrebird-stream | cut -d= -f2)
```

Streams restart only if their encoding parameters, RTSP URL, or recording path changed. Device detection, monitoring intervals, and disk thresholds update without restart.

---

## Health Endpoint Reference

The health endpoint at `http://127.0.0.1:9998/healthz` returns JSON with:

| Field | Type | Meaning |
|-------|------|---------|
| `status` | `"healthy"` / `"unhealthy"` / `"degraded"` | Overall status |
| `services[].state` | string | `running`, `starting`, `failed`, `stopped` |
| `services[].healthy` | bool | `true` only when `state == "running"` |
| `services[].restarts` | int | Total supervisor-level restarts |
| `system.disk_free_bytes` | int | Free bytes on recording filesystem |
| `system.disk_low_warning` | bool | `true` when below `disk_low_threshold_mb` |
| `system.ntp_synced` | bool | `true` when clock is NTP-synchronized |

HTTP response codes:
- `200 OK` → healthy
- `503 Service Unavailable` → unhealthy or degraded

Prometheus metrics at `http://127.0.0.1:9998/metrics`:
```
lyrebird_stream_healthy{stream="blue_yeti"} 1
lyrebird_stream_uptime_seconds{stream="blue_yeti"} 3600.000
lyrebird_stream_restarts_total{stream="blue_yeti"} 0
lyrebird_stream_failures_total{stream="blue_yeti"} 0
lyrebird_disk_free_bytes 42949672960
lyrebird_disk_low_warning 0
lyrebird_ntp_synced 1
```

---

## Reading Structured Logs

The daemon emits structured `stream_event` log lines parseable by `jq`:

```bash
# Show all stream events
journalctl -u lyrebird-stream -o json | \
  jq 'select(.MESSAGE | startswith("stream_event"))' | \
  jq '{time: .SYSLOG_TIMESTAMP, msg: .MESSAGE}'

# Show failures only
journalctl -u lyrebird-stream | grep "stream_failure\|stream_short_run"

# Count restarts per device
journalctl -u lyrebird-stream | grep "stream_recovery" | awk '{print $NF}' | sort | uniq -c
```

Key event types:
- `stream_event` — general status
- `stream_failure` — FFmpeg exited with error
- `stream_short_run_failure` — FFmpeg exited too quickly (< threshold)
- `stream_recovery` — stream recovered after failure

---

## Device Unreachable

When the Raspberry Pi is not responding to SSH:

1. **Check network**: Ping the device IP. If it doesn't respond, the Pi may be hung or powered off.

2. **Physical check**: If accessible, check:
   - Power LED is on
   - Activity LED is blinking (indicates OS activity)
   - USB devices are plugged in

3. **Hard power cycle**: Disconnect power for 10 seconds. The daemon auto-starts via systemd on boot.

4. **After reconnect**: Check that the daemon auto-restarted:
   ```bash
   systemctl status lyrebird-stream
   journalctl -u lyrebird-stream --since boot
   ```

5. **systemd watchdog**: If `WatchdogSec=90s` is configured (default), systemd automatically restarts the daemon if it stops sending heartbeats — even during Go runtime deadlocks.

---

## Emergency Restart Procedure

```bash
# 1. Stop daemon
sudo systemctl stop lyrebird-stream

# 2. Kill any orphaned FFmpeg processes
sudo pkill -9 ffmpeg || true

# 3. Remove stale lock files
sudo rm -f /var/run/lyrebird/*.lock

# 4. Start daemon
sudo systemctl start lyrebird-stream

# 5. Verify
sleep 5
lyrebird status
curl -s http://127.0.0.1:9998/healthz | python3 -m json.tool
```

---

## Update Procedure

```bash
# Check for available updates
lyrebird update --check

# Install update (requires root; creates rollback backup)
sudo lyrebird update

# Verify new version
lyrebird --version

# Rollback if needed (the old binary is saved automatically)
# See update output for rollback path
```
