#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

required_scripts=(
  build_release_candidate.sh
  stage_release_runtime.sh
  smoke_release.sh
  cutover_release.sh
  rollback_release.sh
)

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

tmp_root=""
stage_pid_path=""
stage_release_root=""
symlink_release_root=""
cleanup() {
  if [ -n "$stage_pid_path" ] && [ -f "$stage_pid_path" ]; then
    stage_pid="$(cat "$stage_pid_path" 2>/dev/null || true)"
    if [ -n "$stage_pid" ]; then
      kill "$stage_pid" 2>/dev/null || true
    fi
  fi
  if [ -n "$stage_release_root" ]; then
    rm -rf "$stage_release_root"
  fi
  if [ -n "$symlink_release_root" ]; then
    rm -rf "$symlink_release_root"
  fi
  if [ -n "$tmp_root" ]; then
    rm -rf "$tmp_root"
  fi
}
trap cleanup EXIT

assert_contains() {
  local file="$1"
  local pattern="$2"
  grep -Fq "$pattern" "$file" || fail "missing pattern in $file: $pattern"
}

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  if grep -Fq "$pattern" "$file"; then
    fail "unexpected pattern in $file: $pattern"
  fi
}

assert_fails_with() {
  local expected="$1"
  shift
  local output
  set +e
  output="$("$@" 2>&1)"
  local code="$?"
  set -e
  [ "$code" -ne 0 ] || fail "command unexpectedly succeeded: $*"
  printf '%s' "$output" | grep -Fq "$expected" || fail "command did not fail with expected text '$expected': $output"
}

for script_name in "${required_scripts[@]}"; do
  script_path="$script_dir/$script_name"
  [ -f "$script_path" ] || fail "missing script: $script_path"
  [ -x "$script_path" ] || fail "script is not executable: $script_path"
  bash -n "$script_path"
  assert_contains "$script_path" "set -euo pipefail"
done

assert_contains "$script_dir/build_release_candidate.sh" "worktree add --detach"
assert_contains "$script_dir/build_release_candidate.sh" "RELEASE_TAG"
assert_contains "$script_dir/build_release_candidate.sh" "BINARY_SHA256"
assert_contains "$script_dir/build_release_candidate.sh" "LOCAL_OPTION_OVERRIDES_SHA256"
assert_contains "$script_dir/build_release_candidate.sh" "validate_release_id"
assert_contains "$script_dir/build_release_candidate.sh" "realpath -m"

assert_contains "$script_dir/stage_release_runtime.sh" "PORT=4003"
assert_contains "$script_dir/stage_release_runtime.sh" "SQLITE_PATH="
assert_contains "$script_dir/stage_release_runtime.sh" "candidate.pid"
assert_contains "$script_dir/stage_release_runtime.sh" "validate_release_id"
assert_contains "$script_dir/stage_release_runtime.sh" "SQL_DSN=local"
assert_contains "$script_dir/stage_release_runtime.sh" "LOG_SQL_DSN="
assert_contains "$script_dir/stage_release_runtime.sh" "candidate_pid_owns_port"
assert_contains "$script_dir/stage_release_runtime.sh" "candidate_pid_matches_binary"
assert_contains "$script_dir/stage_release_runtime.sh" "unset PORT SQL_DSN LOG_SQL_DSN SQLITE_PATH"
assert_contains "$script_dir/stage_release_runtime.sh" "refusing symlinked candidate binary"

assert_contains "$script_dir/smoke_release.sh" "/api/status"
assert_contains "$script_dir/smoke_release.sh" "/v1/models"
assert_contains "$script_dir/smoke_release.sh" "/v1/chat/completions"
assert_contains "$script_dir/smoke_release.sh" "/v1/responses"
assert_contains "$script_dir/smoke_release.sh" "MODELS_JSON"
assert_contains "$script_dir/smoke_release.sh" "validate_release_id"
assert_contains "$script_dir/smoke_release.sh" "realpath -m"
assert_not_contains "$script_dir/smoke_release.sh" "printf '%s' \"\$models_json\" | python3"
assert_not_contains "$script_dir/smoke_release.sh" "printf '%s' \"\$models_json\" | model_present"

assert_contains "$script_dir/cutover_release.sh" "cutover-backup.env"
assert_contains "$script_dir/cutover_release.sh" "restart new-api"
assert_contains "$script_dir/cutover_release.sh" "smoke_release.sh"
assert_contains "$script_dir/cutover_release.sh" "validate_release_id"
assert_contains "$script_dir/cutover_release.sh" "realpath -m"
assert_contains "$script_dir/cutover_release.sh" "refusing symlinked live database"

assert_contains "$script_dir/rollback_release.sh" "cutover-backup.env"
assert_contains "$script_dir/rollback_release.sh" "RESTORE_DB"
assert_contains "$script_dir/rollback_release.sh" "restart new-api"

tmp_root="$(mktemp -d)"
fake_bin="$tmp_root/bin"
mkdir -p "$fake_bin"

cat >"$fake_bin/sqlite3" <<'EOF'
#!/usr/bin/env bash
printf 'smoke-token\n'
EOF
chmod +x "$fake_bin/sqlite3"

cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
last="${@: -1}"
case "$last" in
  */v1/models)
    printf '{"data":[{"id":"gpt-smoke"}]}'
    ;;
  */v1/chat/completions|*/v1/responses)
    printf '{}'
    ;;
  *)
    printf '{}'
    ;;
esac
EOF
chmod +x "$fake_bin/curl"

touch "$tmp_root/smoke.db"
PATH="$fake_bin:$PATH" SMOKE_MODEL=gpt-smoke "$script_dir/smoke_release.sh" "http://fake.local" "$tmp_root/smoke.db" full >/dev/null

stage_release_id="test-helper-stage-$$"
stage_release_root="$repo_root/releases/$stage_release_id"
stage_runtime_root="$stage_release_root/runtime"
stage_pid_path="$stage_runtime_root/candidate.pid"
stage_app_root="$tmp_root/app"
mkdir -p "$stage_release_root/bin" "$stage_app_root/data"

cat >"$stage_release_root/bin/new-api" <<'EOF'
#!/usr/bin/env bash
if [ -n "${STAGE_ENV_DUMP:-}" ]; then
  env >"$STAGE_ENV_DUMP"
fi
sleep 60
EOF
chmod +x "$stage_release_root/bin/new-api"
printf 'RELEASE_ID=%s\n' "$stage_release_id" >"$stage_release_root/manifest.env"

cat >"$stage_app_root/.env" <<'EOF'
PORT=4002
SQL_DSN=postgres://live-db.example/newapi
LOG_SQL_DSN=mysql://live-log.example/newapi
SQLITE_PATH=/opt/new-api/data/new-api.db
SESSION_SECRET=test
EOF
touch "$stage_app_root/data/new-api.db"
stage_env_dump="$tmp_root/stage-env-dump.txt"

cat >"$fake_bin/sqlite3" <<'EOF'
#!/usr/bin/env bash
if [ "${2:-}" = ".schema" ]; then
  printf 'create table test(id integer);\n'
  exit 0
fi
case "${2:-}" in
  ".backup "*)
    target="${2#".backup "}"
    target="${target#\'}"
    target="${target%\'}"
    mkdir -p "$(dirname "$target")"
    cp "$1" "$target"
    ;;
esac
EOF
chmod +x "$fake_bin/sqlite3"

cat >"$fake_bin/ss" <<EOF
#!/usr/bin/env bash
pid_file="$stage_pid_path"
if [ -f "\$pid_file" ]; then
  pid="\$(cat "\$pid_file")"
  if kill -0 "\$pid" 2>/dev/null; then
    printf 'LISTEN 0 4096 0.0.0.0:4003 0.0.0.0:* users:(("new-api",pid=%s,fd=3))\\n' "\$pid"
  fi
fi
EOF
chmod +x "$fake_bin/ss"

PATH="$fake_bin:$PATH" APP_ROOT="$stage_app_root" SQL_DSN="postgres://inherited.example/live" LOG_SQL_DSN="mysql://inherited.example/log" STAGE_ENV_DUMP="$stage_env_dump" "$script_dir/stage_release_runtime.sh" "$stage_release_id" >/dev/null

candidate_env="$stage_runtime_root/candidate.env"
expected_sqlite_path="$stage_runtime_root/new-api.db"
if command -v cygpath >/dev/null 2>&1; then
  expected_sqlite_path="$(cygpath -m "$expected_sqlite_path")"
fi

grep -Fxq "PORT=4003" "$candidate_env" || fail "candidate env did not force port 4003"
grep -Fxq "SQL_DSN=local" "$candidate_env" || fail "candidate env did not force SQL_DSN=local"
grep -Fxq "SQLITE_PATH=$expected_sqlite_path" "$candidate_env" || fail "candidate env did not point at runtime SQLite DB"
if grep -Fq "postgres://live-db.example" "$candidate_env" || grep -Fq "mysql://live-log.example" "$candidate_env"; then
  fail "candidate env kept a live external database DSN"
fi
grep -Fxq "SQL_DSN=local" "$stage_env_dump" || fail "candidate process did not inherit SQL_DSN=local"
if grep -Fq "LOG_SQL_DSN=" "$stage_env_dump" || grep -Fq "inherited.example" "$stage_env_dump"; then
  fail "candidate process inherited an external database DSN"
fi

symlink_release_id="test-helper-symlink-$$"
symlink_release_root="$repo_root/releases/$symlink_release_id"
mkdir -p "$symlink_release_root/bin" "$tmp_root/symlink-target"
printf 'RELEASE_ID=%s\n' "$symlink_release_id" >"$symlink_release_root/manifest.env"
cp "$stage_release_root/bin/new-api" "$symlink_release_root/bin/new-api"
chmod +x "$symlink_release_root/bin/new-api"
ln -s "$tmp_root/symlink-target" "$symlink_release_root/runtime"
if [ -L "$symlink_release_root/runtime" ]; then
  assert_fails_with "refusing symlinked runtime directory" env PATH="$fake_bin:$PATH" APP_ROOT="$stage_app_root" "$script_dir/stage_release_runtime.sh" "$symlink_release_id"
fi
rm -rf "$symlink_release_root"

assert_fails_with "release id may only contain" "$script_dir/build_release_candidate.sh" "../bad" HEAD
assert_fails_with "release id may only contain" "$script_dir/stage_release_runtime.sh" "../bad"
assert_fails_with "release id may only contain" env RELEASE_ID="../bad" "$script_dir/smoke_release.sh" "http://fake.local"
assert_fails_with "release id may only contain" "$script_dir/cutover_release.sh" "../bad"

printf 'release-helper-contracts:PASS\n'
