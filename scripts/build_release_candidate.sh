#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
release_id="${1:-${RELEASE_ID:-}}"
release_tag="${2:-${RELEASE_TAG:-}}"
option_manifest="$repo_root/patches/local-option-overrides.json"
option_helper="$repo_root/patches/apply-local-option-overrides.py"

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<'EOF' >&2
usage: build_release_candidate.sh <release-id> <release-tag-or-commit>

Required:
  <release-id>            explicit release identity (for example 2026-07-06-rc01)
  <release-tag-or-commit> explicit git tag, branch, or commit to build
EOF
  exit 1
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

validate_release_id() {
  case "$release_id" in
    "."|".."|*[!A-Za-z0-9._-]*)
      fail "release id may only contain letters, digits, dot, underscore, and dash"
      ;;
  esac
}

pick_go_bin() {
  if [ -n "${GO_BIN:-}" ]; then
    printf '%s\n' "$GO_BIN"
    return 0
  fi
  if [ -x /usr/local/go/bin/go ]; then
    printf '%s\n' /usr/local/go/bin/go
    return 0
  fi
  command -v go >/dev/null 2>&1 || fail "go binary not found; set GO_BIN"
  command -v go
}

pick_bun_bin() {
  if [ -n "${BUN_BIN:-}" ]; then
    [ -x "$BUN_BIN" ] || fail "bun binary is not executable: $BUN_BIN"
    printf '%s\n' "$BUN_BIN"
    return 0
  fi
  command -v bun >/dev/null 2>&1 || fail "bun binary not found; set BUN_BIN"
  command -v bun
}

validate_embed_assets() {
  local default_dist="$1"
  local classic_dist="$2"
  local default_index_size classic_index_size default_js_sample classic_js_sample
  [ -f "$default_dist/index.html" ] || return 1
  [ -f "$classic_dist/index.html" ] || return 1
  default_index_size="$(wc -c < "$default_dist/index.html")"
  classic_index_size="$(wc -c < "$classic_dist/index.html")"
  [ "$default_index_size" -gt 128 ] || return 1
  [ "$classic_index_size" -gt 128 ] || return 1

  default_js_sample="$(find "$default_dist" -type f \( -name '*.js' -o -name '*.mjs' \) -print -quit)"
  classic_js_sample="$(find "$classic_dist" -type f \( -name '*.js' -o -name '*.mjs' \) -print -quit)"
  [ -n "$default_js_sample" ] || return 1
  [ -n "$classic_js_sample" ] || return 1
}

build_embed_assets() {
  local bun_bin="$1"
  local target_root="$2"
  local frontend_root="$target_root/web"
  local default_dist="$frontend_root/default/dist"
  local classic_dist="$frontend_root/classic/dist"

  [ -f "$frontend_root/package.json" ] || fail "missing frontend workspace package.json: $frontend_root/package.json"
  [ -f "$frontend_root/bun.lock" ] || fail "missing frontend lockfile: $frontend_root/bun.lock"
  [ -f "$frontend_root/default/package.json" ] || fail "missing default frontend package.json"
  [ -f "$frontend_root/classic/package.json" ] || fail "missing classic frontend package.json"

  (
    cd "$frontend_root"
    "$bun_bin" install --frozen-lockfile
    (
      cd default
      "$bun_bin" run build
    )
    (
      cd classic
      "$bun_bin" run build
    )
  )

  validate_embed_assets "$default_dist" "$classic_dist" ||
    fail "frontend builds did not produce valid default and classic assets"
}

restore_embed_assets_from_cache() {
  local cache_entry="$1"
  local target_root="$2"
  local frontend_root="$target_root/web"
  local default_dist="$frontend_root/default/dist"
  local classic_dist="$frontend_root/classic/dist"
  local cached_default_dist="$cache_entry/default"
  local cached_classic_dist="$cache_entry/classic"

  [ -d "$cached_default_dist" ] && [ -d "$cached_classic_dist" ] || return 1
  rm -rf "$default_dist" "$classic_dist"
  cp -a "$cached_default_dist" "$default_dist"
  cp -a "$cached_classic_dist" "$classic_dist"
  validate_embed_assets "$default_dist" "$classic_dist"
}

store_embed_assets_in_cache() {
  local cache_root="$1"
  local cache_key="$2"
  local target_root="$3"
  local frontend_root="$target_root/web"
  local cache_entry="$cache_root/$cache_key"
  local cache_tmp

  [ -e "$cache_entry" ] && return 0
  mkdir -p "$cache_root"
  cache_tmp="$(mktemp -d "$cache_root/.${cache_key}.tmp.XXXXXX")"
  cp -a "$frontend_root/default/dist" "$cache_tmp/default"
  cp -a "$frontend_root/classic/dist" "$cache_tmp/classic"
  validate_embed_assets "$cache_tmp/default" "$cache_tmp/classic" || {
    rm -rf "$cache_tmp"
    fail "refusing to cache invalid frontend build artifacts"
  }

  if [ -e "$cache_entry" ]; then
    rm -rf "$cache_tmp"
    return 0
  fi
  if ! mv -T "$cache_tmp" "$cache_entry"; then
    rm -rf "$cache_tmp"
  fi
}

worktree_registered() {
  local source_real source_windows worktrees
  source_real="$(realpath -m "$src_dir")"
  source_windows="$(cygpath -m "$source_real" 2>/dev/null || true)"
  worktrees="$(git -C "$repo_root" worktree list --porcelain)" || return 1
  printf '%s\n' "$worktrees" | grep -Fqx "worktree $source_real" ||
    { [ -n "$source_windows" ] && printf '%s\n' "$worktrees" | grep -Fqx "worktree $source_windows"; }
}

run_worktree_cleanup_command() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 8s "$@"
    return
  fi
  "$@"
}

cleanup_release_tree() {
  ensure_release_path "$release_dir"
  ensure_release_path "$src_dir"
  ensure_release_path "$bin_dir"

  if worktree_registered || [ -e "$src_dir" ]; then
    cleanup_source_worktree || fail "failed to remove previous release source worktree: $src_dir"
  fi
  rm -rf "$src_dir" "$bin_dir"
  rm -f "$manifest_path"
  mkdir -p "$release_dir" "$bin_dir"
}

source_worktree_active="0"
cleanup_source_worktree() {
  local attempt
  ensure_release_path "$src_dir"

  for attempt in 1 2 3; do
    if worktree_registered; then
      run_worktree_cleanup_command git -C "$repo_root" worktree remove --force "$src_dir" >/dev/null 2>&1 || true
    fi
    if [ -e "$src_dir" ]; then
      run_worktree_cleanup_command rm -rf "$src_dir" >/dev/null 2>&1 || true
    fi
    run_worktree_cleanup_command git -C "$repo_root" worktree prune >/dev/null 2>&1 || true
    if ! [ -e "$src_dir" ] && ! worktree_registered; then
      source_worktree_active="0"
      return 0
    fi
    [ "$attempt" -eq 3 ] || sleep 1
  done

  printf 'WARN: failed to remove source worktree after %s attempts: %s\n' "$attempt" "$src_dir" >&2
  return 1
}

cleanup_source_worktree_on_exit() {
  local exit_code="$?"

  if [ "$source_worktree_active" = "1" ]; then
    cleanup_source_worktree || true
  fi
  exit "$exit_code"
}

[ -n "$release_id" ] || usage
[ -n "$release_tag" ] || usage
validate_release_id

release_dir="$repo_root/releases/$release_id"
src_dir="$release_dir/src"
bin_dir="$release_dir/bin"
manifest_path="$release_dir/manifest.env"

[ ! -L "$repo_root/releases" ] || fail "refusing symlinked releases root: $repo_root/releases"
[ ! -L "$release_dir" ] || fail "refusing symlinked release directory: $release_dir"

git -C "$repo_root" rev-parse --show-toplevel >/dev/null 2>&1 || fail "repo root is not a git worktree: $repo_root"
[ -f "$option_manifest" ] || fail "missing local option override manifest: $option_manifest"
[ -f "$option_helper" ] || fail "missing local option override helper: $option_helper"

go_bin="$(pick_go_bin)"
bun_bin="$(pick_bun_bin)"
release_commit="$(git -C "$repo_root" rev-parse --verify "${release_tag}^{commit}")"
frontend_cache_root="$(realpath -m "${FRONTEND_CACHE_ROOT:-"$repo_root/.local-tools/release-cache/frontend"}")"
frontend_bun_version="$("$bun_bin" --version | tr -d '\r\n')"
[ -n "$frontend_bun_version" ] || fail "failed to determine frontend Bun version"
frontend_web_tree="$(git -C "$repo_root" rev-parse "${release_commit}:web")"
frontend_cache_key="$(printf 'web-tree=%s\nbun-version=%s\n' "$frontend_web_tree" "$frontend_bun_version" | sha256sum | awk '{print $1}')"
frontend_cache_entry="$frontend_cache_root/$frontend_cache_key"
frontend_cache_hit="0"

cleanup_release_tree
# Contract marker: git worktree add --detach
git -C "$repo_root" worktree add --detach "$src_dir" "$release_commit" >/dev/null
source_worktree_active="1"
trap cleanup_source_worktree_on_exit EXIT

if restore_embed_assets_from_cache "$frontend_cache_entry" "$src_dir"; then
  frontend_cache_hit="1"
else
  rm -rf "$frontend_cache_entry"
  rm -rf "$src_dir/web/default/dist" "$src_dir/web/classic/dist"
  build_embed_assets "$bun_bin" "$src_dir"
  store_embed_assets_in_cache "$frontend_cache_root" "$frontend_cache_key" "$src_dir"
fi
source_tree_commit="$(git -C "$src_dir" rev-parse HEAD)"

(
  cd "$src_dir"
  "$go_bin" build \
    -ldflags "-s -w -X github.com/QuantumNous/new-api/common.Version=$release_tag" \
    -o "$bin_dir/new-api" \
    .
)

binary_sha256="$(sha256sum "$bin_dir/new-api" | awk '{print $1}')"
local_option_overrides_sha256="$(sha256sum "$option_manifest" | awk '{print $1}')"
local_option_override_helper_sha256="$(sha256sum "$option_helper" | awk '{print $1}')"

cat >"$manifest_path" <<EOF
RELEASE_ID=$release_id
RELEASE_TAG=$release_tag
RELEASE_COMMIT=$release_commit
SOURCE_TREE_COMMIT=$source_tree_commit
BINARY_SHA256=$binary_sha256
WEB_LOCK_SHA256=$(sha256sum "$src_dir/web/bun.lock" | awk '{print $1}')
FRONTEND_CACHE_KEY=$frontend_cache_key
FRONTEND_CACHE_HIT=$frontend_cache_hit
FRONTEND_WEB_TREE=$frontend_web_tree
BUN_VERSION=$frontend_bun_version
LOCAL_OPTION_OVERRIDES_SHA256=$local_option_overrides_sha256
LOCAL_OPTION_OVERRIDE_HELPER_SHA256=$local_option_override_helper_sha256
BUN_BIN=$bun_bin
BUILT_AT=$(date -Iseconds)
EOF

cleanup_source_worktree
trap - EXIT

printf 'Release candidate ready: %s\n' "$release_dir"
printf 'Binary: %s\n' "$bin_dir/new-api"
printf 'Manifest: %s\n' "$manifest_path"
