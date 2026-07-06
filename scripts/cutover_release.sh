#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
release_id="${1:-${RELEASE_ID:-}}"
app_root="${APP_ROOT:-/opt/new-api}"
live_binary="$app_root/new-api"
live_db="${LIVE_DB_PATH:-$app_root/data/new-api.db}"
prod_base_url="${PROD_BASE_URL:-http://127.0.0.1:4002}"
systemctl_bin="${SYSTEMCTL_BIN:-systemctl}"
auto_restore_db_on_failure="${AUTO_RESTORE_DB_ON_FAILURE:-auto}"
release_root="$repo_root/releases/$release_id"
candidate_bin="$release_root/bin/new-api"
runtime_root="$release_root/runtime"
manifest_path="$release_root/manifest.env"
backup_env="$runtime_root/cutover-backup.env"
ts="$(date -u +%Y%m%d-%H%M%S)"
backup_bin="$runtime_root/live-new-api.$ts.bak"
backup_db="$runtime_root/live-new-api.db.$ts.bak"

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

usage() {
  printf 'usage: cutover_release.sh <release-id>\n' >&2
  exit 1
}

restart_service() {
  if [ "$systemctl_bin" = "systemctl" ]; then
    systemctl restart new-api
  else
    "$systemctl_bin" restart new-api
  fi
}

restore_runtime_after_failure() {
  local restore_db="$1"

  install -m 755 "$backup_bin" "$live_binary"
  if [ "$restore_db" = "1" ]; then
    sqlite3 "$backup_db" ".backup '$live_db'"
  fi
  restart_service || true
}

[ -n "$release_id" ] || usage
[ -x "$candidate_bin" ] || fail "missing candidate binary: $candidate_bin"
[ -f "$manifest_path" ] || fail "missing release manifest: $manifest_path"
[ -f "$live_binary" ] || fail "missing live binary: $live_binary"
[ -f "$live_db" ] || fail "missing live database: $live_db"

mkdir -p "$runtime_root"
cp "$live_binary" "$backup_bin"
sqlite3 "$live_db" ".backup '$backup_db'"

previous_binary_sha256="$(sha256sum "$live_binary" | awk '{print $1}')"
candidate_binary_sha256="$(sha256sum "$candidate_bin" | awk '{print $1}')"
live_schema_sha256="$(sqlite3 "$live_db" '.schema' | sha256sum | awk '{print $1}')"

cat >"$backup_env" <<EOF
RELEASE_ID=$release_id
BACKUP_BIN=$backup_bin
BACKUP_DB=$backup_db
LIVE_BINARY=$live_binary
LIVE_DB=$live_db
PROD_BASE_URL=$prod_base_url
PREVIOUS_BINARY_SHA256=$previous_binary_sha256
CANDIDATE_BINARY_SHA256=$candidate_binary_sha256
LIVE_SCHEMA_SHA256=$live_schema_sha256
CREATED_AT=$(date -Iseconds)
EOF

install -m 755 "$candidate_bin" "$live_binary"

if ! restart_service; then
  restore_runtime_after_failure 0
  fail "systemctl restart new-api failed during cutover"
fi

if ! bash "$script_dir/smoke_release.sh" "$prod_base_url" "$live_db" fast; then
  restore_db="0"
  current_schema_sha256="$(sqlite3 "$live_db" '.schema' | sha256sum | awk '{print $1}')"
  case "$auto_restore_db_on_failure" in
    1)
      restore_db="1"
      ;;
    auto)
      if [ "$current_schema_sha256" != "$live_schema_sha256" ]; then
        restore_db="1"
      fi
      ;;
    0) ;;
    *)
      fail "AUTO_RESTORE_DB_ON_FAILURE must be auto, 1, or 0"
      ;;
  esac

  restore_runtime_after_failure "$restore_db"
  fail "post-cutover fast smoke failed; restored previous runtime"
fi

printf '%s\n' "$backup_env"
