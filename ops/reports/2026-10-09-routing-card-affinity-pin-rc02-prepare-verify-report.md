# Routing-card polish + affinity tier-pin fix: release prepare and verify report

Release: `2026-10-09-routing-card-affinity-pin-rc02`
Audit, review and verification: 2026-10-09 10:20-11:05 CST

## What ships

Two reviewed branches, merged into `prod/251` together because the user asked for
one deployment for both.

| branch | commits | scope |
| --- | --- | --- |
| `codex/routing-card-polish` | `e18973534`, `37bd7d389`, `7aac42e04` | routing allocation card redesign, global number-stepper suppression, share bar drawn with the shared `Progress` component, regression tests |
| `codex/fix-affinity-tier-pin` | `ba9f842e5`, `93531cfc6` | a sticky session follows a routing weight or priority edit, candidate-lookup failures are reported, `affinity_pin_not_preferred` policy event and its seven locale labels |

Merge: `git merge --ff-only codex/routing-card-polish` then
`git merge --no-ff codex/fix-affinity-tier-pin` (merge commit `36027f6aa`), then
`63f721af7` records this audit. Both branch histories are preserved, so the
reviewed commit ids are unchanged.

## Audit

Independent reviewers, one per axis, on different model families from the
coordinator (codex + glm-5.3-flash, effort max), plus the coordinator's own pass.

Round 1 (`37bd7d389` / `ba9f842e5`):

- Spec: all four requirements implemented. Gaps: auto-group pins stay stale
  (the candidate set cannot be scoped for `usingGroup == "auto"`), a
  candidate-lookup failure fails open without a log line, strict-mode pins are
  retained against the literal requirement.
- Standards: two hard findings (the share bar hand-rolled markup and clamping that
  `@/components/ui/progress` already provides; no regression test for the stepper
  CSS or the new bar) and three judgement calls (the swallowed lookup error, a
  duplicated test fixture, a stale test name).

Fixes, then round 2 on `7aac42e04` / `93531cfc6`:

- `7aac42e04`: the bar is the shared `Progress` component; the card test asserts
  each row's bar value; a new stylesheet test asserts the number-stepper rules;
  the stale test name is corrected.
- `93531cfc6`: the lookup failure is reported with `common.SysError` before the
  pin is kept, fail-open unchanged; the session fixture is one helper.

Round 2 verdicts: Standards - all five fixes PASS, "no remaining hard violation or
standards findings". Spec - requirements 1-3 verified, both deliberate responses
accepted, only the merge itself outstanding. Standards also confirmed the
`format:check` drift is pre-existing and repo-wide (38 files at `93531cfc6`; both
routing-card files already drifted at `ec4fba9c2`, while the new stylesheet test
and CSS are clean).

Coordinator pass, findings recorded rather than silently accepted:

- `"strict"` is a raw literal here, matching the two existing call sites and the
  policy validation; there is no `SessionModeStrict` constant.
- `Action: "rebind"` needs no whitelist: the frontend labels the decision
  *reason*, and `affinity_pin_not_preferred` has its seven locale entries.
- The share bar is `aria-hidden`, so an unclamped user-typed value never reaches
  the accessibility tree; the percentage field carries the value.
- Deliberate behavior change: when the whole preferred tier is cooling, placement
  no longer drops to the next tier; the normal selection path picks the channel
  and binds it, and the new pin check moves the session back once the top tier
  recovers.
- Auto-group and strict-mode retention are documented limitations, not defects:
  this deployment has no `auto` group and no strict rule.

## Local verification of the merged tree

- `go test ./service/... ./middleware/... ./controller/... ./setting/... ./model/... -count=1`: pass. One run hit `TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner` in `controller`; that package passes 5/5 alone and passes again as a whole, and this merge changes no `controller/` file, so it is a load-dependent flake.
- `cd web && bun run typecheck`: pass. `vitest run src/features/system-settings src/styles`: 19 files / 221 cases pass.

## Candidate build

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 scripts/build_release_candidate.sh 2026-10-09-routing-card-affinity-pin-rc02 63f721af73ff00ea955dedd01faae047dde94daa
```

- exit 0. The first attempt was built without `CGO_ENABLED=0` and came out
  dynamically linked; the deployed artifact is statically linked, so that attempt
  was discarded and the release rebuilt (the production host runs Debian 12
  glibc 2.36, older than the build host).
- `BINARY_SHA256=bc476833d0ac54322ea8c8036930dddd51219da5827cb72769e829a4d5c136de`, size 140050592, statically linked and stripped.
- `RELEASE_COMMIT=63f721af73ff00ea955dedd01faae047dde94daa`, `FRONTEND_CACHE_HIT=1` (frontend tree `8466bc6bfb1e45898e98e163c6560452e2bf6c34`, the same tree the discarded attempt built and verified), `BUN_VERSION=1.4.2`.
- `LOCAL_OPTION_OVERRIDES_SHA256=cc1e8fffa62bb9b7ca3d3af13b972df6522c6e1098635c790dc9d25cca8683b0` (unchanged from the previous release).
- Embedded content: `affinity_pin_not_preferred` x2, `gjson_fingerprint` x2 (the previous fix is still in), `webkit-inner-spin-button` and `appearance:textfield` (the stepper fix), desktop entry `static/js/index.c0742d2ef3.js` and CSS `static/css/index.d77c1cba6e.css`, mobile `index.c0a26bbaa1.js`.

Uploaded to `/opt/new-api/releases/2026-10-09-routing-card-affinity-pin-rc02/`; remote `bin/new-api` sha256 `bc476833...` and `manifest.env` sha256 `224a22f00e97a672e08d0c6e9c44304cf198e7d07bbda46a25e6f5f04565cdbf` both equal the local files.

## Candidate verification (isolated, port 4003)

Staging: `VIRTUAL_POOL_STAGING_ISOLATED=1 scripts/stage_release_runtime.sh 2026-10-09-routing-card-affinity-pin-rc02`.

- candidate PID 739649 owns 4003, executable is the release binary, `SQLITE_PATH` points at the release runtime copy, no production database reference.
- schema hash identical before and after startup (`567bf9ab4fcabad03c19c48459e9c0544b28762c2d338601a4737b87481c0914`), no `schema-changed.flag`.
- production untouched throughout: `new-api.service` active, MainPID 572151, `NRestarts=0`, `/api/status` 200, live binary still `201164c5325430b5274d0c1752741fc6d8cb060ef8bc07ac319465a252ed611b`.

Checks:

- `smoke_release.sh ... fast` - `smoke fast ok`.
- `smoke_release.sh ... full` - `smoke full ok` with `SMOKE_MODEL=glm-5.3-flash`. With the default model the chat check returns 502 because the `input.codes` upstream is currently returning Cloudflare 502s; production logs show the same 502s for `deepseek-v4.1-flash` on channels 9 and 36 in the same window, so this is an upstream outage, not a candidate defect.

### Sticky session follows a policy edit (the reported bug)

`/tmp/verify_affinity_pin.sh <release-id> [port]`, sha256 `31486c7eb25f79fed958d3c3ce457cff20a93978ff104ad14d3ab0428837adec`, model `glm-5.3-flash`, one session key, two requests with a weights edit in between.

| binary | step 1 (channel 46 preferred) | step 2 (46 demoted to 400, 9 promoted to 500) |
| --- | --- | --- |
| deployed `201164c5...` (staged from `releases/2026-10-09-dsh-session-fingerprint-rc01` as a throwaway release, now removed) | served by 46, binding `0ca776cd -> 46` | **stayed on 46**, binding unchanged - bug reproduced |
| candidate `bc476833...` | served by 46, binding `7e32d1cf -> 46` | **moved to 9**, binding `7e32d1cf -> 9` - fixed |

The candidate run was repeated on the freshly staged copy and passed 4/4 both times. The throwaway release directory was deleted and the candidate re-staged afterwards.

## Cutover plan

1. `cd /opt/new-api && scripts/cutover_release.sh 2026-10-09-routing-card-affinity-pin-rc02` (needs the user's explicit approval for this release id in this thread).
2. Confirm `smoke fast ok`, live `sha256sum /opt/new-api/new-api` equals `bc476833...`, `NRestarts=0`, and the pre-cutover binary/database backup in `runtime/cutover-backup.env`.
3. Rollback handle: `cd /opt/new-api && scripts/rollback_release.sh 2026-10-09-routing-card-affinity-pin-rc02` restores the `201164c5...` binary and its database backup.
4. `scripts/finalize_release.sh 2026-10-09-routing-card-affinity-pin-rc02` after stability is confirmed, then the local cleanup (local release directory, caches, `node_modules`, `dist`; `.local-tools/release-cache` is kept).
5. Push `prod/251` (currently `63f721af7` plus this report) to `origin/prod/251` after the production check.

## Notes for the user

- The card change and the pin fix are independent; the pin fix is what stops a session from staying on a channel the routing policy demoted.
- `glm-5.3-flash` still prefers `ollama_pro` (channel 46) because that channel carries priority 500 for it against 498 for `input-0.1X`. That is configuration, not code: raise channel 9 above 500 for glm, or set channel 46's glm weight to 0, or remove glm from the ollama channel.

## Cutover and finalization (2026-10-09 11:07 CST)

The user approved the cutover for this exact release id in-thread, so
`cd /opt/new-api && scripts/cutover_release.sh 2026-10-09-routing-card-affinity-pin-rc02`
was run. Exit 0, `smoke fast ok: http://127.0.0.1:4002`.

- live binary `sha256sum /opt/new-api/new-api` = `bc476833d0ac54322ea8c8036930dddd51219da5827cb72769e829a4d5c136de`, equal to the manifest.
- `new-api.service` active, new MainPID 743275, `NRestarts=0`, started 2026-10-09 11:07:28 CST; `/api/status` and `/` both 200.
- the served desktop entry chunk is `static/js/index.c0742d2ef3.js`, the new one, so the card redesign and the number-stepper fix are live.
- rollback handles recorded in `runtime/cutover-backup.env`: `PREVIOUS_BINARY_SHA256=201164c5325430b5274d0c1752741fc6d8cb060ef8bc07ac319465a252ed611b`, `BACKUP_BIN=runtime/live-new-api.20261009-030713.bak`, `BACKUP_DB=runtime/live-new-api.db.20261009-030713.bak`, `LIVE_SCHEMA_SHA256=567bf9ab4fcabad03c19c48459e9c0544b28762c2d338601a4737b87481c0914`.

Production behavior after cutover, read-only:

- sticky sessions still bind: over a ten minute window, 11 sessions carried a `channel_affinity` fingerprint and 8 of them used exactly one channel. The three that span channels are upstream failover inside a single request, visible in the recorded decision flow (attempt 1 channel 9, 2 channel 36, 3 channel 21, 4 channel 46, `retry_status_matched` between attempts), not a stale pin. No `affinity_pin_not_preferred` event appeared, which is correct because no policy edit happened in that window.
- both key sources are in use: `gjson_fingerprint` on `/v1/chat/completions` and `request_header` (`Session-Id`) on `/v1/responses`.
- startup was clean: no migration error, no panic, service did not restart.

Open external issue, not caused by this release: the `input.codes` upstream behind channels 9, 21 and 36 is returning 502 with `upstream TLS handshake failed`, so requests fail over to channel 46 and succeed. The same 502s were present before the cutover, and they are also why the candidate `smoke full` run needed `SMOKE_MODEL=glm-5.3-flash`.

Finalization: `scripts/finalize_release.sh 2026-10-09-routing-card-affinity-pin-rc02` exit 0 - candidate on 4003 stopped and the port released, production still active with `NRestarts=0`, and the candidate binary, `cutover-backup.env`, the previous binary and the previous database backup are preserved for rollback.

Local cleanup: the release directory, detached worktrees, `web/node_modules`, `web/dist`, `web/mobile-dist`, `.gocache`, `.gomodcache` and `.gopath` are removed; `.local-tools/release-cache` is kept.
