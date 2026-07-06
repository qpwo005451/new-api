#!/bin/bash
set -euo pipefail
cd /opt/new-api

echo "=== Branch ==="
git branch --show-current

echo "=== Apply local option overrides ==="
python3 patches/apply-local-option-overrides.py patches/local-option-overrides.json

echo "=== Build ==="
/usr/local/go/bin/go build -o /opt/new-api/new-api . 2>&1

echo "=== Restart ==="
systemctl restart new-api
systemctl is-active --quiet new-api

echo "new-api restarted from repo-tracked source."
echo "NOTE: source-level customizations must already be present in git."
