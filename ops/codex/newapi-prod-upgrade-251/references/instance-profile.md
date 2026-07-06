# Instance Profile

## Fixed Target
- Host: `10.0.0.251`
- SSH user: `root`
- App root: `/opt/new-api`
- Service: `new-api.service`
- Production port: `4002`
- Candidate port: `4003`
- Production database: `/opt/new-api/data/new-api.db`

## Approved Scope
This skill only operates on `10.0.0.251:/opt/new-api`.
Refuse requests that target any other host, root, port, or deployment.

## Core Runtime Files
- `/opt/new-api/new-api`
- `/opt/new-api/.env`
- `/opt/new-api/post-rebuild-patches.sh`

## Required Patch And Override Assets
- `/opt/new-api/patches/patch-image-gen-filter.py`
- `/opt/new-api/patches/local-option-overrides.json`
- `/opt/new-api/patches/apply-local-option-overrides.py`

## Required Release Scripts
- `/opt/new-api/scripts/build_release_candidate.sh`
- `/opt/new-api/scripts/stage_release_runtime.sh`
- `/opt/new-api/scripts/smoke_release.sh`
- `/opt/new-api/scripts/cutover_release.sh`
- `/opt/new-api/scripts/rollback_release.sh`

## Candidate Runtime Expectations
- Candidate releases live under `/opt/new-api/releases/<release-id>/`
- Candidate binary lives under `/opt/new-api/releases/<release-id>/bin/new-api`
- Candidate runtime must use the copied database under `/opt/new-api/releases/<release-id>/runtime/new-api.db`
- Candidate verification must prove that the expected PID owns port `4003`
