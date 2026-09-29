# 2026-09-29 rc.40 Production Migration, Cutover, And Finalization Report

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api` (port 4002). Action classes executed:
explicit production `migration` + candidate `verify` + production `cutover` + `finalize`.

## Release Identity

- Release id: `2026-09-29-rc40`
- Source commit: `f053d74e3` (`codex/merge-rc40`: upstream `v1.0.0-rc.26` ... `v1.0.0-rc.40` merged into the fork, plus the
  Opus 5.5 `supportsDisable` guard). Pull request: `qpwo005451/new-api#19` (base `prod/251`).
- Candidate binary sha256: `43858fcb7ccc37f53809a45b9fbbfd46e6cb30564cbc5750f00a6774e1edce58`
  (local build, `build_release_candidate_local.ps1`, manifest matches the uploaded artifact).
- Previous live binary sha256: `ed5ec59ab1724147c25a369bdf833a090978d399258fc0440245439e0b9ba98a`.

## Why The Migration Was Needed

The first candidate verify on `4003` failed closed: authenticated `GET /v1/models` returned 500 with
`SQL logic error: no such column: users.access_token_created_at`, plus `no such table: task_plugins`
(see `ops/reports/2026-09-29-rc40-upgrade-candidate-verify-blocked-report.md`). The production schema predated the
merged rc.27 ... rc.40 model set, and `stage_release_runtime.sh` deliberately runs the candidate as `NODE_TYPE=slave`,
so the staged runtime never runs `migrateDB()`.

## Production Migration (explicit, itemized)

- Rehearsal first, on a copy only: fresh `.backup` copy of the live database plus the release binary running as
  `NODE_TYPE=master` inside a `unshare -n` namespace (no egress, so no upstream request can occur). Result: `ready=1`,
  auth `GET /v1/models` returned 200 on the migrated copy, `PRAGMA integrity_check` = ok, row counts unchanged.
- Itemized change set actually applied to `/opt/new-api/data/new-api.db` (purely additive):
  - new table `audit_logs` (+ indexes `idx_audit_logs_created_at`, `idx_audit_logs_category`, `idx_audit_logs_event_id`,
    `idx_audit_logs_request_id`, `idx_audit_logs_username`, `idx_audit_token_time`, `idx_audit_user_time`),
  - new table `login_encryption_keys` (+ `idx_login_encryption_keys_slot`),
  - new table `task_plugins` (+ `idx_task_plugins_active`, `uk_task_plugin_key_version`),
  - new nullable column `users.access_token_created_at` (`bigint`),
  - new column `passkey_credentials.rp_id`.
  - No table dropped, no column removed, no row rewritten.
- Backups taken before the change: `/opt/new-api/data/backups/new-api.20260929-124356.premigrate.bak`
  (binary, sha `ed5ec59a...`) and `/opt/new-api/data/backups/new-api.db.20260929-124356.premigrate.bak`
  (database, sha256 `69f9032d38e14adeb395f3b1958332cd39200bdaeea546b8a0a55d10a6c16bfd`).
- Window: `new-api.service` stopped, migration process ran 1 s, previous binary restarted. Measured service downtime
  **5 seconds**. `PRAGMA integrity_check` = ok before and after, `foreign_key_check` clean, row counts preserved
  (users/tokens/channels/logs `1/6/23/695565` before, `1/6/23/695567` after).
- Rollback tolerance proof: the previous binary (`ed5ec59a...`) was restarted on the migrated schema and served `GET /`
  200, `GET /api/status` 200 and authenticated `GET /v1/models` 200. The additive schema therefore does not block a
  binary-only rollback.

## Candidate Re-Verification (port 4003, isolated database copy)

- `stage_release_runtime.sh 2026-09-29-rc40` with `VIRTUAL_POOL_STAGING_ISOLATED=1`: candidate PID `1050236` owns `4003`,
  exe = `releases/2026-09-29-rc40/bin/new-api`, cwd = `releases/2026-09-29-rc40/runtime`,
  `SQLITE_PATH = .../runtime/new-api.db`; production `4002` and the live database were untouched;
  `schema-before == schema-after` and no `schema-changed.flag`.
- Checks: `GET /` 200, `GET /api/status` 200, authenticated `GET /v1/models` 200 (61 models, including both fixed channel
  samples and the virtual routes `auto-subagent-codex` / `auto-free`), `smoke_release.sh full` passed with
  `model=deepseek-v4.1-flash`, ollama sample `gpt-oss:20b` returned 200, settings surface `GET /api/option/` returned 200
  with the root user access token, zero `500` responses in the candidate log.
- Patch and override assets confirmed present: `/opt/new-api/patches/local-option-overrides.json` (group/model/completion/
  cache ratios plus `billing_setting.billing_expr` for `kimi-k2.7-code`), `/opt/new-api/patches/patch-image-gen-filter.py`,
  `apply-local-option-overrides.py`, `post-rebuild-patches.sh`, `scripts/channel_guard.py`, `scripts/input_budget_guard.py`.
  The stream-scanner buffer customization is source-level in the fork (`relay/helper/stream_scanner.go`,
  `DefaultMaxScannerBufferSize`) and therefore compiled into this candidate, which was built from `f053d74e3`.

## Cutover

- `cutover_release.sh 2026-09-29-rc40` completed with `CUTOVER_RC=0` in 6 seconds (systemd stop 20:47:27, start 20:47:29).
- Live `/opt/new-api/new-api` sha256 now equals the candidate and the manifest (`43858fcb...`); `new-api.service` active,
  MainPID `1052202`; startup banner `New API f053d74e3 started`.
- Rollback handle: `/opt/new-api/releases/2026-09-29-rc40/runtime/cutover-backup.env`
  (previous binary `ed5ec59a...`, database snapshot `runtime/live-new-api.db.20260929-124725.bak`).
- The restarted binary ran `migrateDB()` again as master. Only side effect: GORM normalized the `users` table definition
  (rebuilt the table text and recreated its 14 indexes). Statement count stayed 272, tables 47, indexes 214, and a sorted
  schema comparison against the verified rehearsal/staged schema differs by one cosmetic space only. Row counts after
  cutover: users/tokens/channels/logs `1/6/23/695589`, `PRAGMA integrity_check` = ok.

## Post-Cutover Production Verification

- `GET /` 200, `GET /api/status` 200, authenticated `GET /v1/models` 200, settings surface `GET /api/option/` 200.
- `smoke_release.sh full` passed on production with the fixed deepseek sample (`model=deepseek-v4.1-flash`, chat
  completions and responses both exercised through the dedicated route).
- Fixed channel sample `ollama-baseline` (`gpt-oss:20b`) returned 200 through the ollama cloud channel.
- Zero `500` responses in the journal window since cutover.
- Observed upstream flakiness (not a regression): channels #9, #36 and #21 returned 502/503 for the first attempts of the
  smoke requests on both the previous binary (20:44) and the new binary (20:47); the gateway retried and the requests
  succeeded.
- `finalize_release.sh 2026-09-29-rc40` completed: `finalized.env` written, candidate binary preserved, rollback metadata
  preserved, port `4003` released, staged database copy removed.

## Residual Notes

1. Live binary provenance gap: `ed5ec59a...` (previous live) matches neither the last archived manifest (`522f3b02...`)
   nor the older release manifest (`b1654690...`). Worth an explanation before the next cutover.
2. Open product question (unchanged by this work): adaptive Claude requests without an explicit effort tier are accounted
   as `high` in both upstream and the merged tree, while the requirement document asks for `medium` as the Opus 5.5
   default.
3. Scope note: the pre-existing instruction not to use paid upstream probes was applied to evaluation work. The standard
   validation matrix mandates the two fixed channel samples, and the operator precedent report
   (`2026-09-29-model-health-same-tier-retry-cutover-report.md`) performs a real model smoke, so both samples were
   executed here with the matrix payloads (single-token completions) on the staged runtime and on production.
