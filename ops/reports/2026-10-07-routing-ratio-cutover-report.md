# 2026-10-07 Per-Model Routing Ratio Cutover

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api` (port `4002`).

Release: `2026-10-07-routing-ratio-rc01` at `prod/251` `40620ec2f937bb9df6f8b718fad5229575a6d264`.

Action class: `prepare` + candidate `verify` + explicit production `cutover` + `finalize`.

## Change

- The Model Routing settings page now groups channel model overrides by model. Each group has one ratio selector that either applies a preset scoped to that single model or opens a custom-ratio dialog.
- A model weight preset gained an optional `model` scope field. An empty scope stays a global preset and replaces the whole table, so existing persisted presets keep their exact behaviour. A non-empty scope replaces only that model's rows, and every weight row of a scoped preset must belong to the scope.
- Preset uniqueness is `(scope, name)` case-insensitively, so one preset name may exist once globally and once per model.
- The mobile console gained a `Model routing` tab with real write capability against the same option.
- The custom-ratio dialog now treats a priority tier whose effective weights cannot all be resolved as **locked**: it is read-only, keeps its current weights, and writes nothing for its entries. Before this fix both the desktop and the mobile dialog encoded such a tier as `0` percentages and then wrote `weight: 0` onto every entry, which changed live routing without operator intent.
- `applyPresetToModel` now ignores preset rows whose model does not match the preset scope (defense in depth; the backend already rejects those presets on write).

No schema change, no migration, no billing change, and no change to the channel candidate or weighting algorithm.

## Source And Tests

- Integration branch `codex/model-routing-ratio-selector` at `d0dc5867f`, merged into `prod/251` as `40620ec2f937bb9df6f8b718fad5229575a6d264` and pushed to `origin/prod/251` as a fast-forward (`1665f7e4c..40620ec2f`).
- The merge commit tree is byte-identical to the verified integration head (`c46ea8db062757703d849c0b3c66351fc84b6ee2`).
- `go test -count=1 -timeout 600s ./setting/operation_setting ./model` — ok.
- `bun run build:check` (tsgo + rsbuild production build) — exit 0. `bun run build:mobile` — exit 0. `bun run typecheck` — exit 0.
- `bun run lint` — 67 warnings / 182 errors, identical to the pre-change baseline; no finding in a changed file.
- Full `bunx vitest run` — 206 of 207 files, 2360 passed / 2 skipped. The single failure is `src/features/dashboard/components/overview/__tests__/token-trend-spec.test.ts`, whose `beforeAll` hook times out at 30 s while loading the `canvas` module under full-suite load; that file passes in isolation (3 files / 19 tests green in 5.3 s) and shares no code with this change.
- `bun run mobile:locales` — 104 keys across all seven locales.
- Locale key sets went from 7171 to 7184 entries with zero keys removed and zero values changed in any of `en`, `zh`, `zh-TW`, `fr`, `ja`, `ru`, `vi`.
- An independent read-only Codex audit returned REQUEST CHANGES with two MAJOR findings, both of which were the locked-tier data-mutation defect above. Both were fixed and covered by 33 new desktop regression tests and 4 new mobile regression tests, each verified to fail before the fix and pass after it.

## Candidate Build And Isolation

- Built locally with `scripts/build_release_candidate.sh 2026-10-07-routing-ratio-rc01 40620ec2f` (the Linux helper; `pwsh` is not installed on this workstation, so the `.ps1` variant was not used). Bun `1.4.2`, Go `1.27.1`.
- Binary SHA-256 `c284f2dca293a2d2418d0f85743dce6d2622e5324617ce009318c5598fbcc1b8`, 140161287 bytes, stripped ELF 64-bit x86-64.
- The frontend was rebuilt from source (`FRONTEND_CACHE_HIT=0`), so the embedded desktop and mobile bundles are the ones produced for this commit.
- Binary and manifest were uploaded to `/opt/new-api/releases/2026-10-07-routing-ratio-rc01/`; the remote hash matched the manifest exactly.
- Staged with `VIRTUAL_POOL_STAGING_ISOLATED=1 scripts/stage_release_runtime.sh`. Candidate PID `1404617` owned port `4003` and used an isolated database copy at `.../runtime/new-api.db` with `NODE_TYPE=slave`, an empty `REDIS_CONN_STRING`, `ENABLE_PPROF=false` and `BATCH_UPDATE_ENABLED=false`.
- The copied database schema hash was identical before and after startup (`d1109c02dad36383ea078db2e2c0be93ed2794f6cb84d1f166a1f9cebc6d3e1f`) and no `schema-changed.flag` was written, confirming the candidate performs no migration.
- Production port `4002` remained healthy throughout staging and verification.

## Candidate Acceptance

- The first `full` smoke attempt failed at the relay call. The smoke auto-selected the virtual route `auto-subagent-codex`, whose channel `#21` returned an upstream Cloudflare `502` after retries `36->36->21->21`. This is an upstream provider failure, not a candidate defect: `/` and `/api/status` returned `200` on the candidate throughout.
- The retry with a fixed known-good sample passed: `smoke full ok: http://127.0.0.1:4003 model=deepseek-v4.1-flash`.

## Production Cutover

- The operator confirmed the release id, the target instance and the cutover in-thread before the first production write.
- The production checkout `/opt/new-api` was fast-forwarded from `c028a65a8` to `40620ec2f`. The host-only working-tree modification to `watchdog.sh` was preserved; that file is unchanged between those two commits, so the fast-forward could not and did not touch it. The live binary is untracked and gitignored, so the checkout step never overwrote it.
- `timeout 900s scripts/cutover_release.sh 2026-10-07-routing-ratio-rc01` exited 0 and `smoke fast ok: http://127.0.0.1:4002` passed.
- This was a stop/backup/replace/restart cutover, not a zero-downtime deployment.

## Production Verification

- `new-api.service` is active, started `2026-10-07 10:41:21 CST`; `journalctl -u new-api -p err --since 10:41` reported no entries.
- Live `/opt/new-api/new-api` SHA-256 is `c284f2dca293a2d2418d0f85743dce6d2622e5324617ce009318c5598fbcc1b8`, matching the candidate and the manifest exactly.
- Only port `4002` is listening; the `4003` candidate was stopped by finalization.
- `POST /v1/chat/completions` and `POST /v1/responses` both returned `200` against production with `deepseek-v4.1-flash`.
- SQLite `PRAGMA quick_check` on `/opt/new-api/data/new-api.db` returned `ok`.
- The served desktop bundle `static/js/index.15babb273b.js` contains the new copy (`Routing ratio`, `Preset scope`, `keeps its current weights`, `Model routing`), and the mobile entry `static/js/index.c0a26bbaa1.js` matches the local `mobile-dist` build, so both frontends on production are the ones built for this commit.
- `GET /api/option/request_policy` returns `401` for a regular API token. This is expected: the route is guarded by `RootAuth` and accepts a dashboard session or a personal access token, not a relay API token. It is not a regression.

## Rollback And Cleanup

- Rollback handle: `/opt/new-api/releases/2026-10-07-routing-ratio-rc01/runtime/cutover-backup.env`.
- Previous live binary SHA-256: `bf5848dbbacda2b6591bca6a1721f3a1ef28f18339865b772075dce8b9c48fcf`.
- Timestamped backups are preserved as `runtime/live-new-api.20261007-024111.bak` and `runtime/live-new-api.db.20261007-024111.bak`. Because this release changes no schema, a rollback only needs the previous binary.
- `scripts/finalize_release.sh` stopped the `4003` candidate and preserved the release binary, manifest and rollback metadata.
- Local cleanup removed the candidate build directory and its dependency trees while retaining the reusable release cache under `.local-tools/release-cache`. The temporary audit worktree and branch were removed.

## Known Limitations

- `TestRequestPolicyDatabaseMatrix` exercised SQLite only (3.50.4, passed); the MySQL and PostgreSQL subtests skip because `TEST_MYSQL_DSN` and `TEST_POSTGRES_DSN` are unset in this environment. This change touches no schema, migration, GORM tag, or raw SQL, so the three-database matrix is not required for it, but it was not run.
- The preset uniqueness key joins scope and name with `|`. A model name containing `|` could in theory collide with a crafted pair. This matches the pre-existing `modelWeightKey` pattern and was accepted rather than changed.

## Next Safe Action

- None required. If the release proves unstable, run `scripts/rollback_release.sh 2026-10-07-routing-ratio-rc01` to restore the previous binary from the recorded backup.
