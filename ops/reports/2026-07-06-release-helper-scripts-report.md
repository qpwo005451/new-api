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

## Intent

These scripts remove the `prepare` blocker recorded in `ops/reports/2026-07-06-fork-baseline-wiring-report.md` from the fork source-of-truth side.

They do not by themselves change the live production checkout, rebuild the binary, restart `new-api.service`, or run a candidate release.

## Verification

- `scripts/test_release_helpers.sh`: passed
- `bash -n` across all release helper scripts and the helper contract test: passed
- `git diff --check -- scripts .gitignore ops/reports/2026-07-06-release-helper-scripts-report.md`: passed

## Production Status

Production still needs to fetch or otherwise consume this fork commit before `/opt/new-api/scripts/` contains the new helper scripts.

Next safe action remains `prepare` only after:

- production has the helper scripts from `prod/251`,
- an explicit target version or tag is provided.
