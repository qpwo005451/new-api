# 2026-09-29 claude-effort-rc01 Production Deployment Report

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api` (port 4002). Action classes: candidate `prepare` +
`verify` + explicit production `cutover` + `finalize`.

## Release Identity

- Release id: `2026-09-29-claude-effort-rc01`
- Source commit: `99510d05fe808517e1b8cc212efb8c16842061e9` (merge commit of PR #20, the Claude family default-effort change)
- Candidate binary sha256: `7129c19f55d77bf7bb30d39879dded20eb05c36b21470b24eef4d17e0b070ff3`
- Previous live binary (release `2026-09-29-rc40`): `43858fcb7ccc37f53809a45b9fbbfd46e6cb30564cbc5750f00a6774e1edce58`
- Frontend release cache hit (`FRONTEND_CACHE_HIT=1`); only backend code changed.

## Why

PR #20 changed the accounting tier for adaptive Claude 5 requests that carry no effort signal (Opus 5.5 defaults to
`medium`, Fable 5.1 keeps `high`). The previously deployed binary was built from `f053d74e3` and therefore did not
contain it.

## Candidate Build And Verify (port 4003, isolated database copy)

- Built locally with `scripts/build_release_candidate_local.ps1`; uploaded to
  `/opt/new-api/releases/2026-09-29-claude-effort-rc01/`; host sha256 matches the manifest.
- `stage_release_runtime.sh` with `VIRTUAL_POOL_STAGING_ISOLATED=1`: candidate PID `1086189` owned `4003`, exe and cwd
  inside the release directory, `SQLITE_PATH` pointing at `runtime/new-api.db`; production `4002` and the live database
  untouched; `schema-before == schema-after` (`e434d4f6...`) and no `schema-changed.flag`.
- `smoke_release.sh ... fast` passed; `smoke_release.sh ... full` passed with `model=deepseek-v4.1-flash`; the fixed
  `ollama-baseline` sample (`gpt-oss:20b`) returned 200; settings surface `GET /api/option/` returned 200; the candidate
  log contains zero responses with status 500.

## Cutover

- `cutover_release.sh 2026-09-29-claude-effort-rc01` completed with `CUTOVER_RC=0` in 11 seconds.
- Live binary sha256 now equals the candidate (`7129c19f...`); `new-api.service` active, MainPID `1087261`; startup banner
  `New API 99510d05fe808517e1b8cc212efb8c16842061e9 started`.
- Rollback handle: `/opt/new-api/releases/2026-09-29-claude-effort-rc01/runtime/cutover-backup.env` (previous binary
  `43858fcb...`, database snapshot `runtime/live-new-api.db.20260929-131621.bak`).

## Post-Cutover Verification

- `GET /` 200, `GET /api/status` 200, authenticated `GET /v1/models` 200, settings surface `GET /api/option/` 200.
- `PRAGMA integrity_check` = ok; 47 tables and 214 indexes, row counts preserved (users/tokens/channels/logs
  `1/6/23/695773` immediately after, `1/6/23/695798` after the verification traffic). Zero responses with status 500 in
  the journal window since the cutover.
- First `smoke_release.sh full` attempt failed with a client timeout (`curl: (28)`): the chosen channels (`#9`, `#36`,
  `#21`) returned 502/503 and the retry chain exceeded the smoke client's 30 second limit; server-side the chat request
  completed on channel `#47` and the `/v1/responses` call was canceled by the client at 30 s (HTTP 499). A re-run passed
  (`smoke full ok`), and manual chat and responses calls each returned 200 in about 2.5 seconds. This is the known
  upstream channel instability, not a release regression.
- Fixed channel samples: `deepseek-baseline` (`deepseek-v4.1-flash`, chat + responses) and `ollama-baseline`
  (`gpt-oss:20b`) both succeeded on production.

## Observations

1. The post-cutover `.schema` hash differs from the pre-cutover hash only by the *order* of the `users` indexes in
   `sqlite_master`; a sorted comparison of both schemas is identical, object counts are unchanged, and
   `integrity_check` is ok. GORM rewrites the `users` table during `migrateDB()` on startup (observed at the previous
   cutover as well). Low priority, pre-existing upstream behavior, no data change.
2. Behavior change delivered by this release: `claude-opus-5*` requests without an explicit effort are accounted at
   `medium` instead of `high`; outbound `output_config.effort` is still omitted when the caller did not request a level.

## Finalization

`finalize_release.sh 2026-09-29-claude-effort-rc01` completed: `finalized.env` written, candidate runtime copies removed,
port `4003` released, candidate binary and rollback metadata preserved.
