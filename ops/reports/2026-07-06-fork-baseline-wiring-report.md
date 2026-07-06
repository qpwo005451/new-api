# 2026-07-06 Fork Baseline Wiring

Scope: local repo `C:\Users\Administrator\Documents\Codex\2026-06-07\gpt-5-4-10-0-0\newapi-fork-251` and production NewAPI instance at `10.0.0.251:/opt/new-api`

## Local Fork Baseline

- Working branch: `prod/251`
- Initial baseline commit pushed to fork: `db53196955184b99831a6ae80495cc34dd68e31b` (`docs: tighten prod 251 skill verify semantics`)
- Local tracking updated to `prod/251 -> origin/prod/251`
- `origin`: `https://github.com/qpwo005451/new-api.git`
- `upstream`: `https://github.com/QuantumNous/new-api.git`
- Red-light baseline confirmed before push: `git ls-remote --heads https://github.com/qpwo005451/new-api.git prod/251` returned no matching head
- Fork branch head observed during production wiring verification: `f8ef93a9a1526aa3a9e0558744895cb540abccb9`
- Wiring-time fork provenance note: `origin/prod/251` existed on the fork and resolved to `f8ef93a9a1526aa3a9e0558744895cb540abccb9` during the production remote wiring verification captured by this report.

## Production Remote Wiring

- Target repo: `10.0.0.251:/opt/new-api`
- Production git remotes rewired to:
  - `origin => https://github.com/qpwo005451/new-api.git`
  - `upstream => https://github.com/QuantumNous/new-api.git`
- Safety boundary for Task 6:
  - only git remote wiring was changed on production;
  - the production-side fetch activity stayed inside the Task 6 Step 4 remote wiring verification boundary as remote metadata fetch / remote-tracking refresh;
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
- Post-change `git rev-parse origin/prod/251`: `f8ef93a9a1526aa3a9e0558744895cb540abccb9`
- Post-change `systemctl is-active new-api.service`: `active`
- The production live branch and live commit remained unchanged across the remote rewiring.

## Notes

- Task 6 Step 4 explicitly defined remote wiring verification to include production-side remote metadata fetch plus `git rev-parse origin/prod/251`; the production repo therefore performed only remote metadata fetch / remote-tracking refresh for verification and provenance capture, not branch checkout, source mutation, build, or service restart.
- The actual production-side refresh stayed narrow: it refreshed the remote-tracking ref needed for `git rev-parse origin/prod/251`, and the wiring-time verified fork head was `f8ef93a9a1526aa3a9e0558744895cb540abccb9`.
- The report-bearing branch head may advance with later report-only refinements. Before this self-reference fix, a report-only clarification commit had advanced GitHub `prod/251` to `f3b51ad67dc9045a20235c89cfa638bb6f8fee3e`; that later branch movement did not change the wiring-time verified fork head recorded above.
- Expected release helper scripts under `/opt/new-api/scripts/` were absent during this task:
  - `build_release_candidate.sh`
  - `stage_release_runtime.sh`
  - `smoke_release.sh`
  - `cutover_release.sh`
  - `rollback_release.sh`
- Those missing scripts did not block Task 6 because this task did not perform `prepare`, `verify`, `cutover`, or `rollback`.
- Next safe action: `prepare` after the required release helper scripts are added or confirmed present
