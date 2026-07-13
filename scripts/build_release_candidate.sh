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

  local default_index_size classic_index_size default_js_sample classic_js_sample
  default_index_size="$(wc -c < "$default_dist/index.html")"
  classic_index_size="$(wc -c < "$classic_dist/index.html")"
  [ "$default_index_size" -gt 128 ] || fail "default dist index.html looks like placeholder content ($default_index_size bytes)"
  [ "$classic_index_size" -gt 128 ] || fail "classic dist index.html looks like placeholder content ($classic_index_size bytes)"

  default_js_sample="$(find "$default_dist" -type f \( -name '*.js' -o -name '*.mjs' \) -print -quit)"
  classic_js_sample="$(find "$classic_dist" -type f \( -name '*.js' -o -name '*.mjs' \) -print -quit)"
  [ -n "$default_js_sample" ] || fail "default dist missing JavaScript assets"
  [ -n "$classic_js_sample" ] || fail "classic dist missing JavaScript assets"
}

cleanup_release_tree() {
  ensure_release_path "$release_dir"
  ensure_release_path "$src_dir"
  ensure_release_path "$bin_dir"

  if git -C "$repo_root" worktree list --porcelain | grep -Fqx "worktree $src_dir"; then
    git -C "$repo_root" worktree remove --force "$src_dir"
    git -C "$repo_root" worktree prune
  fi
  rm -rf "$src_dir" "$bin_dir"
  rm -f "$manifest_path"
  mkdir -p "$release_dir" "$bin_dir"
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

cleanup_release_tree
# Contract marker: git worktree add --detach
git -C "$repo_root" worktree add --detach "$src_dir" "$release_commit" >/dev/null

build_embed_assets "$bun_bin" "$src_dir"
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
LOCAL_OPTION_OVERRIDES_SHA256=$local_option_overrides_sha256
LOCAL_OPTION_OVERRIDE_HELPER_SHA256=$local_option_override_helper_sha256
BUN_BIN=$bun_bin
BUILT_AT=$(date -Iseconds)
EOF

printf 'Release candidate ready: %s\n' "$release_dir"
printf 'Candidate source: %s\n' "$src_dir"
printf 'Binary: %s\n' "$bin_dir/new-api"
printf 'Manifest: %s\n' "$manifest_path"
