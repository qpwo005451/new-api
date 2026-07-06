#!/bin/bash
set -euo pipefail
cd /opt/new-api

git fetch origin --prune
git checkout prod/251
git pull --ff-only origin prod/251
git rev-parse --short HEAD
