#!/bin/bash
set -u

ENV_FILE="/opt/new-api/watchdog.env"
STATE_DIR="/opt/new-api/.watchdog"
LAST_CHECK_FILE="$STATE_DIR/last_check"
FAIL_COUNT_FILE="$STATE_DIR/fail_count"
LOCK_FILE="$STATE_DIR/lock"
URL="${URL:-http://localhost:4002/v1/chat/completions}"
CHECK_INTERVAL_SECONDS="${CHECK_INTERVAL_SECONDS:-3600}"
FAIL_THRESHOLD="${FAIL_THRESHOLD:-2}"
REQUEST_TIMEOUT_SECONDS="${REQUEST_TIMEOUT_SECONDS:-70}"
MODEL="${MODEL:-deepseek-v4-flash}"
MAX_TOKENS="${MAX_TOKENS:-3}"

if [ -f "$ENV_FILE" ]; then
  . "$ENV_FILE"
fi

WATCHDOG_KEY="${NEW_API_WATCHDOG_KEY:-}"
if [ -z "$WATCHDOG_KEY" ]; then
  echo "$(date) | ERROR missing watchdog key"
  exit 1
fi

mkdir -p "$STATE_DIR"
chmod 700 "$STATE_DIR"
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  exit 0
fi

now=$(date +%s)
last_check=0
if [ -f "$LAST_CHECK_FILE" ]; then
  read -r last_check < "$LAST_CHECK_FILE" || last_check=0
fi
case "$last_check" in
  ""|*[!0-9]*) last_check=0 ;;
esac

if [ "$CHECK_INTERVAL_SECONDS" -gt 0 ] && [ $((now - last_check)) -lt "$CHECK_INTERVAL_SECONDS" ]; then
  exit 0
fi
printf "%s\n" "$now" > "$LAST_CHECK_FILE"

fail_count=0
if [ -f "$FAIL_COUNT_FILE" ]; then
  read -r fail_count < "$FAIL_COUNT_FILE" || fail_count=0
fi
case "$fail_count" in
  ""|*[!0-9]*) fail_count=0 ;;
esac

payload=$(printf '{"model":"%s","messages":[{"role":"user","content":"hi"}],"max_tokens":%s}' "$MODEL" "$MAX_TOKENS")
http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$REQUEST_TIMEOUT_SECONDS" \
  "$URL" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $WATCHDOG_KEY" \
  -d "$payload" 2>/dev/null || true)

case "$http_code" in
  2*)
    printf "0\n" > "$FAIL_COUNT_FILE"
    ;;
  000|500|502|503|504|"")
    fail_count=$((fail_count + 1))
    printf "%s\n" "$fail_count" > "$FAIL_COUNT_FILE"
    if [ "$fail_count" -ge "$FAIL_THRESHOLD" ]; then
      printf "0\n" > "$FAIL_COUNT_FILE"
      systemctl restart new-api
      sleep 5
    fi
    ;;
  *)
    printf "0\n" > "$FAIL_COUNT_FILE"
    ;;
esac
