# 2026-10-07 Per-Model Routing Allocation Card - rc01 Prepare And Verify

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api` (production port `4002`, candidate port `4003`).

Release: `2026-10-07-routing-allocation-card-rc01` at `prod/251` `583e8ddf9c09f816e90be4e04d936e15b65d07b8`.

Action class: `prepare` + candidate `verify`. **No cutover, no rollback, no service mutation.**

## Change

- The desktop Model Routing page replaces the exception-override table with one traffic-allocation card per model: header (model name, channel count, per-model `Preset` dropdown, `Average split`, `Advanced`), one percentage input per channel, tier subheaders only when the model has more than one priority tier, and a single draft write path (nothing is written until the page-level `Save changes`).
- Channel selection is enabled channels that serve the model or already carry an override; disabled channels are never listed and their existing override entries are preserved byte-identical on save.
- Percentage -> weight uses the existing `normalizeRatioWeights`; each priority tier is normalized independently.
- The now-dead custom-ratio dialog and its helpers are deleted; the always-visible help sentence moved into a tooltip.
- i18n: seven new English source strings translated into all seven locales (en, zh, zh-TW, fr, ja, ru, vi).

No Go source change, no schema change, no migration, no billing change, no change to channel candidate selection or weighting.

## Source

- Integration branch `codex/routing-allocation-card` merged into `prod/251` by fast-forward to `583e8ddf9` and pushed as `3a5fc38cb..583e8ddf9`; `prod/251` == `origin/prod/251`.
- Merge tree is identical to the verified integration head (`986dd4d74074872ba238546f968e0b87071409cd`).
- Diff `3a5fc38cb..583e8ddf9` touches 13 files, all under `web/` (6 model-weight sources/tests plus 7 locale JSON files); `git diff --stat -- '*.go'` is empty.

## Local Acceptance

- `git rev-parse HEAD` = `583e8ddf9c09f816e90be4e04d936e15b65d07b8`, working tree clean.
- `go test -count=1 -timeout 540s ./setting/operation_setting ./model` - ok (0.028s / 9.375s).
- `bun install --frozen-lockfile` - 1228 packages.
- Full frontend suite `NODE_ENV=test vitest run` - **207 of 207 files, 2355 passed**, exit 0 (the `token-trend-spec` canvas flake did not reproduce).
- `bun run typecheck` (`tsgo -b`) - exit 0.
- `bun run lint` - exit 1 with exactly the pre-change baseline of 67 warnings / 182 errors; no finding in any release-changed file.
- `bun run mobile:locales` - exit 0, 104 keys in each of the seven locales.
- Changed-surface suites `src/features/system-settings/models` + `src/mobile/features/routing-weights` - 12 files / 133 tests passed, exit 0.

## Candidate Build And Upload

- `scripts/build_release_candidate.sh 2026-10-07-routing-allocation-card-rc01 583e8ddf9c09f816e90be4e04d936e15b65d07b8` exit 0; Bun 1.4.2, Go 1.27.1, CGO disabled, `linux/amd64`, detached source worktree removed afterwards.
- Binary sha256 `7372604e4112ea978aac0835a91bb5195de7155b67290f94e782f1171ff0d57c`, 140038304 bytes, stripped ELF 64-bit x86-64.
- `FRONTEND_CACHE_HIT=0` (the web tree changed, so the embedded desktop and mobile bundles were rebuilt from source for this commit).
- Binary and manifest uploaded to `/opt/new-api/releases/2026-10-07-routing-allocation-card-rc01/`; the remote `sha256sum` equals the manifest exactly and the remote manifest file is byte-identical to the local one.

## Candidate Staging Isolation

- Staged with `VIRTUAL_POOL_STAGING_ISOLATED=1 scripts/stage_release_runtime.sh 2026-10-07-routing-allocation-card-rc01` from the host checkout `/opt/new-api` (exit 0).
- Candidate PID `1731825` owns port `4003`; `readlink -f /proc/1731825/exe` is the release `bin/new-api`.
- Candidate runtime database is `/opt/new-api/releases/2026-10-07-routing-allocation-card-rc01/runtime/new-api.db` with `PORT=4003`, `SQL_DSN=local`, `NODE_TYPE=slave`; no reference to `/opt/new-api/data/new-api.db` or port `4002`.
- Copied database schema hash identical before and after startup (`87bd8254bb3719e128dbab52c5cbf306b7103c1172cf8c1684f44c8f418af8cf`), no `schema-changed.flag`.
- Production port `4002` stayed on its original PID `1406273` and `new-api.service` was never stopped, started or restarted.

## Candidate Verification (port 4003)

- `GET /` 200; `GET /api/status` 200 (`success=true`); authenticated `GET /v1/models` 200 with 62 models; `POST /v1/chat/completions` 200 and `POST /v1/responses` 200 with `deepseek-v4.1-flash`.
- Authenticated admin settings surface: `GET /api/option/` 200 and `GET /api/option/request_policy` 200 with the copied database root PAT; a relay API token correctly returns 401 on the `RootAuth` route.
- Fixed channel samples: `deepseek-baseline` (`deepseek-v4.1-flash`) 200 via channel 9; `ollama-baseline` (`gpt-oss:20b`) 200 via channel 46.
- Required smoke: `VIRTUAL_POOL_SMOKE_ISOLATED=1 SMOKE_MODEL=deepseek-v4.1-flash scripts/smoke_release.sh http://127.0.0.1:4003 <candidate-db> full` printed `smoke full ok: http://127.0.0.1:4003 model=deepseek-v4.1-flash`.
- Patch and override checks: stream-scanner timeout customization present; image-generation filter present; `patches/local-option-overrides.json` sha256 `cc1e8fffa62bb9b7ca3d3af13b972df6522c6e1098635c790dc9d25cca8683b0` matches the manifest with 69 ModelRatio / 69 CompletionRatio / 69 CacheRatio entries including `deepseek-v4.1-flash` 0.15/4.0/0.02, `gpt-oss:20b` 0.009, `deepseek-v4-flash` 0.07 and the `kimi-k2.7-code` tiered expression.
- Served-bundle proof (this release changes no Go source, so the binary differs only in embedded frontend assets): desktop `/static/js/index.cba5c67ce7.js` sha256 `82afa791269d3a978729c113d708b1d84f87c95e6a57ca1f425792475bbf8a73` contains `Average split`, `No enabled channel serves this model.` and the locked-tier copy `Channel data is unavailable, so this tier keeps its current weights.`; mobile `/m/static/js/index.c0a26bbaa1.js` sha256 `36f5098a1338c6468c985af2b2b047117b39a3c3c8cb95788caf4557ecd86cd2` contains the locked-tier copy. The served desktop entry and hash were re-checked independently by the coordinator.

## Read-Only Production Baseline (before cutover)

- `new-api.service` active, `ActiveEnterTimestamp` Wed 2026-10-07 10:41:21 CST, zero `journalctl -u new-api -p err` entries since then.
- Production PID `1406273` owns port `4002`; live `/opt/new-api/new-api` sha256 `c284f2dca293a2d2418d0f85743dce6d2622e5324617ce009318c5598fbcc1b8` (release `2026-10-07-routing-ratio-rc01`).
- `/opt/new-api` checkout on `prod/251` at `3a5fc38cbafa4899e0fe06985a0a94c906474afb` with only the pre-existing ` M watchdog.sh` working-tree modification.
- Release directories on the host: `2026-09-29-claude-effort-rc01`, `2026-09-29-rc40`, `2026-09-30-usage-log-refresh-rc01` and `2026-10-07-routing-ratio-rc01` finalized; `2026-09-29-model-health-rc01` and this candidate not finalized.
- Only `4002` and `4003` listen for `new-api`; the host has 44 GB free on `/` and the candidate database copy is 856 MB.

## Risks And Notes

- **Non-blocking, explained**: the verify pass initially flagged a mismatch between the manifest's `LOCAL_OPTION_OVERRIDE_HELPER_SHA256` (`d6da0df8...`) and the host helper (`74c56d78...`). Investigated: this is a line-ending artifact, not content drift. The local build worktree checks the file out with CRLF (`core.autocrlf=true`, 87 CRLF lines), so the raw working-tree hash differs from the LF blob; `git show 583e8ddf9:patches/apply-local-option-overrides.py` and the host's `git show HEAD:...` are the same blob `74c56d78...`, and the host `git status` is clean for `patches/`. The override manifest JSON matches exactly on both sides. No action required.
- The candidate is staged but **not finalized**: port `4003` and the release directory stay in place until an operator-confirmed cutover and finalization.
- `TestRequestPolicyDatabaseMatrix` was not re-run; this release changes no schema, migration or GORM tag.


## Production Cutover (operator-confirmed)

- Operator confirmation was given in-thread for release `2026-10-07-routing-allocation-card-rc01`, target `10.0.0.251:/opt/new-api` (port `4002`, `new-api.service`), the stop/backup/replace/restart cutover, and the rollback handle. The operator also confirmed the following sequence: cutover, post-cutover smoke and production verification, then finalization.
- The host checkout `/opt/new-api` was fast-forwarded to `583e8ddf9c09f816e90be4e04d936e15b65d07b8` before the binary swap (`git fetch origin prod/251` + `git merge --ff-only`). The pre-existing local ` M watchdog.sh` modification was preserved and is untouched by the merge (`git diff --stat 3a5fc38cb 583e8ddf9 -- watchdog.sh` is empty).
- `cd /opt/new-api && timeout 900s scripts/cutover_release.sh 2026-10-07-routing-allocation-card-rc01` exited 0 and reported `smoke fast ok: http://127.0.0.1:4002`. This is a stop/backup/replace/restart cutover, not zero-downtime.
- Post-cutover state, independently re-checked by the coordinator:
  - live `/opt/new-api/new-api` sha256 `7372604e4112ea978aac0835a91bb5195de7155b67290f94e782f1171ff0d57c`, matching the manifest and the verified candidate;
  - `new-api.service` active with a new `ActiveEnterTimestamp` Wed 2026-10-07 15:07:36 CST;
  - port `4002` moved from PID `1406273` to PID `1764190`; the `4003` candidate PID `1731825` was untouched;
  - `GET /` 200 and `GET /api/status` 200 on `http://127.0.0.1:4002`;
  - `journalctl -u new-api -p err --since 2026-10-07 15:07:30` reports no entries;
  - `sqlite3 /opt/new-api/data/new-api.db "PRAGMA quick_check;"` returns `ok` and the schema hash is unchanged;
  - the served production desktop entry is `/static/js/index.cba5c67ce7.js` (sha256 prefix `82afa791269d3a978729c113`) and contains this release's new copy.
- Rollback handle: `/opt/new-api/releases/2026-10-07-routing-allocation-card-rc01/runtime/cutover-backup.env`, with `PREVIOUS_BINARY_SHA256=c284f2dca293a2d2418d0f85743dce6d2622e5324617ce009318c5598fbcc1b8`, `BACKUP_BIN=.../runtime/live-new-api.20261007-070727.bak` and `BACKUP_DB=.../runtime/live-new-api.db.20261007-070727.bak`. Because this release changes no schema, a rollback only needs the previous binary.

## Rollback Plan

- Cutover is stop/backup/replace/restart, not zero-downtime. The previous live binary is `c284f2dca293a2d2418d0f85743dce6d2622e5324617ce009318c5598fbcc1b8`; `cutover_release.sh` writes the rollback handle to `/opt/new-api/releases/2026-10-07-routing-allocation-card-rc01/runtime/cutover-backup.env` and timestamped `live-new-api.*.bak` / `live-new-api.db.*.bak` copies.
- Because this release changes no schema, a rollback only needs the previous binary.

## Next Safe Action

Production-scope verification against port `4002`, then `scripts/finalize_release.sh 2026-10-07-routing-allocation-card-rc01` once stability is confirmed, then local release cleanup.

Status at time of writing: `prepare`, candidate `verify` and the operator-confirmed production `cutover` are complete; production runs `2026-10-07-routing-allocation-card-rc01` and production-scope verification is in progress.
