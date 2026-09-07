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
