#!/usr/bin/env bash
# LyreBirdAudio Health Check Script — for use with systemd-timer
#
# GAP-5 / C-3: Minimal alerting via the health endpoint.
# This script checks the /healthz endpoint and sends a notification
# (email, webhook, or custom) when the status is not "healthy".
#
# Installation (systemd-timer method):
#
#   1. Copy this script to /usr/local/bin/lyrebird-health-check
#   2. chmod +x /usr/local/bin/lyrebird-health-check
#   3. Create /etc/systemd/system/lyrebird-health-check.service:
#
#       [Unit]
#       Description=LyreBird health check
#       After=lyrebird-stream.service
#
#       [Service]
#       Type=oneshot
#       ExecStart=/usr/local/bin/lyrebird-health-check
#
#   4. Create /etc/systemd/system/lyrebird-health-check.timer:
#
#       [Unit]
#       Description=Run LyreBird health check every 5 minutes
#
#       [Timer]
#       OnBootSec=60
#       OnUnitActiveSec=5m
#       AccuracySec=30
#
#       [Install]
#       WantedBy=timers.target
#
#   5. Enable: sudo systemctl enable --now lyrebird-health-check.timer
#
# Configuration (edit variables below):

HEALTH_URL="http://127.0.0.1:9998/healthz"
TIMEOUT=10                 # curl timeout in seconds
ALERT_CMD=""               # Command to run on alert (see examples below)
HOSTNAME_LABEL="$(hostname -s)"

# Alert command examples:
#   Email (requires mailutils):
#     ALERT_CMD="mail -s 'LyreBird alert on ${HOSTNAME_LABEL}' admin@example.com"
#   Slack webhook:
#     ALERT_CMD="curl -s -X POST -H 'Content-type: application/json' \
#       --data '{\"text\":\"LyreBird alert on ${HOSTNAME_LABEL}: '"'"'${STATUS}'"'"'\"}' \
#       https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
#   Simple log:
#     ALERT_CMD="logger -t lyrebird-alert"
#   ntfy.sh push notification:
#     ALERT_CMD="curl -s -d '${MESSAGE}' https://ntfy.sh/your-topic"

# --- Script logic (no edits needed below) ---

set -euo pipefail

response="$(curl -sf --max-time "${TIMEOUT}" "${HEALTH_URL}" 2>/dev/null || echo '')"

if [[ -z "${response}" ]]; then
    STATUS="endpoint_unreachable"
    MESSAGE="LyreBird health endpoint at ${HEALTH_URL} is not responding on ${HOSTNAME_LABEL}"
elif command -v jq &>/dev/null; then
    STATUS="$(echo "${response}" | jq -r '.status // "unknown"')"
    SERVICES="$(echo "${response}" | jq -r '[.services[]? | select(.healthy==false) | .name] | join(", ")' 2>/dev/null || echo '')"
    DISK_WARN="$(echo "${response}" | jq -r '.system.disk_low_warning // false')"
    NTP_SYNC="$(echo "${response}" | jq -r '.system.ntp_synced // true')"

    if [[ "${STATUS}" == "healthy" ]]; then
        echo "OK: lyrebird is healthy on ${HOSTNAME_LABEL}"
        exit 0
    fi

    DETAILS=""
    [[ -n "${SERVICES}" ]] && DETAILS+=" unhealthy_streams=[${SERVICES}]"
    [[ "${DISK_WARN}" == "true" ]] && DETAILS+=" disk_low=true"
    [[ "${NTP_SYNC}" == "false" ]] && DETAILS+=" ntp_desync=true"

    MESSAGE="LyreBird alert on ${HOSTNAME_LABEL}: status=${STATUS}${DETAILS}"
else
    # No jq available — basic string check
    if echo "${response}" | grep -q '"status":"healthy"'; then
        echo "OK: lyrebird is healthy on ${HOSTNAME_LABEL}"
        exit 0
    fi
    STATUS="not_healthy"
    MESSAGE="LyreBird alert on ${HOSTNAME_LABEL}: status is not healthy (install jq for details)"
fi

# Alert
echo "ALERT: ${MESSAGE}" >&2
logger -t "lyrebird-health-check" "${MESSAGE}"

if [[ -n "${ALERT_CMD}" ]]; then
    eval "${ALERT_CMD}" <<< "${MESSAGE}" 2>/dev/null || true
fi

exit 1
