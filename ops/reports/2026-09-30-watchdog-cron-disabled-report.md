# 2026-09-30 Watchdog Cron Disabled On Host 251

Operator request: disable the `new-api` health watchdog.

## What Changed

Host `10.0.0.251`, root crontab:

- Before: `*/10 * * * * /opt/new-api/watchdog.sh >> /opt/new-api/watchdog.log 2>&1`
- After: `# DISABLED 2026-09-30 (relay-probe restart churn while upstream channels are unstable; uncomment to restore auto-repair): */10 * * * * /opt/new-api/watchdog.sh >> /opt/new-api/watchdog.log 2>&1`

The script itself (`/opt/new-api/watchdog.sh`), its environment file (`watchdog.env`) and its state directory
(`/opt/new-api/.watchdog/`) are left in place. The stale fail counter was reset to `0`. The channel-guard cron entry
(`*/2 * * * * ... channel_guard.py --rule input_budget`) is untouched.

Root crontab backup: `/opt/new-api/data/backups/crontab.root.20260930-055213.watchdog-disable.bak`.

## Why

The watchdog restarts `new-api` when its relay probe (`MODEL=glm-5.3-flash` against
`http://localhost:4002/v1/chat/completions`) fails with a transport error (HTTP 000) twice in a row. During the current
upstream channel instability that probe fails often enough to restart the service about every 20 minutes (8 restarts
between 02:51 and 05:21 on 2026-09-30, plus one at 05:41), even though the liveness probe kept returning 200. Each
restart costs a few seconds of downtime and can interrupt in-flight streams.

## Impact

- Still covered: a crashed process is restarted by systemd, because the unit keeps `Restart=on-failure` and
  `RestartSec=5`.
- No longer covered: detection and repair of a hung relay path with a live HTTP server, and the watchdog's
  n8n/Telegram relay alerts (including the "non-2xx relay answer" notifications that never restarted the service).
- Recovery is otherwise manual: check `systemctl status new-api` and `journalctl -u new-api`, and restart with
  `systemctl restart new-api`.

## Re-enabling

Either uncomment the cron line in the root crontab, or restore the backup:

```
crontab /opt/new-api/data/backups/crontab.root.20260930-055213.watchdog-disable.bak
```

If the goal is to keep the alerting but stop the restarts, set `FAIL_THRESHOLD` high in `/opt/new-api/watchdog.env`
instead of re-enabling the cron entry.

## Verification

- `crontab -l` shows zero active `watchdog.sh` entries and one active `channel_guard.py` entry; the cron daemon is
  active.
- No `/opt/new-api/watchdog.sh` process was running at the time of the change; `new-api.service` remained active, the
  MainPID (1698446, started by the 05:41 watchdog restart) was unchanged by this change, and `/api/status` returned 200.
- Last watchdog log line before the change: `Wed Sep 30 05:51:11 CST 2026 | FAIL 1/2 (relay transport failure (HTTP 000))`.
  That would have triggered a restart at about 06:01, which no longer happens.
