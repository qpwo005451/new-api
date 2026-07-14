# Artifact Locations

## Repository Tracked Locations
- Local fork root: `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251`
- Instance docs: `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251\ops\instance\251`
- Reports: `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251\ops\reports`
- Repo-tracked skill: `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251\ops\codex\newapi-prod-upgrade-251`

## Runtime Source Of Truth
- App root: `/opt/new-api`
- Production database: `/opt/new-api/data/new-api.db`

## Runtime Patch And Override Assets
- `/opt/new-api/post-rebuild-patches.sh`
- `/opt/new-api/patches/patch-image-gen-filter.py`
- `/opt/new-api/patches/local-option-overrides.json`
- `/opt/new-api/patches/apply-local-option-overrides.py`
- `/opt/new-api/scripts/channel_guard.py`
- `/opt/new-api/scripts/input_budget_guard.py`
- `/opt/new-api/scripts/sync_origin_prod_251.sh`
- `/opt/new-api/watchdog.sh`

## Release Identity Artifacts
- `/opt/new-api/releases/release-id/manifest.env`
- `/opt/new-api/releases/release-id/runtime/candidate.env`
- `/opt/new-api/releases/release-id/runtime/candidate.pid`
- `/opt/new-api/releases/release-id/runtime/candidate.log`
- `/opt/new-api/releases/release-id/runtime/cutover-backup.env`
- `/opt/new-api/releases/release-id/finalized.env`

The detached `releases/release-id/src` worktree and candidate database are temporary. The build helper removes the source worktree after compilation, and `finalize_release.sh` removes candidate runtime copies after the operator confirms stability.
