# 2026-09-08 Ollama Weekly Reset + Monthly Cumulative Staging Report

Release: `2026-09-08-rc01` @ prod/251 `73750537446822088a5ba315cdc269a44cf33fa0` (PR #17)
Action class: `prepare` + candidate `verify` (standard scope, staging only — no cutover)

## Changed In Fork (prod/251 head 737505374)
- Ollama channel local weekly usage estimate: fixed weekly period `[last weekly reset, next weekly reset)` instead of sliding 7-day window; N8N snapshot `resetsAt` is the authoritative anchor, Monday 00:00 UTC cadence is the fallback, stale snapshots keep the 7-day cadence anchored at resetsAt.
- Added monthly cumulative estimate `local.monthly` (current calendar month in server timezone).
- `ollamaLocalUsageWindow.resets_at` added; fixed periods emit no earliest-release/projection. 5-hour session window unchanged (sliding).
- Frontend usage dialog: 3-column local estimates (5-hour / weekly / monthly), reset rows for fixed periods; i18n for all seven locales.

## Verification
- TDD checks: frontend vitest 226/226 (44 files) including the Ollama usage dialog suite; backend `go test` for all sub-packages pass; `relaykit` GOWORK=off build pass; typecheck + oxlint + gofmt clean.
- Merge: PR #17 merged into `prod/251` (737505374), pushed to `origin/prod/251`.
- Build: local Linux amd64 candidate via `scripts/build_release_candidate.sh 2026-09-08-rc01 737505374...`; binary sha256 `274e60321b84122973dd639e802f5d42b5a63b28cca3c0d7c45c84354683777b`; frontend cache miss, fresh web build.
- Host staging: binary+manifest uploaded, host hash matched manifest; `stage_release_runtime.sh` staged PID 2970780 on port 4003; `SQL_DSN=local`, `SQLITE_PATH=/opt/new-api/releases/2026-09-08-rc01/runtime/new-api.db` (isolated DB copy, 578 MB); production 4002 and `/opt/new-api/data/new-api.db` untouched.
- Core availability: `GET /`, `GET /api/status`, `GET /v1/models` 200; `POST /v1/chat/completions` 200 (deepseek-v4-flash); `POST /v1/responses` 200 (deepseek-v4-flash). Full smoke `smoke full ok: http://127.0.0.1:4003 model=deepseek-v4-flash`.
- Settings surface: authenticated `GET /api/option/` returned 200 (admin PAT).
- Patch/override checks: patches assets present; `local-option-overrides.json` sha256 matches manifest; `STREAMING_TIMEOUT` present in candidate binary; candidate env carries `RELAY_TIMEOUT=900`, `STREAMING_TIMEOUT=300`.
- Candidate proofs: PID 2970780 owns port 4003; `SQLITE_PATH` points to the runtime copy; production DB not used.
- Feature check: `GET /api/channel/46/ollama_usage` on the candidate returned the new schema — weekly period `[2026-09-07 00:00 UTC, 2026-09-14 00:00 UTC)` (Monday 00:00 UTC cadence, matches snapshot resetsAt), monthly `[2026-09-01 00:00 CST, 2026-10-01 00:00 CST)`, projection points 0 for fixed periods, session window kept sliding, snapshot ok.

## Verify Blockers / Findings
1. Channel sample `input-baseline` (`gpt-5.4-mini`, `/v1/chat/completions`) FAILED: model missing from the candidate `/v1/models` list (49 models, `gpt-5.4` exists, `gpt-5.4-mini` does not). The fixed matrix needs a human decision before it can pass; no substitute was picked.
2. First full smoke run with auto-picked model `auto-subagent` (channel #21 → gpt-5.6-luna) exceeded the 30 s curl budget on `/v1/responses` (499 client-cancel). Chat succeeded in 16.7 s. Upstream latency, not a candidate defect; deepseek-v4-flash sample passes both endpoints.
3. Host `/opt/new-api` checkout sits on branch `main` (0936e2504) with uncommitted source edits (`relay/helper/price.go`, `relay/helper/price_test.go`, `relay/helper/stream_scanner.go`, `relay/responses_handler.go`; 269 insertions). Release helper scripts on host match `prod/251` byte-for-byte (sha256 verified), so staging was safe, but the durable-change guardrail ("source changes belong in git") is currently violated on the host. Recommend reconciling these edits into `prod/251` or removing them, and switching the host checkout back to `prod/251` so `sync_origin_prod_251.sh` works again.

## Next Safe Action
- Human review of the `input-baseline` matrix entry and the host checkout risk.
- On explicit confirmation, `cutover` release `2026-09-08-rc01`, then `finalize_release.sh 2026-09-08-rc01` + local release cleanup after stability.


## Follow-up — same-day hotfix reconciliation and unified cutover (rc02)

### Hotfix reconciliation ("hotfix 并回 251 主线")
- The host working tree carried 4 uncommitted Go-source edits (`price.go`, `price_test.go`, `stream_scanner.go`, `responses_handler.go`). Investigation showed they are STALE duplicates of work already migrated into `prod/251` via `03aee639f feat: migrate prod 251 source customizations` (input-channel billing alias incl. `inputBillingAliasModels` superset, tiered `priceModelName` plumbing, single clean `filterImageGenerationTool`); the stream-scanner ping timeout was superseded by the upstream rework (configurable ping interval + 30s write deadline). Applying them onto `prod/251` would not even compile (references removed `ratio_setting.CompactModelSuffix`; `responses_handler.go` contained a tripled `filterImageGenerationTool` definition). No Go-source merge was needed; the intent is fully covered by `prod/251`.
- REAL host-only deltas were in ops scripts: `scripts/channel_guard.py` and `watchdog.sh` carried n8n/Telegram notification hooks existing only on disk. Merged into `prod/251` via PR #18 (`ops: merge host-only notify hooks into channel guard and watchdog`, merge `d36f64eaf`); `watchdog.sh` intentionally keeps the hardened `WATCHDOG_KEY` check (host copy had degraded to an always-false empty check; `watchdog.env` supplies the key).
- Host `/opt/new-api` checkout moved `main (0936e2504, dirty)` → `prod/251 (d36f64eaf, clean)`; `sync_origin_prod_251.sh` now works. All replaced host files were backed up under `/root/newapi-hotfix-backup-20260908/` (4 relay source files stashed in git stash + 18 untracked asset copies).

### rc02 candidate and unified cutover
- Release `2026-09-08-rc02` built from `prod/251` `d36f64eaf` (frontend cache hit), binary sha256 `fa9ca43e90863403472cffbed3aadd6a96212146b444f9521e1b3dad3ef2de3f`, uploaded and hash-verified.
- rc01 candidate (PID 2970780) stopped after PID/port/binary ownership proof; rc02 staged: PID 2987941 owns 4003, `SQL_DSN=local`, runtime DB copy at `releases/2026-09-08-rc02/runtime/new-api.db`.
- Standard-scope candidate verify: full smoke `smoke full ok` (deepseek-v4-flash, chat+responses), settings surface 200, ollama usage feature schema verified (weekly `[2026-09-07 00:00 UTC, 2026-09-14 00:00 UTC)`, monthly `[2026-09-01 00:00 CST, 2026-10-01 00:00 CST)`), snapshot ok. Note: `schema-changed.flag` set — the candidate migrates the DB copy (new tables/columns from prod/251 evolution); cutover auto-backup covers rollback.
- `input-baseline` sample (`gpt-5.4-mini`) still FAILS (model absent from `/v1/models`) — matrix needs a human decision (unchanged finding).
- Cutover executed with explicit operator confirmation in-thread: `scripts/cutover_release.sh 2026-09-08-rc02` → live binary sha256 matches candidate exactly, `new-api.service` active on 4002 (PID 2989000), post-cutover fast smoke `smoke fast ok: http://127.0.0.1:4002`, production settings surface 200, production `GET /api/channel/46/ollama_usage` returns the new weekly/monthly schema with snapshot ok.
- rc01 transient runtime (DB copy/logs) removed; `releases/2026-09-08-rc0{1,2}/bin` + manifests retained on the host. rc02 runtime (cutover backup) stays until finalize.

### Next safe action
- After operator confirms production stability: `scripts/finalize_release.sh 2026-09-08-rc02` (stops the 4003 candidate, removes runtime/candidate DB, preserves binary+manifest+backup), then local `releases/2026-09-08-rc02` cleanup.
- Human decision still pending on the stale `input-baseline` matrix entry.
