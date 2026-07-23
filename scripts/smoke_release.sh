#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
base_url="${1:-${BASE_URL:-}}"
db_path="${2:-${DB_PATH:-}}"
mode="${3:-${SMOKE_MODE:-full}}"
release_id="${RELEASE_ID:-}"
requested_model="${SMOKE_MODEL:-}"

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<'EOF' >&2
usage: smoke_release.sh <base-url> [db-path] [full|fast]

Examples:
  smoke_release.sh http://127.0.0.1:4003
  smoke_release.sh http://127.0.0.1:4002 /opt/new-api/data/new-api.db fast
EOF
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

model_present() {
  local model="$1"
  MODELS_JSON="$models_json" python3 - "$model" <<'PY'
import json
import os
import sys

target = sys.argv[1]
data = json.loads(os.environ["MODELS_JSON"])
ids = [item.get("id") for item in data.get("data", []) if isinstance(item, dict)]
sys.exit(0 if target in ids else 1)
PY
}

pick_model() {
  if [ -n "$requested_model" ]; then
    model_present "$requested_model" || fail "requested smoke model missing from /v1/models: $requested_model"
    printf '%s\n' "$requested_model"
    return 0
  fi

  MODELS_JSON="$models_json" python3 - <<'PY'
import json
import os

data = json.loads(os.environ["MODELS_JSON"])
skip = ("embedding", "reranker", "bge", "tts", "voice", "image", "whisper", "moderation")
for item in data.get("data", []):
    model_id = item.get("id") or ""
    lower = model_id.lower()
    if model_id and not any(term in lower for term in skip):
        print(model_id)
        break
PY
}

[ -n "$base_url" ] || usage
if [ -z "$db_path" ] && [ -n "$release_id" ]; then
  validate_release_id
  [ ! -L "$repo_root/releases" ] || fail "refusing symlinked releases root: $repo_root/releases"
  [ ! -L "$repo_root/releases/$release_id" ] || fail "refusing symlinked release directory: $repo_root/releases/$release_id"
  [ ! -L "$repo_root/releases/$release_id/runtime" ] || fail "refusing symlinked runtime directory: $repo_root/releases/$release_id/runtime"
  db_path="$repo_root/releases/$release_id/runtime/new-api.db"
  ensure_release_path "$db_path"
fi
[ -f "$db_path" ] || fail "missing database: $db_path"
case "$mode" in
  fast|full) ;;
  *)
    fail "mode must be fast or full"
    ;;
esac

curl_fast=(curl --connect-timeout 3 --max-time 8 -fsS)
curl_slow=(curl --connect-timeout 3 --max-time 30 -fsS)

read_smoke_token() {
  sqlite3 -cmd ".timeout 5000" -noheader -batch "$db_path" "$1"
}

"${curl_fast[@]}" "$base_url/" >/dev/null
"${curl_fast[@]}" "$base_url/api/status" >/dev/null

token="$(read_smoke_token "select key from tokens where status = 1 order by id limit 1;" || true)"
if [ -z "$token" ]; then
  token="$(read_smoke_token "select key from tokens order by id limit 1;" || true)"
fi
[ -n "$token" ] || fail "missing token in smoke database"

models_json="$("${curl_fast[@]}" -H "Authorization: Bearer $token" "$base_url/v1/models")"
if [ "$mode" = "fast" ]; then
  printf 'smoke fast ok: %s\n' "$base_url"
  exit 0
fi

selected_model="$(pick_model)"
[ -n "$selected_model" ] || fail "missing usable model id from /v1/models"

chat_payload="$(printf '{"model":"%s","messages":[{"role":"user","content":"ping"}],"max_tokens":1}' "$selected_model")"
responses_payload="$(printf '{"model":"%s","input":"ping","max_output_tokens":8}' "$selected_model")"

"${curl_slow[@]}" -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
  -d "$chat_payload" "$base_url/v1/chat/completions" >/dev/null
"${curl_slow[@]}" -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
  -d "$responses_payload" "$base_url/v1/responses" >/dev/null

printf 'smoke full ok: %s model=%s\n' "$base_url" "$selected_model"
