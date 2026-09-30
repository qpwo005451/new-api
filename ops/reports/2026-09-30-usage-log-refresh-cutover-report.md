# Usage Log Refresh Release

## Source And Build
- Fix: `01bee8c4bf6988d15db54170633e8563382bfb00`, merged into and pushed to `origin/prod/251`.
- Release: `2026-09-30-usage-log-refresh-rc01`.
- Local build: `scripts/build_release_candidate_local.ps1 -ReleaseId 2026-09-30-usage-log-refresh-rc01 -ReleaseTag 01bee8c4bf6988d15db54170633e8563382bfb00`, exit 0; Linux amd64 static ELF.
- Binary SHA-256: `68bf28ae1b732bffa59858980d3e2f169d24c12b55afb43cb72fcb8a1f8508ae`.
- Prior focused frontend verification: 317 tests, typecheck, targeted lint/format and production build passed.

## Candidate Acceptance
- Candidate used a SQLite backup in its release runtime directory, slave mode and no Redis.
- A systemd PrivateNetwork namespace had no external routes. Loopback proxy and SSH tunnel provided browser access only.
- Browser acceptance passed on desktop 1440x1000 and mobile 390x844. Real candidate log responses were delayed during automatic refresh: existing logs stayed visible without opacity or skeleton states; Search remained enabled. Screenshots were inspected.
- Authentication bootstrap was mocked using a temporary credential valid only in the copied database. No real upstream model calls or test consumption records were generated.

## Production Cutover
- Operator explicitly confirmed cutover and subsequent backup cleanup.
- `timeout 120s bash /opt/new-api/scripts/cutover_release.sh 2026-09-30-usage-log-refresh-rc01` exited 0; fast smoke passed on port 4002.
- Service active, PID 1885394; live binary SHA-256 matched the candidate. `/api/status` succeeded and reported the fix commit. SQLite `PRAGMA quick_check` returned `ok`.
- This was a stop/backup/replace/restart cutover, not a zero-downtime deployment.

## Cleanup
- Candidate service, loopback proxy and SSH tunnel stopped; port 4003 released.
- Finalization removed candidate database, copied environment and candidate logs.
- At operator request, this release's old binary, database backup and rollback metadata were deleted after health verification. That database snapshot is no longer available for rollback.
- Candidate binary, manifest and finalized record remain on the host. Older release artifacts were not changed.
- Local cleanup helper removed the candidate and temporary build/dependency trees while retaining reusable release caches. Untracked `.zcode/` was preserved.
