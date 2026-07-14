#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
release_id="${1:-${RELEASE_ID:-}}"
app_root="${APP_ROOT:-/opt/new-api}"
live_binary="${LIVE_BINARY:-$app_root/new-api}"
release_root="$repo_root/releases/$release_id"
runtime_root="$release_root/runtime"
candidate_bin="$release_root/bin/new-api"
manifest_path="$release_root/manifest.env"
pid_path="$runtime_root/candidate.pid"
finalized_path="$release_root/finalized.env"

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

usage() {
  printf 'usage: finalize_release.sh <release-id>\n' >&2
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

manifest_value() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$manifest_path"
}

candidate_pid_owns_port() {
  local pid="$1"
  ss -ltnp '( sport = :4003 )' 2>/dev/null | grep -q "pid=$pid,"
}

candidate_pid_matches_binary() {
  local pid="$1"
  [ -e "/proc/$pid/exe" ] || return 1
  [ "$(readlink -f "/proc/$pid/exe")" = "$(readlink -f "$candidate_bin")" ]
}

stop_candidate() {
  if [ -f "$pid_path" ]; then
    local pid
    pid="$(cat "$pid_path" 2>/dev/null || true)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      candidate_pid_owns_port "$pid" || fail "refusing to kill PID $pid because it does not own port 4003"
      candidate_pid_matches_binary "$pid" || fail "refusing to kill PID $pid because it is not $candidate_bin"
      kill "$pid"
      for _ in $(seq 1 10); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 1
      done
      kill -0 "$pid" 2>/dev/null && fail "candidate PID $pid did not stop"
    fi
    rm -f "$pid_path"
  fi

  if ss -ltn '( sport = :4003 )' 2>/dev/null | grep -q LISTEN; then
    fail "port 4003 is still in use; refusing to finalize"
  fi
}

cleanup_source_worktree() {
  local src_dir="$release_root/src"

  ensure_release_path "$src_dir"
  if git -C "$repo_root" worktree list --porcelain | grep -Fqx "worktree $src_dir"; then
    git -C "$repo_root" worktree remove --force "$src_dir"
  elif [ -e "$src_dir" ]; then
    rm -rf "$src_dir"
  fi
  git -C "$repo_root" worktree prune
}

[ -n "$release_id" ] || usage
validate_release_id
[ ! -L "$repo_root/releases" ] || fail "refusing symlinked releases root: $repo_root/releases"
[ ! -L "$release_root" ] || fail "refusing symlinked release directory: $release_root"
[ ! -L "$runtime_root" ] || fail "refusing symlinked runtime directory: $runtime_root"
ensure_release_path "$release_root"
ensure_release_path "$runtime_root"
[ -f "$manifest_path" ] || fail "missing release manifest: $manifest_path"
[ ! -L "$manifest_path" ] || fail "refusing symlinked release manifest: $manifest_path"
[ -x "$candidate_bin" ] || fail "missing candidate binary: $candidate_bin"
[ ! -L "$candidate_bin" ] || fail "refusing symlinked candidate binary: $candidate_bin"
[ -f "$live_binary" ] || fail "missing live binary: $live_binary"
[ ! -L "$live_binary" ] || fail "refusing symlinked live binary: $live_binary"

expected_sha256="$(manifest_value BINARY_SHA256)"
[ -n "$expected_sha256" ] || fail "release manifest missing BINARY_SHA256"
candidate_sha256="$(sha256sum "$candidate_bin" | awk '{print $1}')"
live_sha256="$(sha256sum "$live_binary" | awk '{print $1}')"
[ "$candidate_sha256" = "$expected_sha256" ] || fail "candidate binary hash does not match release manifest"
[ "$live_sha256" = "$expected_sha256" ] || fail "live binary does not match release candidate; refusing to finalize"

stop_candidate
cleanup_source_worktree

rm -f \
  "$runtime_root/candidate.env" \
  "$runtime_root/candidate.log" \
  "$runtime_root/live.env" \
  "$runtime_root/new-api.db" \
  "$runtime_root/new-api.db-shm" \
  "$runtime_root/new-api.db-wal" \
  "$runtime_root/schema-before.sha256" \
  "$runtime_root/schema-after.sha256" \
  "$runtime_root/schema-changed.flag"
rm -rf "$runtime_root/logs"

cat >"$finalized_path" <<EOF
RELEASE_ID=$release_id
LIVE_BINARY=$live_binary
LIVE_BINARY_SHA256=$live_sha256
FINALIZED_AT=$(date -Iseconds)
EOF

printf 'Release finalized: %s\n' "$release_root"
printf 'Preserved candidate binary: %s\n' "$candidate_bin"
if [ -f "$runtime_root/cutover-backup.env" ]; then
  printf 'Preserved rollback metadata: %s\n' "$runtime_root/cutover-backup.env"
fi
