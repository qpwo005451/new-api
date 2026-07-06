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

assert_contains() {
  local file="$1"
  local pattern="$2"
  grep -Fq "$pattern" "$file" || fail "missing pattern in $file: $pattern"
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

assert_contains "$script_dir/stage_release_runtime.sh" "PORT=4003"
assert_contains "$script_dir/stage_release_runtime.sh" "SQLITE_PATH="
assert_contains "$script_dir/stage_release_runtime.sh" "candidate.pid"

assert_contains "$script_dir/smoke_release.sh" "/api/status"
assert_contains "$script_dir/smoke_release.sh" "/v1/models"
assert_contains "$script_dir/smoke_release.sh" "/v1/chat/completions"
assert_contains "$script_dir/smoke_release.sh" "/v1/responses"

assert_contains "$script_dir/cutover_release.sh" "cutover-backup.env"
assert_contains "$script_dir/cutover_release.sh" "restart new-api"
assert_contains "$script_dir/cutover_release.sh" "smoke_release.sh"

assert_contains "$script_dir/rollback_release.sh" "cutover-backup.env"
assert_contains "$script_dir/rollback_release.sh" "RESTORE_DB"
assert_contains "$script_dir/rollback_release.sh" "restart new-api"

printf 'release-helper-contracts:PASS\n'
