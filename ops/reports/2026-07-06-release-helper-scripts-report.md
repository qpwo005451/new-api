# 2026-07-06 Release Helper Scripts

Scope: add repo-tracked release helper scripts for the fork-backed NewAPI production workflow at `10.0.0.251:/opt/new-api`.

## Changed In Fork

- Added `scripts/build_release_candidate.sh`
- Added `scripts/stage_release_runtime.sh`
- Added `scripts/smoke_release.sh`
- Added `scripts/cutover_release.sh`
- Added `scripts/rollback_release.sh`
- Added `scripts/test_release_helpers.sh`
- Added `.gitignore` coverage for `releases/` runtime artifacts
- Hardened release-id validation for build, stage, and cutover helpers so release artifacts stay under `releases/<release-id>/`
- Fixed smoke model selection so Python reads the `/v1/models` JSON from `MODELS_JSON` instead of losing stdin to heredocs
- Hardened candidate staging so the candidate runtime forces `SQL_DSN=local`, removes `LOG_SQL_DSN`, and uses the copied runtime SQLite database
- Hardened candidate launch so inherited `PORT`, `SQL_DSN`, `LOG_SQL_DSN`, and `SQLITE_PATH` are unset before loading `candidate.env`
- Hardened stale candidate shutdown so a PID is killed only when it owns port `4003` and matches the candidate binary
- Hardened release path handling with realpath checks and symlinked release/runtime directory refusal
- Hardened stage and cutover helpers to reject symlinked candidate manifests/binaries and symlinked live cutover targets
- Hardened smoke default `RELEASE_ID` handling so implicit candidate DB lookup also rejects unsafe release ids and symlinked release paths

## Intent

These scripts remove the `prepare` blocker recorded in `ops/reports/2026-07-06-fork-baseline-wiring-report.md` from the fork source-of-truth side.

They do not by themselves change the live production checkout, rebuild the binary, restart `new-api.service`, or run a candidate release.

## Verification

- `scripts/test_release_helpers.sh`: passed
- `bash -n` across all release helper scripts and the helper contract test: passed
- `git diff --check -- scripts .gitignore ops/reports/2026-07-06-release-helper-scripts-report.md`: passed
- Invalid release-id negative checks for build, stage, smoke, and cutover helpers: passed
- Fake smoke test for `/v1/models`, `/v1/chat/completions`, and `/v1/responses`: passed
- Stage dry-run with live-looking `SQL_DSN` and `LOG_SQL_DSN`: passed; generated `candidate.env` forced `SQL_DSN=local` and runtime `SQLITE_PATH`
- Stage dry-run with inherited external DB env: passed; candidate process inherited `SQL_DSN=local` and no `LOG_SQL_DSN`
- Runtime symlink refusal check: covered when the local filesystem supports `test -L` symlinks

## Production Status

Production still needs to fetch or otherwise consume this fork commit before `/opt/new-api/scripts/` contains the new helper scripts.

Next safe action remains `prepare` only after:

- production has the helper scripts from `prod/251`,
- an explicit target version or tag is provided.
