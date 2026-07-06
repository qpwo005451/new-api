# Repository Policy

## Local Fork Root
- `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251`

## Git Remotes
- `origin`: `https://github.com/qpwo005451/new-api.git`
- `upstream`: `https://github.com/QuantumNous/new-api.git`

## Branch Policy
- `prod/251` is the only production deployment branch.
- Work happens in `topic/*` branches and is merged back into `prod/251`.

## Tracked Operational Material
- `ops/instance/251/`
- `ops/reports/`
- `patches/`
- `scripts/`

## Runtime-Only Material
- `/opt/new-api/.env`
- `/opt/new-api/watchdog.env`
- `/opt/new-api/data/new-api.db`
- `/opt/new-api/.watchdog/`

## Reporting Expectations
- `check` and `report` should mention fork branch and commit provenance when available.
