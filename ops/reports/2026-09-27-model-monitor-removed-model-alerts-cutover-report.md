# 2026-09-27 Model Monitor: Stop Alerts For Removed Models Cutover

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api`.

Release: `2026-09-27-rc01` at `prod/251` `054c5b32aebacbbc8bed83052c2c9b9e587494c0`.

Action class: `prepare` + candidate `verify` + explicit production `cutover`.

## Change

Model monitor alerts (Telegram repeat notifications and email/Telegram delivery) were still being produced for paths whose channel had already removed the model from its `models` list. A path could stay `unavailable` in `model_monitor_path_states` long after the model stopped being routed, so repeats kept firing for a model the channel no longer served.

`model.IsEnabledModelMonitorPath(siteID, targetID, channelID, modelName)` now re-checks the live channel `models`/`model_mapping` through `Channel.SupportsModel` and returns false when the channel no longer serves the model. It is applied in three places:

- `QueueDueModelMonitorTelegramRepeats` skips ineligible paths instead of queuing repeats.
- `HasDueModelMonitorTelegramRepeat` skips them when deciding whether any repeat is due.
- `IsCurrentModelMonitorUnavailableTransition` returns false for removed models, and the new `IsCurrentModelMonitorAlert` wraps it so `deliverModelMonitorAlert` drops queued `unavailable` events for removed models.

The change is backward compatible: recovery/available events are unaffected (`IsCurrentModelMonitorAlert` returns true for any non-`unavailable` status), and eligible unavailable paths behave exactly as before.

## Source And Tests

- Commit: `054c5b32a` (`fix(model-monitor): stop alerts for removed models`).
- Focused tests: `go test ./model/ ./service/ -run 'ModelMonitor' -count=1` -> `ok` for both packages.
- New tests cover: Telegram repeats stopping after a channel drops the model (`TestModelMonitorTelegramRepeatsStopWhenChannelNoLongerSupportsModel`), and alert dispatch skipping a queued `unavailable` event once the model is removed (`TestDispatchModelMonitorAlertsSkipsUnavailableAfterModelRemoval`).

## Candidate Build And Isolation

- Built locally for `linux/amd64` with the project-private Go/Bun toolchain and `scripts/build_release_candidate_local.ps1`; frontend release cache hit (`FRONTEND_CACHE_HIT=1`).
- Candidate binary sha256: `b1654690b0190976c99d9df36e6b0ea030a87690c80be8719034af80ab76f32f`.
- Previous production binary sha256: `db5fcd7e61a837e6e943e6851a81b3aefb223576130fe87716ffdc0ee5d09b9f` (release `2026-09-17-rc02`).
- Uploaded binary and manifest to `/opt/new-api/releases/2026-09-27-rc01/`; remote hash matched the manifest.
- The stale `2026-09-17-rc02` `4003` candidate from the previous release was still running; it was finalized first (`finalize_release.sh 2026-09-17-rc02`) because its binary matched the live binary, which freed port `4003`.
- Candidate PID `1902233` owned port `4003`; its runtime used `/opt/new-api/releases/2026-09-27-rc01/runtime/new-api.db` with a copied environment, and production port `4002` stayed active throughout rehearsal.
- Candidate schema was unchanged against the staged baseline (`9008e8403a7a6eebc337c33079734c30b92a3f76da681c09967732d8fd9fee6c`) and `PRAGMA integrity_check` returned `ok`.

## Candidate Verification

- `smoke_release.sh ... full` passed with `model=glm-5.3-flash` on port `4003`.
- Candidate `/api/status` reported version `054c5b32aebacbbc8bed83052c2c9b9e587494c0`, confirming the new binary was staged.
- The default smoke model (`auto-subagent`, the first id in `/v1/models`) returned `503 model_not_found` on both the candidate and production; this is a pre-existing routing/data condition on production, not a binary regression, so the smoke was re-run with an explicit healthy model.
- Authenticated model monitor endpoints (`/api/model-monitor/summary`, `/api/model-monitor/sites/5`) returned HTTP 200.

## Cutover And Production Verification

- `cutover_release.sh 2026-09-27-rc01` completed successfully; the post-cutover fast smoke passed.
- Live `/opt/new-api/new-api` sha256 matches the manifest and the candidate binary exactly (`b1654690b0190976c99d9df36e6b0ea030a87690c80be8719034af80ab76f32f`).
- `new-api.service` is active on port `4002` (PID `1904877`); production `PRAGMA integrity_check` returned `ok`.
- Production full smoke passed (`model=glm-5.3-flash`).
- Production `/api/status` reports version `054c5b32aebacbbc8bed83052c2c9b9e587494c0`.
- No model monitor delivery errors appeared in the service journal after cutover.
- The fix targets a real production state: `15` paths are `unavailable` in `model_monitor_path_states` while their channel no longer lists the model, so the old binary would have kept producing repeats for them.

## Rollback And Cleanup

- Rollback handle: `/opt/new-api/releases/2026-09-27-rc01/runtime/cutover-backup.env` (previous binary `db5fcd7e...` and the pre-cutover database backup are recorded there).
- No option or schema migration is involved; rolling back only requires restoring the previous binary (and the pre-cutover database only if the new binary had written incompatible state).
- Candidate `4003` (PID `1902233`) remains running as an observation-period safeguard.
- Local release directory to be removed with `scripts/cleanup_local_release.ps1 -ReleaseId 2026-09-27-rc01`; reusable local build caches are retained.

## Companion Ops Change

The `newapi-251` relay watchdog (`n8n-homelab` `agents/newapi-251/newapi-watchdog.sh`) was also corrected in the same window: it now restarts new-api only on process-level failures (liveness `/api/status` non-200, or relay transport failure `000`) and only alerts on relay application errors such as `503 model_not_found`. The probe model moved from the single-channel `deepseek-v4-flash` to the multi-channel `glm-5.3-flash`, and `CHECK_INTERVAL_SECONDS` was tightened to `600` (with `FAIL_THRESHOLD=2`, a real hang is repaired within ~20 minutes). The retired `deepseek-v4-flash` mapping was removed from channel `46`.

## Companion Data Change: auto-subagent Retirement

The virtual route `auto-subagent` was found broken during this release (all three pool members had no channel in group `svip`: `gpt-5.6-luna`, `grok-4.5`, and the retired `deepseek-v4-flash`), which also made it the first model the smoke picked. It is unused (no consume logs in 7 days), so it was retired through the admin API:

- removed the `auto-subagent` key from `model_retry_policy_setting.virtual_model_routes`;
- removed `auto-subagent` from the `models` and `model_mapping` of channels `21`, `36` (active) and `37` (disabled), which were legacy entries predating automatic virtual-model visibility;
- removed `auto-subagent` from `model_retry_policy_setting.single_pass_priority_models`.

Verification: `abilities` for the exact model `auto-subagent` is now `0`, `/v1/models` no longer lists it, and `auto-subagent-codex` (routed via `gpt-5.6-terra`) and `auto-free` still return HTTP 200.

Rollback artifacts (pre-change values) are preserved under `/opt/new-api/releases/2026-09-27-rc01/runtime/`: `option-before-virtual-routes.json`, `option-before-single-pass.json`, `channels-before-auto-subagent-removal.txt`.

## Next Safe Action

- Release `2026-09-27-rc01` is finalized: the `4003` candidate is stopped and transient candidate runtime files are removed, while the binary, manifest, cutover rollback metadata, and the `auto-subagent` rollback artifacts are preserved.
- `prod/251` (`054c5b32a` plus this report) is pushed to `origin`.
- Optional follow-up: review the `15` stale unavailable model monitor paths now that the removed-model alert guard is live.
