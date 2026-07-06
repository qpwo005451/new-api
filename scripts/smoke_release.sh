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

model_present() {
  local model="$1"
  python3 - "$model" <<'PY'
import json
import sys

target = sys.argv[1]
data = json.load(sys.stdin)
ids = [item.get("id") for item in data.get("data", []) if isinstance(item, dict)]
sys.exit(0 if target in ids else 1)
PY
}

pick_model() {
  if [ -n "$requested_model" ]; then
    printf '%s' "$models_json" | model_present "$requested_model" || fail "requested smoke model missing from /v1/models: $requested_model"
    printf '%s\n' "$requested_model"
    return 0
  fi

  printf '%s' "$models_json" | python3 - <<'PY'
import json
import sys

data = json.load(sys.stdin)
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
  db_path="$repo_root/releases/$release_id/runtime/new-api.db"
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

"${curl_fast[@]}" "$base_url/" >/dev/null
"${curl_fast[@]}" "$base_url/api/status" >/dev/null

token="$(sqlite3 -noheader -batch "$db_path" "select key from tokens where status = 1 order by id limit 1;" || true)"
if [ -z "$token" ]; then
  token="$(sqlite3 -noheader -batch "$db_path" "select key from tokens order by id limit 1;" || true)"
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
