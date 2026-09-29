# 2026-09-29 rc.40 Upgrade Candidate Verification Report (blocked)

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api` (port 4002). Action class: `prepare` (done) + candidate
`verify` (**failed**) — `cutover` was withheld by the action contract.

## Release under test

- Release id: `2026-09-29-rc40`
- Source: `codex/merge-rc40` head `f053d74e3` (upstream `v1.0.0-rc.26` … `v1.0.0-rc.40` merged into the fork, plus the
  Opus 5.5 `supportsDisable` guard). Pull request: `qpwo005451/new-api#19` (base `prod/251`).
- Built locally with `scripts/build_release_candidate_local.ps1 -ReleaseId 2026-09-29-rc40 -ReleaseTag f053d74e3`
  (`FRONTEND_CACHE_HIT=0`), binary sha256 `43858fcb7ccc37f53809a45b9fbbfd46e6cb30564cbc5750f00a6774e1edce58`.
- Uploaded to `/opt/new-api/releases/2026-09-29-rc40/bin/new-api` and `.../manifest.env`; the host-side sha256 matches the
  local manifest exactly.

## Candidate staging (passed)

- `stage_release_runtime.sh 2026-09-29-rc40` started the candidate on port `4003`, PID `1011917`, binary
  `/opt/new-api/releases/2026-09-29-rc40/bin/new-api --port 4003`, cwd
  `/opt/new-api/releases/2026-09-29-rc40/runtime`, copied database
  `/opt/new-api/releases/2026-09-29-rc40/runtime/new-api.db` (755 MB snapshot of the live database).
- Candidate proofs: `ss` shows the candidate PID owns `4003`; `GET /` and `GET /api/status` return 200; production `4002`
  and `new-api.service` were untouched throughout.

## Verification (failed)

- `GET /v1/models` (authenticated) returned **500**:
  `TokenAuth GetUserCache error for user 1: SQL logic error: no such column: users.access_token_created_at`.
- The candidate also logs `sync task plugins: SQL logic error: no such table: task_plugins`.
- The staging helper's own schema check agrees that nothing was migrated: `runtime/schema-before.sha256` equals
  `runtime/schema-after.sha256`, and `runtime/schema-changed.flag` was never written.
- Both gaps are **pre-existing in production**: `PRAGMA table_info(users)` on the live database has no
  `access_token_created_at` (30 columns total), and production has no `task_plugins` table.
- Consequence: cutting over to `2026-09-29-rc40` today would break every authenticated request, so `cutover` is blocked by
  the action contract ("successful recent verify") and by the production-write red line.

## Open question for the operator

The startup path (`main.go` -> `model.InitDB` -> early return when the node is not master -> `migrateDB()` ->
`DB.AutoMigrate` of the model list, `User` included) should create these objects; the candidate did not. Candidates to
confirm on the database copy before touching production: (a) how the master-node flag evaluates at the point `InitDB`
runs, (b) an early return inside `migrateDB()`, (c) the fork's curated schema path intentionally skipping this
table/column. The plan already reserved this decision (`merge-rc40-tdd-plan.md` item D3/P4: the migration action needs
explicit per-item approval).

## Side observation — live binary provenance gap

`sha256sum /opt/new-api/new-api` = `ed5ec59ab1724147c25a369bdf833a090978d399258fc0440245439e0b9ba98a`, which matches
neither the last archived manifest (`522f3b02…`, release `2026-09-29-model-health-rc01`) nor the value recorded in
`ops/reports/2026-09-29-model-health-same-tier-retry-cutover-report.md`. `cutover_release.sh` backs up the live binary
before promotion, so rollback remains well defined, but the gap is worth an explanation.

## Notes on isolation

The staging step ran with `VIRTUAL_POOL_STAGING_ISOLATED=1` and the smoke with `VIRTUAL_POOL_SMOKE_ISOLATED=1`, matching the
practice recorded in `ops/reports/2026-09-29-model-health-same-tier-retry-cutover-report.md` (candidate on `4003` with an
isolated database copy). The smoke failed at `GET /v1/models` before any relay call, so no upstream request or spend
occurred. The `4003` candidate was stopped after verification; its runtime directory is kept for forensics.
