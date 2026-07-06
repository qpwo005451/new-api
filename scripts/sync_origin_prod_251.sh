#!/bin/bash
set -euo pipefail
cd /opt/new-api

current_branch="$(git branch --show-current)"
if [ "$current_branch" != "prod/251" ]; then
  echo "Refusing to sync: current branch is '$current_branch' (expected 'prod/251')." >&2
  exit 1
fi

git fetch origin --prune
git pull --ff-only origin prod/251
git rev-parse --short HEAD
