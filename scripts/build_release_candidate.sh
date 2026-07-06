#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
release_id="${1:-${RELEASE_ID:-}}"
release_tag="${2:-${RELEASE_TAG:-}}"
app_root="${APP_ROOT:-/opt/new-api}"
source_app_root="${SOURCE_APP_ROOT:-$app_root}"
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
  case "$1" in
    "$repo_root/releases/"*) ;;
    *)
      fail "refusing to operate outside $repo_root/releases: $1"
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

sync_embed_assets() {
  local source_root="$1"
  local target_root="$2"
  local source_default="$source_root/web/default/dist"
  local source_classic="$source_root/web/classic/dist"
  local target_default="$target_root/web/default/dist"
  local target_classic="$target_root/web/classic/dist"

  [ -d "$source_default" ] || fail "missing source default dist: $source_default"
  [ -d "$source_classic" ] || fail "missing source classic dist: $source_classic"
  [ -f "$source_default/index.html" ] || fail "missing source default dist index: $source_default/index.html"
  [ -f "$source_classic/index.html" ] || fail "missing source classic dist index: $source_classic/index.html"

  rm -rf "$target_default" "$target_classic"
  mkdir -p "$target_default" "$target_classic"
  cp -a "$source_default/." "$target_default/"
  cp -a "$source_classic/." "$target_classic/"

  local default_index_size classic_index_size default_js_sample classic_js_sample
  default_index_size="$(wc -c < "$target_default/index.html")"
  classic_index_size="$(wc -c < "$target_classic/index.html")"
  [ "$default_index_size" -gt 128 ] || fail "default dist index.html looks like placeholder content ($default_index_size bytes)"
  [ "$classic_index_size" -gt 128 ] || fail "classic dist index.html looks like placeholder content ($classic_index_size bytes)"

  default_js_sample="$(find "$target_default" -type f \( -name '*.js' -o -name '*.mjs' \) -print -quit)"
  classic_js_sample="$(find "$target_classic" -type f \( -name '*.js' -o -name '*.mjs' \) -print -quit)"
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

release_dir="$repo_root/releases/$release_id"
src_dir="$release_dir/src"
bin_dir="$release_dir/bin"
manifest_path="$release_dir/manifest.env"

git -C "$repo_root" rev-parse --show-toplevel >/dev/null 2>&1 || fail "repo root is not a git worktree: $repo_root"
[ -f "$option_manifest" ] || fail "missing local option override manifest: $option_manifest"
[ -f "$option_helper" ] || fail "missing local option override helper: $option_helper"

go_bin="$(pick_go_bin)"
release_commit="$(git -C "$repo_root" rev-parse --verify "${release_tag}^{commit}")"
source_tree_commit="$(git -C "$repo_root" rev-parse HEAD)"

cleanup_release_tree
# Contract marker: git worktree add --detach
git -C "$repo_root" worktree add --detach "$src_dir" "$release_commit" >/dev/null

sync_embed_assets "$source_app_root" "$src_dir"

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
LOCAL_OPTION_OVERRIDES_SHA256=$local_option_overrides_sha256
LOCAL_OPTION_OVERRIDE_HELPER_SHA256=$local_option_override_helper_sha256
SOURCE_APP_ROOT=$source_app_root
BUILT_AT=$(date -Iseconds)
EOF

printf 'Release candidate ready: %s\n' "$release_dir"
printf 'Candidate source: %s\n' "$src_dir"
printf 'Binary: %s\n' "$bin_dir/new-api"
printf 'Manifest: %s\n' "$manifest_path"
