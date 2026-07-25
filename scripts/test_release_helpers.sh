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
  finalize_release.sh
)

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

tmp_root=""
stage_pid_path=""
stage_release_root=""
symlink_release_root=""
build_release_root=""
build_release_src=""
cached_build_release_root=""
failed_build_release_root=""
frontend_cache_root=""
cutover_release_root=""
finalize_release_root=""
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
  if [ -n "$build_release_src" ]; then
    git -C "$repo_root" worktree remove --force "$build_release_src" >/dev/null 2>&1 || true
    git -C "$repo_root" worktree prune
  fi
  if [ -n "$build_release_root" ]; then
    rm -rf "$build_release_root"
  fi
  if [ -n "$cached_build_release_root" ]; then
    rm -rf "$cached_build_release_root"
  fi
  if [ -n "$failed_build_release_root" ]; then
    rm -rf "$failed_build_release_root"
  fi
  if [ -n "$frontend_cache_root" ]; then
    rm -rf "$frontend_cache_root"
  fi
  if [ -n "$cutover_release_root" ]; then
    rm -rf "$cutover_release_root"
  fi
  if [ -n "$finalize_release_root" ]; then
    rm -rf "$finalize_release_root"
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
assert_contains "$script_dir/build_release_candidate.sh" "WEB_LOCK_SHA256"
assert_contains "$script_dir/build_release_candidate.sh" "validate_release_id"
assert_contains "$script_dir/build_release_candidate.sh" "realpath -m"
assert_contains "$script_dir/build_release_candidate.sh" "build_embed_assets"
assert_contains "$script_dir/build_release_candidate.sh" "install --frozen-lockfile"
assert_contains "$script_dir/build_release_candidate.sh" "cleanup_source_worktree_on_exit"
assert_contains "$script_dir/build_release_candidate.sh" "FRONTEND_CACHE_ROOT"
assert_contains "$script_dir/build_release_candidate.sh" "FRONTEND_CACHE_HIT"
assert_contains "$script_dir/build_release_candidate.sh" "store_embed_assets_in_cache"
assert_not_contains "$script_dir/build_release_candidate.sh" "SOURCE_APP_ROOT"
assert_not_contains "$script_dir/build_release_candidate.sh" "sync_embed_assets"

assert_contains "$script_dir/stage_release_runtime.sh" "PORT=4003"
assert_contains "$script_dir/stage_release_runtime.sh" "SQLITE_PATH="
assert_contains "$script_dir/stage_release_runtime.sh" "candidate.pid"
assert_contains "$script_dir/stage_release_runtime.sh" "validate_release_id"
assert_contains "$script_dir/stage_release_runtime.sh" "SQL_DSN=local"
assert_contains "$script_dir/stage_release_runtime.sh" "LOG_SQL_DSN="
assert_contains "$script_dir/stage_release_runtime.sh" ".timeout 5000"
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
assert_contains "$script_dir/smoke_release.sh" ".timeout 5000"
assert_not_contains "$script_dir/smoke_release.sh" "printf '%s' \"\$models_json\" | python3"
assert_not_contains "$script_dir/smoke_release.sh" "printf '%s' \"\$models_json\" | model_present"

assert_contains "$script_dir/cutover_release.sh" "cutover-backup.env"
assert_contains "$script_dir/cutover_release.sh" "restart new-api"
assert_contains "$script_dir/cutover_release.sh" "smoke_release.sh"
assert_contains "$script_dir/cutover_release.sh" "validate_release_id"
assert_contains "$script_dir/cutover_release.sh" "realpath -m"
assert_contains "$script_dir/cutover_release.sh" "refusing symlinked live database"
assert_contains "$script_dir/cutover_release.sh" "wait_for_http_ready"

assert_contains "$script_dir/rollback_release.sh" "cutover-backup.env"
assert_contains "$script_dir/rollback_release.sh" "RESTORE_DB"
assert_contains "$script_dir/rollback_release.sh" "restart new-api"

assert_contains "$script_dir/finalize_release.sh" "live binary does not match release candidate"
assert_contains "$script_dir/finalize_release.sh" "cleanup_source_worktree"
assert_contains "$script_dir/finalize_release.sh" "candidate.pid"
assert_contains "$script_dir/finalize_release.sh" "cutover-backup.env"
assert_contains "$script_dir/finalize_release.sh" "finalized.env"

tmp_root="$(mktemp -d)"
fake_bin="$tmp_root/bin"
mkdir -p "$fake_bin"

build_release_id="test-helper-build-$$"
build_release_root="$repo_root/releases/$build_release_id"
build_release_src="$build_release_root/src"
cached_build_release_id="test-helper-build-cache-hit-$$"
cached_build_release_root="$repo_root/releases/$cached_build_release_id"
frontend_cache_root="$tmp_root/frontend-cache"
fake_bun_log="$tmp_root/fake-bun.log"
fake_go_log="$tmp_root/fake-go.log"

cat >"$fake_bin/bun" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf '%s|%s\n' "$PWD" "$*" >>"$FAKE_BUN_LOG"

case "${1:-}:${2:-}" in
  --version:)
    printf '1.3.14-test\n'
    ;;
  install:--frozen-lockfile)
    exit 0
    ;;
  run:build)
    if [ "${FAIL_BUN_BUILD:-0}" = "1" ]; then
      exit 42
    fi
    case "$PWD" in
      */web/default)
        marker="default"
        ;;
      */web/classic)
        marker="classic"
        ;;
      *)
        printf 'unexpected build directory: %s\n' "$PWD" >&2
        exit 1
        ;;
    esac
    mkdir -p dist/assets
    printf '<!doctype html><html><body>fresh %s source build for embedded release validation; this fixture deliberately exceeds the release artifact minimum index size check.</body></html>\n' "$marker" >dist/index.html
    printf 'console.log("fresh %s source build")\n' "$marker" >"dist/assets/$marker.js"
    ;;
  *)
    printf 'unexpected bun command: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$fake_bin/bun"

cat >"$fake_bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf '%s|%s\n' "$PWD" "$*" >>"$FAKE_GO_LOG"

output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

[ -n "$output" ] || exit 1
mkdir -p "$(dirname "$output")"
printf '#!/usr/bin/env bash\nexit 0\n' >"$output"
chmod +x "$output"
EOF
chmod +x "$fake_bin/go"

FAKE_BUN_LOG="$fake_bun_log" \
FAKE_GO_LOG="$fake_go_log" \
BUN_BIN="$fake_bin/bun" \
GO_BIN="$fake_bin/go" \
FRONTEND_CACHE_ROOT="$frontend_cache_root" \
"$script_dir/build_release_candidate.sh" "$build_release_id" HEAD >/dev/null

[ ! -e "$build_release_src" ] || fail "release source worktree was not removed after build"
grep -Fxq "BUN_BIN=$fake_bin/bun" "$build_release_root/manifest.env" || fail "release manifest did not record the bun binary"
expected_web_lock_sha="$(git -C "$repo_root" show HEAD:web/bun.lock | sha256sum | awk '{print $1}')"
grep -Fxq "WEB_LOCK_SHA256=$expected_web_lock_sha" "$build_release_root/manifest.env" || fail "release manifest did not record the frontend lockfile"
frontend_cache_key="$(sed -n 's/^FRONTEND_CACHE_KEY=//p' "$build_release_root/manifest.env")"
[ -n "$frontend_cache_key" ] || fail "release manifest did not record the frontend cache key"
grep -Fxq "FRONTEND_CACHE_HIT=0" "$build_release_root/manifest.env" || fail "first release build unexpectedly hit the frontend cache"
grep -Fq "install --frozen-lockfile" "$fake_bun_log" || fail "release build did not install locked frontend dependencies"
[ "$(grep -Fc "run build" "$fake_bun_log")" -eq 2 ] || fail "release build did not build both frontend themes"
grep -Fq "build " "$fake_go_log" || fail "release build did not invoke Go"

: >"$fake_bun_log"
: >"$fake_go_log"
FAKE_BUN_LOG="$fake_bun_log" \
FAKE_GO_LOG="$fake_go_log" \
BUN_BIN="$fake_bin/bun" \
GO_BIN="$fake_bin/go" \
FRONTEND_CACHE_ROOT="$frontend_cache_root" \
"$script_dir/build_release_candidate.sh" "$cached_build_release_id" HEAD >/dev/null

cached_frontend_cache_key="$(sed -n 's/^FRONTEND_CACHE_KEY=//p' "$cached_build_release_root/manifest.env")"
[ "$cached_frontend_cache_key" = "$frontend_cache_key" ] || fail "same frontend source did not reuse the cache key"
grep -Fxq "FRONTEND_CACHE_HIT=1" "$cached_build_release_root/manifest.env" || fail "second release build did not hit the frontend cache"
if grep -Fq "install --frozen-lockfile" "$fake_bun_log" || grep -Fq "run build" "$fake_bun_log"; then
  fail "frontend cache hit still ran Bun install or theme builds"
fi
grep -Fq "build " "$fake_go_log" || fail "frontend cache hit skipped the Go build"

failed_build_release_id="test-helper-build-failure-$$"
failed_build_release_root="$repo_root/releases/$failed_build_release_id"
set +e
FAKE_BUN_LOG="$fake_bun_log" \
FAKE_GO_LOG="$fake_go_log" \
FAIL_BUN_BUILD="1" \
BUN_BIN="$fake_bin/bun" \
GO_BIN="$fake_bin/go" \
FRONTEND_CACHE_ROOT="$tmp_root/frontend-cache-failure" \
"$script_dir/build_release_candidate.sh" "$failed_build_release_id" HEAD >/dev/null 2>&1
failed_build_exit="$?"
set -e
[ "$failed_build_exit" -ne 0 ] || fail "failed frontend build unexpectedly succeeded"
[ ! -e "$failed_build_release_root/src" ] || fail "failed build left its source worktree behind"
if git -C "$repo_root" worktree list --porcelain | grep -Fqx "worktree $failed_build_release_root/src"; then
  fail "failed build left registered worktree metadata behind"
fi
rm -rf "$failed_build_release_root"

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
stage_pid="$(cat "$stage_pid_path")"
kill "$stage_pid" 2>/dev/null || true
wait "$stage_pid" 2>/dev/null || true
rm -f "$stage_pid_path"

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

cutover_release_id="test-helper-cutover-$$"
cutover_release_root="$repo_root/releases/$cutover_release_id"
cutover_app_root="$tmp_root/cutover-app"
cutover_restart_log="$tmp_root/cutover-restarts.log"
cutover_curl_state="$tmp_root/cutover-curl-state"
mkdir -p "$cutover_release_root/bin" "$cutover_release_root/runtime" "$cutover_app_root/data"
printf '#!/usr/bin/env bash\nexit 0\n' >"$cutover_release_root/bin/new-api"
chmod +x "$cutover_release_root/bin/new-api"
printf 'RELEASE_ID=%s\n' "$cutover_release_id" >"$cutover_release_root/manifest.env"
printf '#!/usr/bin/env bash\nexit 0\n' >"$cutover_app_root/new-api"
chmod +x "$cutover_app_root/new-api"
printf 'database\n' >"$cutover_app_root/data/new-api.db"

cat >"$fake_bin/systemctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$CUTOVER_RESTART_LOG"
EOF
chmod +x "$fake_bin/systemctl"

cat >"$fake_bin/sqlite3" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

db=""
for arg in "$@"; do
  case "$arg" in
    *.db)
      db="$arg"
      ;;
  esac
done
query="${!#}"
case "$query" in
  ".schema")
    printf 'create table test(id integer);\n'
    ;;
  ".backup "*)
    target="${query#".backup "}"
    target="${target#\'}"
    target="${target%\'}"
    cp "$db" "$target"
    ;;
  *"select key from tokens"*)
    printf 'smoke-token\n'
    ;;
esac
EOF
chmod +x "$fake_bin/sqlite3"

cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

state_file="${CUTOVER_CURL_STATE:?}"
last="${!#}"
case "$last" in
  */api/status)
    count=0
    if [ -f "$state_file" ]; then
      count="$(cat "$state_file")"
    fi
    count=$((count + 1))
    printf '%s\n' "$count" >"$state_file"
    if [ "$count" -lt 3 ]; then
      exit 7
    fi
    printf '{}'
    ;;
  */v1/models)
    printf '{"data":[{"id":"gpt-smoke"}]}'
    ;;
  *)
    printf '{}'
    ;;
esac
EOF
chmod +x "$fake_bin/curl"

cat >"$fake_bin/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$fake_bin/sleep"

PATH="$fake_bin:$PATH" \
APP_ROOT="$cutover_app_root" \
PROD_BASE_URL="http://cutover.test" \
SYSTEMCTL_BIN="$fake_bin/systemctl" \
CUTOVER_RESTART_LOG="$cutover_restart_log" \
CUTOVER_CURL_STATE="$cutover_curl_state" \
"$script_dir/cutover_release.sh" "$cutover_release_id" >/dev/null

cmp -s "$cutover_release_root/bin/new-api" "$cutover_app_root/new-api" || fail "cutover did not install the candidate binary"
[ "$(wc -l <"$cutover_restart_log")" -eq 1 ] || fail "cutover rolled back instead of waiting for service readiness"
[ "$(cat "$cutover_curl_state")" -ge 3 ] || fail "cutover did not retry the readiness endpoint"

finalize_release_id="test-helper-finalize-$$"
finalize_release_root="$repo_root/releases/$finalize_release_id"
finalize_app_root="$tmp_root/finalize-app"
mkdir -p "$finalize_release_root/bin" "$finalize_release_root/runtime/logs" "$finalize_release_root/src" "$finalize_app_root"
printf '#!/usr/bin/env bash\nexit 0\n' >"$finalize_release_root/bin/new-api"
chmod +x "$finalize_release_root/bin/new-api"
cp "$finalize_release_root/bin/new-api" "$finalize_app_root/new-api"
finalize_binary_sha="$(sha256sum "$finalize_release_root/bin/new-api" | awk '{print $1}')"
cat >"$finalize_release_root/manifest.env" <<EOF
RELEASE_ID=$finalize_release_id
BINARY_SHA256=$finalize_binary_sha
EOF
touch \
  "$finalize_release_root/runtime/candidate.env" \
  "$finalize_release_root/runtime/candidate.log" \
  "$finalize_release_root/runtime/live.env" \
  "$finalize_release_root/runtime/new-api.db" \
  "$finalize_release_root/runtime/schema-before.sha256" \
  "$finalize_release_root/runtime/schema-after.sha256" \
  "$finalize_release_root/runtime/schema-changed.flag" \
  "$finalize_release_root/runtime/logs/oneapi.log" \
  "$finalize_release_root/runtime/live-new-api.test.bak" \
  "$finalize_release_root/runtime/live-new-api.db.test.bak"
cat >"$finalize_release_root/runtime/cutover-backup.env" <<EOF
RELEASE_ID=$finalize_release_id
BACKUP_BIN=$finalize_release_root/runtime/live-new-api.test.bak
BACKUP_DB=$finalize_release_root/runtime/live-new-api.db.test.bak
EOF

PATH="$fake_bin:$PATH" APP_ROOT="$finalize_app_root" "$script_dir/finalize_release.sh" "$finalize_release_id" >/dev/null

[ ! -e "$finalize_release_root/src" ] || fail "finalize did not remove release source"
[ ! -e "$finalize_release_root/runtime/new-api.db" ] || fail "finalize did not remove candidate database"
[ ! -e "$finalize_release_root/runtime/candidate.log" ] || fail "finalize did not remove candidate log"
[ ! -e "$finalize_release_root/runtime/logs" ] || fail "finalize did not remove candidate log directory"
[ -f "$finalize_release_root/bin/new-api" ] || fail "finalize removed the candidate binary"
[ -f "$finalize_release_root/manifest.env" ] || fail "finalize removed the release manifest"
[ -f "$finalize_release_root/finalized.env" ] || fail "finalize did not record completion"
[ -f "$finalize_release_root/runtime/cutover-backup.env" ] || fail "finalize removed rollback metadata"
[ -f "$finalize_release_root/runtime/live-new-api.test.bak" ] || fail "finalize removed rollback binary"
[ -f "$finalize_release_root/runtime/live-new-api.db.test.bak" ] || fail "finalize removed rollback database"

assert_fails_with "release id may only contain" "$script_dir/build_release_candidate.sh" "../bad" HEAD
assert_fails_with "release id may only contain" "$script_dir/stage_release_runtime.sh" "../bad"
assert_fails_with "release id may only contain" env RELEASE_ID="../bad" "$script_dir/smoke_release.sh" "http://fake.local"
assert_fails_with "release id may only contain" "$script_dir/cutover_release.sh" "../bad"
assert_fails_with "release id may only contain" "$script_dir/finalize_release.sh" "../bad"

printf 'release-helper-contracts:PASS\n'
