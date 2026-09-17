# 2026-09-17 Virtual Model Route Aggregation (Rotation, Health, Channel-Derived Pools) Cutover

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api`.

Release: `2026-09-17-rc02` at `prod/251` `61f569e982d029d68b9c77f237254e3de07e39c7`.

Action class: `prepare` + candidate `verify` + explicit production `cutover`.

## Change

Virtual model routes (`model_retry_policy_setting.virtual_model_routes`) gained three abilities, all backward compatible:

- `rotation`: `ordered` (default) / `random` / `round_robin` chooses the first pool member per request instead of always starting at the pool head.
- `health` (optional): a pool member that fails with `429`, `404`, or `>=500` is moved behind the healthy members for `cooldown_seconds` (doubling up to `max_cooldown_seconds`) and returns after a success. The signal comes from relayed traffic only: no probe, no new table, process-local state.
- `sources` (optional): pool members are derived from a channel's `models` list, so an aggregate model follows the channel instead of a hand-kept target list. The channel stays the single place to add or remove models.
- `max_attempts` bounds how many pool members one request may try.
- Configured virtual model names are now included in the model list (`GetGroupsEnabledModels`), so an aggregate model needs no channel `models` entry and no `model_mapping` fallback to be visible.

Production state after cutover adds one route:

```json
"auto-free": {
  "rotation": "round_robin",
  "max_attempts": 3,
  "health": { "enabled": true, "failure_threshold": 2, "cooldown_seconds": 60, "max_cooldown_seconds": 600 },
  "sources": [ { "channel_id": 5 }, { "channel_id": 6 } ]
}
```

`auto-free` therefore aggregates every model of channel `5` (OpenRouter Free) and channel `6` (NVIDIA Free); maintaining those two channel model lists is the only upkeep. Design notes: `docs/virtual-model-route-design.md`.

## Source And Tests

- Commits: `583c0f2e5` (rotation + max_attempts), `da6278252` (health-aware ordering), `523345f51` (channel-derived pools + visibility), merged as `61f569e98`.
- Focused tests: `go test ./setting/... ./service/ ./controller/... ./middleware/ ./model/ -count=1`.
- New tests cover round-robin rotation, random spread, attempt limits, legacy array and object parsing, option validation, cooldown ordering with a cooling member kept as last resort, cooldown recovery on success, client errors ignored, channel-derived pools following a channel model edit, source-channel scoping, and virtual model visibility.
- Static checks: `go vet ./service/ ./controller/ ./setting/operation_setting/ ./model/`; `go build ./...` shows no new failures.

## Configuration Write Ordering

The deployed binary before this release only accepted the legacy `"name": [ ... ]` route form, so writing the object form first would have failed option validation. The option write therefore followed the cutover; the value was written through `PUT /api/option/` with the root token used in place on the host, and the previous value was saved to `/opt/new-api/releases/2026-09-17-rc02/runtime/option-before-virtual-routes.json` as the option rollback artifact. `auto-subagent` and `auto-subagent-codex` were re-read after the write and were unchanged.

## Candidate Build And Isolation

- Built locally for `linux/amd64` with the project-private Go/Bun toolchain and `scripts/build_release_candidate_local.ps1`; frontend release cache hit.
- Candidate binary sha256: `db5fcd7e61a837e6e943e6851a81b3aefb223576130fe87716ffdc0ee5d09b9f`.
- Previous production binary sha256: `20831fccd43ab9b63c9bbf0b5bc2b03c65a85ed5b57bb482c09290d9f27eb10b`.
- Uploaded binary and manifest to `/opt/new-api/releases/2026-09-17-rc02/`; remote hash matched the manifest.
- Candidate PID `1188682` owned port `4003`; its runtime used `/opt/new-api/releases/2026-09-17-rc02/runtime/new-api.db` with a copied environment, and production port `4002` (PID `195031`) stayed active throughout rehearsal.
- Candidate schema was unchanged against the staged baseline and `PRAGMA integrity_check` returned `ok`.

## Candidate Verification

- `smoke_release.sh` full smoke passed; the selected model was `auto-subagent`, so the smoke also exercised the existing legacy-form virtual route on the new binary.
- Fixed channel samples returned HTTP 200: `deepseek-v4-flash` (routed upstream as `deepseek-v4-flash:0731`) and `gpt-oss:20b`.
- Authenticated `GET /api/option/` returned HTTP 200; the settings surface is healthy.
- Sandbox feature rehearsal (writes limited to the candidate's copied database): the `auto-free` route was written through the candidate's own `PUT /api/option/`, which proved the new validation and parsing path; `GET /v1/models` then listed `auto-free`; five `auto-free` chat completions returned HTTP 200 with different upstream models each time (`cohere/north-mini-code:free`, `nvidia/nemotron-3-ultra-550b-a55b:free`, `nvidia/nemotron-3.5-lightning:free`, `openrouter/free`), one request hit an upstream `429` and completed by switching to the next pool member in the same request.
- With `sources` temporarily narrowed to `[{ "channel_id": 6 }]` on the candidate, `auto-free` served channel 6 models (`nvidia/nemotron-3-super-120b-a12b`, `nvidia/nemotron-3-ultra-550b-a55b`), proving that both source channels feed the pool. The candidate route was restored to `sources: [5, 6]` afterwards.

## Cutover And Production Verification

- `cutover_release.sh 2026-09-17-rc02` completed successfully; the post-cutover fast smoke passed.
- Live `/opt/new-api/new-api` sha256 matches the manifest and the candidate binary exactly.
- `new-api.service` is active on port `4002` (PID `1191665`); production `PRAGMA integrity_check` returned `ok`.
- Production full smoke passed (`model=auto-subagent`).
- Production `GET /v1/models` lists `auto-free` (55 models total).
- Four production `auto-free` chat completions returned HTTP 200 with rotating upstream models (`cohere/north-mini-code:free`, `nvidia/nemotron-3-ultra-550b-a55b:free`, `poolside/laguna-s-2.1:free`, `nvidia/nemotron-3.5-lightning:free`); all four consume logs recorded `use_channel: ["5"]`, matching the pool order (channel 5 members first).
- One production `auto-subagent` chat completion returned HTTP 200 (`gpt-5.6-luna`), confirming existing routes are unaffected.

## Rollback And Cleanup

- Rollback handle: `/opt/new-api/releases/2026-09-17-rc02/runtime/cutover-backup.env` (previous binary and pre-cutover database backups are recorded there).
- Option rollback artifact: `/opt/new-api/releases/2026-09-17-rc02/runtime/option-before-virtual-routes.json`; replay it through `PUT /api/option/` to remove `auto-free`.
- Removing the `auto-free` key alone is enough to disable the aggregate model; the new binary keeps the legacy route behavior for the existing routes.
- Local release directory was removed with `scripts/cleanup_local_release.ps1 -ReleaseId 2026-09-17-rc02`; reusable local build caches were retained. Candidate `4003` remains running as an observation-period safeguard.

## Next Safe Action

- After the release is confirmed stable, finalize release `2026-09-17-rc02` to stop the `4003` candidate and remove transient candidate runtime files while preserving the binary, manifest, and cutover rollback metadata.
- Optional follow-ups: give `auto-free` an explicit `ModelRatio` entry, keep the free channel catalogs reviewed periodically (the pool follows the channel lists automatically), and consider `hidden` for internal-only virtual models.
