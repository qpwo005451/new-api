#!/usr/bin/env bash
set -euo pipefail

backup_env="${1:-}"
app_root="${APP_ROOT:-/opt/new-api}"
data_root="${DATA_ROOT:-/opt/new-api/data}"
health_url="${HEALTH_URL:-http://127.0.0.1:4002/api/status}"
restore_db_mode="${RESTORE_DB:-auto}"
systemctl_bin="${SYSTEMCTL_BIN:-systemctl}"

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

usage() {
  printf 'usage: rollback_release.sh <release-runtime>/cutover-backup.env\n' >&2
  exit 1
}

stop_service() {
  if [ "$systemctl_bin" = "systemctl" ]; then
    systemctl stop new-api
  else
    "$systemctl_bin" stop new-api
  fi
}

restart_service() {
  if [ "$systemctl_bin" = "systemctl" ]; then
    systemctl restart new-api
  else
    "$systemctl_bin" restart new-api
  fi
}

[ -n "$backup_env" ] || usage
[ -f "$backup_env" ] || fail "missing cutover-backup.env: $backup_env"
# shellcheck disable=SC1090
source "$backup_env"

[ -n "${BACKUP_BIN:-}" ] || fail "cutover-backup.env missing BACKUP_BIN"
[ -n "${BACKUP_DB:-}" ] || fail "cutover-backup.env missing BACKUP_DB"
[ -n "${LIVE_BINARY:-}" ] || LIVE_BINARY="$app_root/new-api"
[ -n "${LIVE_DB:-}" ] || LIVE_DB="$data_root/new-api.db"
[ -f "$BACKUP_BIN" ] || fail "missing backup binary: $BACKUP_BIN"
[ -f "$BACKUP_DB" ] || fail "missing backup database: $BACKUP_DB"
[ -f "$LIVE_DB" ] || fail "missing live database: $LIVE_DB"

install -m 755 "$BACKUP_BIN" "$LIVE_BINARY"

restore_db="0"
case "$restore_db_mode" in
  1)
    restore_db="1"
    ;;
  0)
    restore_db="0"
    ;;
  auto)
    current_schema="$(sqlite3 "$LIVE_DB" '.schema' | sha256sum | awk '{print $1}')"
    if [ -n "${LIVE_SCHEMA_SHA256:-}" ] && [ "${LIVE_SCHEMA_SHA256:-}" != "$current_schema" ]; then
      restore_db="1"
    fi
    ;;
  *)
    fail "RESTORE_DB must be auto, 1, or 0"
    ;;
esac

if [ "$restore_db" = "1" ]; then
  stop_service
  sqlite3 "$BACKUP_DB" ".backup '$LIVE_DB'"
fi

restart_service
curl --connect-timeout 3 --max-time 8 -fsS "$health_url" >/dev/null
printf 'rollback ok: %s\n' "$backup_env"
