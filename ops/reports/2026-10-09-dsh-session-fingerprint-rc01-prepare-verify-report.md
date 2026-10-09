# 2026-10-09 DSH Session Fingerprint Stickiness - rc01 Prepare And Verify

Release: `2026-10-09-dsh-session-fingerprint-rc01`
Source commit: `e1c792cc9994aca04c08d47b7e46b1bb14d85969` (branch `codex/dsh-session-fingerprint`, based on `prod/251` head `993207f57`)
Host: `10.0.0.251:/opt/new-api`, production port `4002`, candidate port `4003`
Status: prepare + candidate verify complete. Production cutover NOT performed and NOT approved yet.

## Why rc01 exists

DeepSeek Harness (`@deepseek-ai/dsh` 0.2.0-rc.2, provider `newapi-cc`, api `openai-completions`) sent no
session identifier at all, so the production rule "deepseek glm session stickiness" matched every request
but extracted an empty key from all nine configured sources:

- headers observed on real traffic: Stainless headers, `authorization`, `user-agent: deepseek-harness/0.2.0-rc.2`
- body observed on real traffic: `model`, `messages`, `stream`, `stream_options`, `store`, `thinking`, `tools`, `max_completion_tokens`
- no `prompt_cache_key`, no `session_id`, no `X-NewAPI-Session-ID` / `Thread_id` family header

With an empty affinity key, each request fell back to the weighted draw in `model_weight_setting.weights`
(`deepseek-v4.1-flash`: channel 9 weight 50, channel 47 weight 5, channel 46 weight 10, priority 500).
Production logs show the resulting flapping between channel 9 and channel 46 for a single session.

## Change

New channel affinity key source type `gjson_fingerprint`: it reads the same gjson body path syntax as the
existing `gjson` source but returns a short hash (`affinityFingerprint`, 8 hex characters) of the extracted
value instead of the raw value, so no prompt text is stored in the affinity key.

- `service/channel_affinity.go`: new `gjson_fingerprint` type, shared `extractChannelAffinityBodyValue(c, path)`
- `setting/operation_setting/channel_affinity_setting.go`: key source type documentation
- `web/src/features/system-settings/general/channel-affinity/{types.ts,rule-editor-dialog.tsx,session-rules-table.tsx}`
- `service/channel_affinity_template_test.go`: body-path fingerprint extraction and rule tests

Rehearsed production rule change (option key `channel_affinity_setting.rules`, appended as the tenth source of
the third rule): `{"type":"gjson_fingerprint","path":"messages.#(role==\"user\").content"}`.
`messages.#(role=="user").content` is the first user message, which was measured to be stable across turns of
one real DSH session (turn 1 and turn 2 of the same session differ only in later messages).

## Local checks (workstation)

- `go test ./service/... ./setting/... ./model/... ./middleware/... ./controller/...` - all ok
- `gofmt -l` on changed Go files - no output
- `cd web && bun run typecheck` (`tsgo -b`) - pass
- `oxlint` on the changed frontend directory - only the pre-existing `constants.ts` `unicorn(prefer-structured-clone)` warning
- `vitest run src/features/system-settings` - 18 files / 218 cases pass
- `bun run format:check` - reports 33 files of pre-existing drift, none of them changed by this release

## Candidate build

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 scripts/build_release_candidate.sh 2026-10-09-dsh-session-fingerprint-rc01 e1c792cc9994aca04c08d47b7e46b1bb14d85969
```

- exit 0; `FRONTEND_CACHE_HIT=0` (web change invalidated the reusable frontend cache, so both frontends were rebuilt from source)
- `BINARY_SHA256=201164c5325430b5274d0c1752741fc6d8cb060ef8bc07ac319465a252ed611b`, size 140042400
- `file` on the artifact: ELF 64-bit LSB executable, statically linked, stripped
- `BUN_VERSION=1.4.2`, `WEB_LOCK_SHA256=bcf11235f1aa60f93ae7c14b7fe21abdc464af97ba21766301b593a4a9952e52`
- `LOCAL_OPTION_OVERRIDES_SHA256=cc1e8fffa62bb9b7ca3d3af13b972df6522c6e1098635c790dc9d25cca8683b0`
- `BUILT_AT=2026-10-09T08:20:33+08:00`

Upload to `/opt/new-api/releases/2026-10-09-dsh-session-fingerprint-rc01/` verified:
remote binary sha256 `201164c5...` equals the manifest, remote manifest sha256 `cc0b042e2c32cbf3f55eab9e12dd72289674f5643c3d2cbaa57c29452f3fbc73` equals the local file.

## Candidate staging (port 4003)

```
cd /opt/new-api && VIRTUAL_POOL_STAGING_ISOLATED=1 scripts/stage_release_runtime.sh 2026-10-09-dsh-session-fingerprint-rc01
```

- candidate PID 537313 owns port 4003, executable is the release binary, cwd is the release `runtime` directory
- candidate env: `PORT=4003`, local `SQL_DSN`, `SQLITE_PATH` pointing at the release `runtime/new-api.db`
- no production database reference and no 4002 reference in the candidate environment
- candidate DB schema hash identical before and after startup (`7650d1e105a2a3c7d6a2f89e126191467b58301b9b4bc63bb49b216ae7565aa7`), no `schema-changed.flag`
- production service untouched throughout: `new-api.service` active, MainPID 1967502 (started 2026-10-07 17:50:35 CST), still listening on 4002

## Candidate verification (port 4003)

Endpoints:

| check | result |
| --- | --- |
| `GET /` | 200 |
| `GET /api/status` | 200 |
| `GET /v1/models` | 200 |
| `GET /api/option/` (admin PAT from the candidate DB copy) | 200 |
| `GET /api/option/request_policy` (admin PAT) | 200 |

Served frontend: desktop entry chunk `static/js/index.01264a5b9a.js` (sha256 `b5f7c53146d5cfa958f2c7ee700a6f66312d8a4466bad4b48a5542d76b57e00c`)
contains `gjson_fingerprint` twice. The mobile build has no rule editor at all
(`web/src/mobile/features/routing` is a read-only routing/bindings view), so the mobile bundle is expected not
to contain the new key source.

Full smoke (standard flow; it does issue small real upstream calls with the copied production keys):

```
VIRTUAL_POOL_SMOKE_ISOLATED=1 SMOKE_MODEL=deepseek-v4.1-flash scripts/smoke_release.sh http://127.0.0.1:4003 <candidate-db> full
```

Result: `smoke full ok: http://127.0.0.1:4003 model=deepseek-v4.1-flash`, exit 0.

Patch and option-override checks on the host checkout and the candidate runtime:

- `patches/patch-image-gen-filter.py` present; `filterImageGenerationTool` present in `relay/responses_handler.go` (3 occurrences)
- `relay/helper/stream_scanner_timeout.go` present with `NewStreamScannerWithIdleTimeout` (2 occurrences)
- `patches/local-option-overrides.json` sha256 `cc1e8fffa62bb9b7ca3d3af13b972df6522c6e1098635c790dc9d25cca8683b0` equals manifest `LOCAL_OPTION_OVERRIDES_SHA256`; 69 `ModelRatio`, 69 `CompletionRatio`, 69 `CacheRatio` entries, including `deepseek-v4.1-flash` 0.15 / 4.0 / 0.02
- candidate DB `ModelRatio` carries `deepseek-v4.1-flash = 0.15`

### Stickiness rehearsal on the isolated candidate DB

The rehearsed production option change was applied only to the candidate's own database copy through
`PUT /api/option/` (200), and read back through `GET /api/option/` (the third rule then listed ten sources).
Three DSH-shaped `POST /v1/chat/completions` requests were sent with `max_tokens=1` and
`user-agent: deepseek-harness/0.2.0-rc.2`, then the candidate log rows and the live bindings were read.

| request | first user message | log id | channel | affinity key fingerprint | key source / path |
| --- | --- | --- | --- | --- | --- |
| A1 turn 1 | `rehearsal session alpha` | 797970 | 9 | `ea732767` | `gjson_fingerprint` / `messages.#(role=="user").content` |
| A2 turn 2, longer history | `rehearsal session alpha` | 797971 | 9 | `ea732767` | `gjson_fingerprint` / `messages.#(role=="user").content` |
| B1 other session | `rehearsal session beta` | 797972 | 9 | `0d16eae1` | `gjson_fingerprint` / `messages.#(role=="user").content` |

`GET /api/log/channel_affinity_bindings` reported exactly two entries for the rule
"deepseek glm session stickiness", model `deepseek-v4.1-flash`, group `svip`:

- key hint `c26507b5`, fingerprint `ea732767` -> channel 9
- key hint `74551dae`, fingerprint `0d16eae1` -> channel 9

`sha1("rehearsal session alpha")[:8] = c26507b5` and `sha1("rehearsal session beta")[:8] = 74551dae`, which
confirms that both turns of one session produced the same affinity key while the other session produced a
different one, and that only the digest, never the message text, is stored.

All three requests were served by channel 9 because balanced placement minimizes live sessions per unit of
weight and channel 9 carries weight 50 against 5 (channel 47) and 10 (channel 46). Stickiness therefore shows
up as a stable key and a stable binding per session, not as a spread of sessions across channels. A session can
still be placed on channel 46 or 47 when channel 9 is loaded.

No production write was performed. Every write in this section was confined to the isolated candidate database copy.

## Cutover (requires explicit operator confirmation in the current thread)

```
cd /opt/new-api && scripts/cutover_release.sh 2026-10-09-dsh-session-fingerprint-rc01
```

The script backs up the live binary and database into the release `runtime` directory, installs the candidate
binary as `/opt/new-api/new-api`, restarts `new-api.service`, waits for `/api/status`, and runs the fast smoke.
On failure it restores the previous binary and, when the schema changed, the previous database.

After cutover, and only then, apply the production option change through the admin API (`PUT /api/option/`,
`RootAuth`) and read it back:

- option key `channel_affinity_setting.rules`
- append `{"type":"gjson_fingerprint","path":"messages.#(role==\"user\").content"}` to the key sources of the rule "deepseek glm session stickiness"
- the running pre-cutover binary does not understand the new type, so the write must follow the cutover

## Rollback plan

```
cd /opt/new-api && scripts/rollback_release.sh 2026-10-09-dsh-session-fingerprint-rc01
```

The rollback reads `releases/2026-10-09-dsh-session-fingerprint-rc01/runtime/cutover-backup.env`, which records
the previous live binary sha256 (`8aca07135ff8f45e1e80708d63116efc9750eb96326360268e1f70bd445cf0d7`, the rc02
binary), the backup binary and database paths, and the pre-cutover schema hash. The extra
`gjson_fingerprint` source is inert for the rolled-back binary (unknown source types are skipped), so the option
change does not need to be reverted for a rollback to work; it can be removed later through the same API.

## Next safe action

Wait for explicit operator confirmation of this exact release identity, then run the cutover, the production
option write, production verification, finalize, and local cleanup (`scripts/cleanup_local_release.ps1`).
