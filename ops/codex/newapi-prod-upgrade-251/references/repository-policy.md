# Repository Policy

## Local Fork Root
- `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251`

## Git Remotes
- `origin`: `https://github.com/qpwo005451/new-api.git`
- `upstream`: `https://github.com/QuantumNous/new-api.git`

## Branch Policy
- `prod/251` is the only production deployment branch.
- Work happens in `topic/*` branches and is merged back into `prod/251`.

## Interim Tracking Warning
- Until Task 6 publishes `origin/prod/251` and fixes upstream tracking, implicit `git pull` / `git push` on `prod/251` is not safe.
- For any networked git command on `prod/251`, explicitly specify the remote and branch arguments instead of relying on branch tracking defaults.
- Treat any branch tracking mismatch, including `prod/251` tracking `origin/main`, as a risk for `check` and `report`, and as a blocker for networked git actions until it is explicitly handled.

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
- `check` and `report` should call out branch tracking mismatch as a risk or blocker when repository context shows it.
