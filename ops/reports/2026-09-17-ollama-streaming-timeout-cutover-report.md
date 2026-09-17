# 2026-09-17 Ollama Streaming Idle Timeout Production Cutover

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api`.

Release: `2026-09-17-rc01` at `prod/251` `eb974a37915deb48ad9207cf79449c99ea0a61b9`.

Action class: `prepare` + candidate `verify` + explicit production `cutover`.

## Change

- Added a line-oriented idle watchdog for Ollama chat/generate and Responses streaming paths.
- The watchdog times out only while waiting for the next complete upstream line; receiving a line resets the idle window.
- On idle timeout, the upstream body is closed, the blocked scan is released, and the relay returns `504 Gateway Timeout` without emitting a synthetic `[DONE]` or `response.completed`.
- The implementation reuses `constant.StreamingTimeout`, so `STREAMING_TIMEOUT=180` applies to the Ollama paths and remains a global NewAPI streaming idle-timeout setting.
- `RELAY_TIMEOUT=900` remains the independent upstream request-total timeout.

## Source And Tests

- Commit: `eb974a37915deb48ad9207cf79449c99ea0a61b9`.
- Focused tests: `go test ./relay/helper ./relay/channel/ollama -count=1`.
- Race checks: `go test -race ./relay/helper ./relay/channel/ollama -run 'IdleTimeoutScanner|IdleTimeout' -count=5`.
- Static check: `go vet ./relay/helper ./relay/channel/ollama`.
- Branch was pushed to `origin/prod/251`; production `/opt/new-api` checkout was fast-forwarded to the deployed commit without a rebuild or restart.

## Candidate Build And Isolation

- Built locally for `linux/amd64` with the project-private Go/Bun toolchain and `scripts/build_release_candidate_local.ps1`.
- Candidate binary sha256: `20831fccd43ab9b63c9bbf0b5bc2b03c65a85ed5b57bb482c09290d9f27eb10b`.
- Previous production binary sha256: `58dc6a4b99386123803637a8bc88e3ee1f94d04672f84d8de3e704e4ef9082b7`.
- Candidate manifest and binary were uploaded to `/opt/new-api/releases/2026-09-17-rc01/`; remote hash matched the manifest.
- Candidate PID `184800` owned port `4003`; its runtime used `/opt/new-api/releases/2026-09-17-rc01/runtime/new-api.db`, `SQL_DSN=local`, and `NODE_TYPE=slave` with Redis and batch jobs disabled.
- Candidate database schema remained unchanged and `PRAGMA integrity_check` returned `ok`.
- Candidate full smoke passed with `deepseek-v4-flash`, the fixed `gpt-oss:20b` Ollama sample returned HTTP 200, authenticated `GET /api/option/` returned 200, and production port `4002` remained active throughout rehearsal.

## Cutover And Production Verification

- `scripts/cutover_release.sh 2026-09-17-rc01` completed successfully.
- Live `/opt/new-api/new-api` sha256 matches the manifest and candidate binary exactly.
- `new-api.service` is active on port `4002`; final PID after the timeout configuration restart is `195031`.
- Production full smoke passed: `smoke full ok: http://127.0.0.1:4002 model=deepseek-v4-flash`.
- Production `gpt-oss:20b` chat completion returned HTTP 200.
- Production Ollama streaming returned HTTP 200 and completed with `data: [DONE]`; no idle-timeout errors were observed in the post-cutover logs.
- Production SQLite `PRAGMA integrity_check` returned `ok`.

## Timeout Configuration

- `/opt/new-api/.env` now contains `STREAMING_TIMEOUT=180` and `RELAY_TIMEOUT=900`.
- Pre-change env backup: `/opt/new-api/releases/2026-09-17-rc01/runtime/live.env.pre-timeout.20260917-023713.bak`.
- The running process environment confirms `STREAMING_TIMEOUT=180` and `RELAY_TIMEOUT=900`.

## Rollback And Cleanup

- Rollback handle: `/opt/new-api/releases/2026-09-17-rc01/runtime/cutover-backup.env`.
- Previous binary and pre-cutover database backups remain under the same release runtime directory.
- Candidate `4003` remains running as an additional observation-period safeguard.
- Local candidate build directory was removed with `scripts/cleanup_local_release.ps1 -ReleaseId 2026-09-17-rc01`; reusable local build caches were retained.

## Next Safe Action

- After the production release is confirmed stable, finalize release `2026-09-17-rc01` to stop the `4003` candidate and remove transient candidate runtime files while preserving the release binary, manifest, and cutover rollback metadata.
