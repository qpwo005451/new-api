# 2026-09-29 Model Health Same-Tier Retry Fix Cutover Report

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api` (port 4002).

Release: `2026-09-29-model-health-rc01` at `prod/251` `cca4fdc4c7020cea855a9f4f9f7f5d46056228b9`.

Action class: `prepare` + candidate `verify` (smoke + e2e) + explicit production `cutover` + `finalize`.

## Change

Ordinary (non-virtual-route) model retries walked priority tiers instead of
candidates: the attempt following a failure skipped the healthy same-priority
sibling and descended straight to the next priority level. Virtual pool routes
walk candidates and were never affected.

`RetryParam` now records attempted channel ids (`MarkAttemptedChannel`, called
next to `addUsedChannel` in the relay retry loop), and
`selectSatisfiedChannelWithModelHealth` returns the first untried channel from
the priority-ordered candidate list for policy models before falling back to
weighted random tier selection.

Commits in this release window (prod/251):

- `ab68ad24d` feat(router): add model-level channel cooldown and admin UI
- `b6803bba3` test(router): verify ordinary model-level channel cooldown on 4003
- `cca4fdc4c` fix(router): prefer fresh same-tier channels on policy model retries

## Source And Tests

- Focused tests: `go test ./service ./controller ./model -count=1` -> `ok`.
- New regression tests in `service/model_health_retry_test.go` cover: same-tier
  failover to the healthy sibling before leaving the tier, and fall-through to
  the next tier once the whole tier is cooling.
- `relaykit` module independence verified (`GOWORK=off go build ./...`).

## Candidate Build And Isolation

- Built locally with `scripts/build_release_candidate_local.ps1`
  (`-ReleaseId 2026-09-29-model-health-rc01 -ReleaseTag cca4fdc4c`); frontend
  cache hit (`FRONTEND_CACHE_HIT=1`).
- Candidate binary sha256: `522f3b02bf6183b6b64e1c7744e18ef715c043ff41d6607d94d912337b896c08`.
- Previous production binary sha256: `b1654690b0190976c99d9df36e6b0ea030a87690c80be8719034af80ab76f32f`
  (release `2026-09-27-rc01`, commit `054c5b32a`).
- Candidate staged on port 4003 via `stage_release_runtime.sh` with an isolated
  DB copy; the ad-hoc 4003 candidate (`2026-09-28-model-health-4003`) was
  stopped manually before staging (its binary was a local dev build whose hash
  did not match the live production binary, so `finalize_release.sh` could not
  be used on it; its runtime dir remains for forensics).

## Candidate Verification

- `smoke_release.sh ... full` passed with `model=glm-5.3-flash`.
- E2E harness `scripts/testing/run_model_health_4003.py` passed 5/5
  (H01 policy save, H02 policy read, H03 same-tier cooldown skip, H04 other
  model independence, H05 recovery after cooldown expiry) against the staged RC.

## Cutover And Production Verification

- `cutover_release.sh 2026-09-29-model-health-rc01` completed successfully with
  automatic backups; rollback handle:
  `/opt/new-api-release-runner/releases/2026-09-29-model-health-rc01/runtime/cutover-backup.env`
  (previous binary `b1654690...`, DB snapshot included).
- Live `/opt/new-api/new-api` sha256 matches the candidate exactly
  (`522f3b02...`); `new-api.service` active, PID 85649.
- Production `PRAGMA integrity_check` returned `ok`.
- Production full smoke passed (`model=glm-5.3-flash`); virtual routes
  `auto-subagent-codex` and `auto-free` verified 200 post-cutover.
- `finalize_release.sh` stopped the 4003 candidate and finalized the release.

## Harness Hardening Notes (for future runs)

- The e2e harness now disables `model_monitor_setting.*` probes while running
  (they relay real traffic to the mock and pollute call/health counters).
- Cooldown-aware waits replaced fixed iteration loops; cooldown windows
  overlapped fixed loops and made assertions flaky.
- Policy settings restore to a clean baseline (`false`/`[]`) instead of the
  pre-run snapshot, so aborted runs no longer leak stale 300s policies into
  later runs.
- Manual experiment channels must be deleted; leftover mock channels once
  joined the candidate pool and skewed every counter.
