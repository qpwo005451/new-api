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
service_stopped="0"
runtime_replaced="0"

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

usage() {
  printf 'usage: cutover_release.sh <release-id>\n' >&2
  exit 1
}

validate_release_id() {
  case "$release_id" in
    "."|".."|*[!A-Za-z0-9._-]*)
      fail "release id may only contain letters, digits, dot, underscore, and dash"
      ;;
  esac
}

ensure_release_path() {
  local releases_real target_real
  releases_real="$(realpath -m "$repo_root/releases")"
  target_real="$(realpath -m "$1")"
  case "$target_real/" in
    "$releases_real/"*) ;;
    *)
      fail "refusing to operate outside $repo_root/releases: $1"
      ;;
  esac
}

restart_service() {
  if [ "$systemctl_bin" = "systemctl" ]; then
    if ! systemctl restart new-api; then
      return 1
    fi
  else
    if ! "$systemctl_bin" restart new-api; then
      return 1
    fi
  fi
  service_stopped="0"
}

stop_service() {
  if [ "$systemctl_bin" = "systemctl" ]; then
    if ! systemctl stop new-api; then
      return 1
    fi
  else
    if ! "$systemctl_bin" stop new-api; then
      return 1
    fi
  fi
  service_stopped="1"
}

start_service() {
  if [ "$systemctl_bin" = "systemctl" ]; then
    if ! systemctl start new-api; then
      return 1
    fi
  else
    if ! "$systemctl_bin" start new-api; then
      return 1
    fi
  fi
  service_stopped="0"
}

wait_for_http_ready() {
  local attempt

  for ((attempt = 1; attempt <= 30; attempt++)); do
    if curl --connect-timeout 1 --max-time 2 -fsS "$prod_base_url/api/status" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  return 1
}

restore_runtime_after_failure() {
  local restore_db="$1"

  stop_service
  install -m 755 "$backup_bin" "$live_binary"
  runtime_replaced="0"
  if [ "$restore_db" = "1" ]; then
    sqlite3 "$backup_db" ".backup '$live_db'"
  fi
  restart_service
}

recover_on_exit() {
  local exit_code="$?"
  trap - EXIT
  if [ "$exit_code" -ne 0 ]; then
    if [ "$runtime_replaced" = "1" ] && [ -f "$backup_bin" ]; then
      install -m 755 "$backup_bin" "$live_binary" || true
      runtime_replaced="0"
    fi
    if [ "$service_stopped" = "1" ]; then
      start_service || true
    fi
  fi
  exit "$exit_code"
}

trap recover_on_exit EXIT

[ -n "$release_id" ] || usage
validate_release_id
[ ! -L "$repo_root/releases" ] || fail "refusing symlinked releases root: $repo_root/releases"
[ ! -L "$release_root" ] || fail "refusing symlinked release directory: $release_root"
[ ! -L "$runtime_root" ] || fail "refusing symlinked runtime directory: $runtime_root"
ensure_release_path "$release_root"
ensure_release_path "$runtime_root"
[ -x "$candidate_bin" ] || fail "missing candidate binary: $candidate_bin"
[ ! -L "$candidate_bin" ] || fail "refusing symlinked candidate binary: $candidate_bin"
[ -f "$manifest_path" ] || fail "missing release manifest: $manifest_path"
[ ! -L "$manifest_path" ] || fail "refusing symlinked release manifest: $manifest_path"
[ -f "$live_binary" ] || fail "missing live binary: $live_binary"
[ ! -L "$live_binary" ] || fail "refusing symlinked live binary: $live_binary"
[ -f "$live_db" ] || fail "missing live database: $live_db"
[ ! -L "$live_db" ] || fail "refusing symlinked live database: $live_db"

mkdir -p "$runtime_root"
cp "$live_binary" "$backup_bin"
previous_binary_sha256="$(sha256sum "$live_binary" | awk '{print $1}')"
candidate_binary_sha256="$(sha256sum "$candidate_bin" | awk '{print $1}')"

stop_service
sqlite3 "$live_db" ".timeout 30000" ".backup '$backup_db'"

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
runtime_replaced="1"

if ! restart_service; then
  restore_runtime_after_failure 0 || true
  fail "systemctl restart new-api failed during cutover"
fi

if ! wait_for_http_ready; then
  restore_runtime_after_failure 0 || true
  fail "new-api did not become ready after cutover; restored previous runtime"
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

  restore_runtime_after_failure "$restore_db" || true
  fail "post-cutover fast smoke failed; restored previous runtime"
fi

runtime_replaced="0"
printf '%s\n' "$backup_env"
