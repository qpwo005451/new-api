#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
app_root="${APP_ROOT:-/opt/new-api}"
candidate_port="${CANDIDATE_PORT:-4003}"
release_id="${1:-${RELEASE_ID:-}}"
release_root="$repo_root/releases/$release_id"
runtime_root="$release_root/runtime"
candidate_bin="$release_root/bin/new-api"
manifest_path="$release_root/manifest.env"
copied_env="$runtime_root/live.env"
log_path="$runtime_root/candidate.log"
pid_path="$runtime_root/candidate.pid"
env_path="$runtime_root/candidate.env"
db_path="$runtime_root/new-api.db"
schema_before="$runtime_root/schema-before.sha256"
schema_after="$runtime_root/schema-after.sha256"
schema_changed_flag="$runtime_root/schema-changed.flag"
source_env="$app_root/.env"
source_db="$app_root/data/new-api.db"

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  if [ -f "$log_path" ]; then
    tail -40 "$log_path" >&2 || true
  fi
  exit 1
}

usage() {
  printf 'usage: stage_release_runtime.sh <release-id>\n' >&2
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

candidate_pid_owns_port() {
  local pid="$1"
  ss -ltnp '( sport = :4003 )' 2>/dev/null | grep -q "pid=$pid,"
}

candidate_pid_matches_binary() {
  local pid="$1"
  [ -e "/proc/$pid/exe" ] || return 1
  [ "$(readlink -f "/proc/$pid/exe")" = "$(readlink -f "$candidate_bin")" ]
}

stop_existing_candidate() {
  if [ ! -f "$pid_path" ]; then
    return 0
  fi
  local old_pid
  old_pid="$(cat "$pid_path" 2>/dev/null || true)"
  if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
    candidate_pid_owns_port "$old_pid" || fail "refusing to kill PID $old_pid because it does not own port 4003"
    candidate_pid_matches_binary "$old_pid" || fail "refusing to kill PID $old_pid because it is not $candidate_bin"
    kill "$old_pid" 2>/dev/null || true
    for _ in $(seq 1 10); do
      kill -0 "$old_pid" 2>/dev/null || break
      sleep 1
    done
  fi
  rm -f "$pid_path"
}

[ -n "$release_id" ] || usage
validate_release_id
[ ! -L "$repo_root/releases" ] || fail "refusing symlinked releases root: $repo_root/releases"
[ ! -L "$release_root" ] || fail "refusing symlinked release directory: $release_root"
[ ! -L "$runtime_root" ] || fail "refusing symlinked runtime directory: $runtime_root"
ensure_release_path "$release_root"
ensure_release_path "$runtime_root"
[ "$candidate_port" = "4003" ] || fail "candidate port must stay fixed at 4003"
[ -f "$manifest_path" ] || fail "missing release manifest: $manifest_path"
[ ! -L "$manifest_path" ] || fail "refusing symlinked release manifest: $manifest_path"
[ -x "$candidate_bin" ] || fail "missing candidate binary: $candidate_bin"
[ ! -L "$candidate_bin" ] || fail "refusing symlinked candidate binary: $candidate_bin"
[ -f "$source_env" ] || fail "missing source env: $source_env"
[ -f "$source_db" ] || fail "missing source db: $source_db"

mkdir -p "$runtime_root"
stop_existing_candidate

if ss -ltn '( sport = :4003 )' | grep -q LISTEN; then
  fail "port 4003 already in use"
fi

cp "$source_env" "$copied_env"
rm -f "$log_path" "$env_path" "$db_path" "$schema_before" "$schema_after" "$schema_changed_flag"
sqlite3 "$source_db" ".backup '$db_path'"
sqlite3 "$db_path" '.schema' | sha256sum | awk '{print $1}' > "$schema_before"

python3 - "$source_env" "$env_path" "$db_path" <<'PY'
from pathlib import Path
import sys

source_env = Path(sys.argv[1])
target_env = Path(sys.argv[2])
runtime_db = sys.argv[3]

lines = []
for raw_line in source_env.read_text(encoding="utf-8").splitlines():
    if raw_line.startswith(("PORT=", "SQL_DSN=", "LOG_SQL_DSN=", "SQLITE_PATH=")):
        continue
    lines.append(raw_line)

lines.append("PORT=4003")
lines.append("SQL_DSN=local")
lines.append(f"SQLITE_PATH={runtime_db}")
target_env.write_text("\n".join(lines) + "\n", encoding="utf-8")
PY

set -a
unset PORT SQL_DSN LOG_SQL_DSN SQLITE_PATH
# shellcheck disable=SC1090
source "$env_path"
set +a

cd "$runtime_root"
nohup "$candidate_bin" --port 4003 >"$log_path" 2>&1 &
printf '%s\n' "$!" > "$pid_path"
candidate_pid="$(cat "$pid_path")"

for _ in $(seq 1 30); do
  if ss -ltnp '( sport = :4003 )' | grep -q "pid=$candidate_pid,"; then
    break
  fi
  if ! kill -0 "$candidate_pid" 2>/dev/null; then
    fail "candidate exited before binding port 4003"
  fi
  sleep 1
done

if ! ss -ltnp '( sport = :4003 )' | grep -q "pid=$candidate_pid,"; then
  fail "candidate did not bind port 4003"
fi

sqlite3 "$db_path" '.schema' | sha256sum | awk '{print $1}' > "$schema_after"
if ! cmp -s "$schema_before" "$schema_after"; then
  touch "$schema_changed_flag"
fi

printf 'Staged candidate PID %s on port 4003\n' "$candidate_pid"
printf 'Candidate env: %s\n' "$env_path"
printf 'Candidate DB: %s\n' "$db_path"
