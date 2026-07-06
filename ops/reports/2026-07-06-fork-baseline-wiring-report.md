# 2026-07-06 Fork Baseline Wiring

Scope: local repo `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251` and production NewAPI instance at `10.0.0.251:/opt/new-api`

## Local Fork Baseline

- Working branch: `prod/251`
- Initial baseline commit pushed to fork: `db53196955184b99831a6ae80495cc34dd68e31b` (`docs: tighten prod 251 skill verify semantics`)
- Local tracking updated to `prod/251 -> origin/prod/251`
- `origin`: `https://github.com/qpwo005451/new-api.git`
- `upstream`: `https://github.com/QuantumNous/new-api.git`
- Red-light baseline confirmed before push: `git ls-remote --heads https://github.com/qpwo005451/new-api.git prod/251` returned no matching head
- Final post-push verification confirmed that `origin/prod/251` exists on the fork and points at the report-bearing branch head.

## Production Remote Wiring

- Target repo: `10.0.0.251:/opt/new-api`
- Production git remotes rewired to:
  - `origin => https://github.com/qpwo005451/new-api.git`
  - `upstream => https://github.com/QuantumNous/new-api.git`
- Safety boundary for Task 6:
  - only git remote wiring was changed on production;
  - no branch checkout was performed;
  - no production source file, `.env`, database, or script content was modified;
  - no rebuild, restart, or `new-api.service` bounce was performed.

## Production Verification

- Pre-change live branch: `main`
- Pre-change live commit: `0936e2504655a5cbf7bc3c388f6d3e2bb24916d3`
- Post-change `git remote -v`:
  - `origin https://github.com/qpwo005451/new-api.git (fetch)`
  - `origin https://github.com/qpwo005451/new-api.git (push)`
  - `upstream https://github.com/QuantumNous/new-api.git (fetch)`
  - `upstream https://github.com/QuantumNous/new-api.git (push)`
- Post-change `git branch --show-current`: `main`
- Post-change `git rev-parse HEAD`: `0936e2504655a5cbf7bc3c388f6d3e2bb24916d3`
- Post-change `git rev-parse origin/prod/251` resolved successfully after the explicit remote-tracking-ref fetch used for verification.
- Post-change `systemctl is-active new-api.service`: `active`
- The production live branch and live commit remained unchanged across the remote rewiring.

## Notes

- To materialize `origin/prod/251` on production for verification, the repo fetched only that remote-tracking ref with an explicit branch spec and without any checkout; the ref was refreshed again after the report commit was pushed so production-side verification reflects the final branch head.
- Expected release helper scripts under `/opt/new-api/scripts/` were absent during this task:
  - `build_release_candidate.sh`
  - `stage_release_runtime.sh`
  - `smoke_release.sh`
  - `cutover_release.sh`
  - `rollback_release.sh`
- Those missing scripts did not block Task 6 because this task did not perform `prepare`, `verify`, `cutover`, or `rollback`.
- Next safe action: `prepare`
