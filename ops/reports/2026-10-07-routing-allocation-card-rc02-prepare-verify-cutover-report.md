# 2026-10-07 Per-Model Routing Allocation Card - rc02 Prepare, Verify And Cutover

Release id: `2026-10-07-routing-allocation-card-rc02`
Source commit: `6c2ae0d56330556d695bd88ded0616485687cc4a` (prod/251 tip)
Previous live release: `2026-10-07-routing-allocation-card-rc01` (binary sha256 `7372604e4112ea978aac0835a91bb5195de7155b67290f94e782f1171ff0d57c`)

## Why rc02 exists

rc01 shipped with tests, HTTP probes and served-bundle greps only - no real browser
render check. The operator then opened the live Model Routing page and reported the
per-model allocation card as visually broken ("too ugly"), while confirming that the
functionality itself was fine. No rollback was requested: rc01 stayed live and the
card was redesigned, re-verified and re-released as rc02.

## Change

Presentation-only redesign of the allocation card (`web/src/features/system-settings/models/`):

- compact single-line rows on one shared grid (name / advanced override inputs / share),
- tier band carrying the priority, the role badge and the tier total,
- percent suffix moved inside the share field.

No behavior change: props, PATCH payloads, locking, presets and the Advanced gate are
untouched; no i18n key was renamed and no new copy was introduced.

## Defects measured on the live rc01 page (independent verifier, DOM geometry)

| # | Defect | rc01 baseline | rc02 |
| - | ------ | ------------- | ---- |
| D1 | dead space between channel name and share control | median 971.95 px @1440, 1451.95 px @1920 | 20.92 px |
| D2 | tier header visually weaker than the rows it groups | tier/row visual weight 0.34 | 1.50 |
| D3 | inconsistent row pitch | 40/68/155 px | uniform 41 px within a tier; tier band and card boundary still add height |
| D4 | mobile stacked name and share control | 9/9 rows stacked @390, 60 px rows | 0/9 stacked, 40-41 px rows |
| NR | numeric column alignment (must not regress) | share/priority/weight spread 0 | spread 0 (pass) |
| BQ | behavior equality | - | edit + restore PATCH bodies byte-identical to the rc01 baseline, HTTP 200/200 |

## Verification method

- Reusable Playwright harness `ops/visual/model-weights-verify.cjs` on the verifier
  branch `qpwo005451/ui-render-verify`: real UI login, one browser context,
  screenshots plus DOM measurements and PATCH body capture at 1440x900, 1920x1080 and 390x844.
- The sandbox account used for rendering exists only in the candidate's disposable DB copy.
- The fix worker's own suite: `vitest run src/features/system-settings/models` = 10 files /
  117 tests passed, typecheck exit 0, lint 67 warnings / 182 errors (unchanged baseline).
- The verifier re-ran the harness against a scratch checkout of `6c2ae0d56` and against the
  staged rc02 candidate; both passed.

## Candidate build and staging

- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 scripts/build_release_candidate.sh 2026-10-07-routing-allocation-card-rc02 6c2ae0d56330556d695bd88ded0616485687cc4a`, `FRONTEND_CACHE_HIT=0`, BUILD_EXIT=0.
  The briefed command omitted `CGO_ENABLED`; the first build produced a dynamically linked
  ELF, so the build was redone with `CGO_ENABLED=0` to match the static rc01/production binary.
- Binary sha256 `8aca07135ff8f45e1e80708d63116efc9750eb96326360268e1f70bd445cf0d7`, size 140042400 bytes, statically linked.
- Upload verified: remote sha256 == manifest `BINARY_SHA256`; manifest local/remote sha equal.
- Port 4003 was held by the stale rc01 candidate (PID 1731825); that single PID was killed to
  free the port. The rc01 release directory was not modified.
- Staged candidate PID 1887027 on 4003, candidate DB `.../rc02/runtime/new-api.db`,
  zero references to the production DB, schema identical before/after startup,
  no `schema-changed.flag`.

## Candidate verification (port 4003)

`GET /` 200, `/api/status` 200, Bearer `/v1/models` 200 (62 models), deepseek-baseline
(`deepseek-v4.1-flash`) 200, ollama-baseline (`gpt-oss:20b`) 200, `/v1/responses` 200,
admin `/api/option/` and `/api/option/request_policy` 200 via the copied DB root PAT,
patch/override checks green, `smoke_release.sh full` exit 0, served desktop bundle
`static/js/index.057de9ae8e.js` (sha256 prefix `5754317cce360c947e6b662f`) and mobile bundle
carry the new card, and the real-browser render check passed at all three viewports.

## Production cutover (operator-confirmed, itemized)

Operator confirmed the itemized checklist (action, host checkout ff, binary install and
backups, service restart, timing, rollback path, no finalize) before execution.

- Host checkout `/opt/new-api` fast-forwarded `583e8ddf9` -> `6c2ae0d56330556d695bd88ded0616485687cc4a`;
  `git status --porcelain` still shows only the pre-existing ` M watchdog.sh`
  (sha256 `7af8d72361258e6e979fd24c3ad8bc1180a57c11d0836fda0884d08b877b422e`, unchanged by the merge).
- `scripts/cutover_release.sh 2026-10-07-routing-allocation-card-rc02` exit 0, printed
  `smoke fast ok: http://127.0.0.1:4002`, backup env
  `.../rc02/runtime/cutover-backup.env`.
- Post-cutover proof (independently re-checked by the coordinator): live binary sha256
  `8aca0713...` == manifest; `systemctl is-active new-api` = active with a new
  `ActiveEnterTimestamp Wed 2026-10-07 17:50:35 CST` and new MainPID 1967502 owning 4002;
  `GET /` and `GET /api/status` 200; `journalctl -u new-api -p err --since` = 0 entries;
  `PRAGMA quick_check` = ok; production served bundle `static/js/index.057de9ae8e.js`
  sha256 prefix `5754317cce360c947e6b662f` is byte-identical to the verified candidate bundle;
  sorted schema set of the live DB equals the candidate DB (`14858a8b6cbcffdb3f27922c`, 47 tables);
  4003 candidate untouched (PID 1887027).

## Rollback plan

`cutover-backup.env` records `PREVIOUS_BINARY_SHA256=7372604e...`,
`BACKUP_BIN=.../runtime/live-new-api.20261007-095019.bak`,
`BACKUP_DB=.../runtime/live-new-api.db.20261007-095019.bak` (backup DB `integrity_check` = ok).
Rollback: `scripts/rollback_release.sh 2026-10-07-routing-allocation-card-rc02`.

## Finalize (operator-approved, separate step)

- Preconditions proved read-only: live binary sha256 `8aca0713...` == manifest == candidate bin;
  service active; `GET /` and `/api/status` 200; `journalctl -p err --since 17:50:35` = 0 entries;
  the process owning 4003 (PID 1887027) resolved to the rc02 candidate binary.
- `cd /opt/new-api && timeout 900s scripts/finalize_release.sh 2026-10-07-routing-allocation-card-rc02`
  exit 0: "Release finalized", "Preserved candidate binary", "Preserved rollback metadata".
  `finalized.env` written with `FINALIZED_AT=2026-10-07T18:07:03+08:00`.
- Post-conditions: nothing listens on 4003; the transient candidate files
  (`candidate.env`, `candidate.log`, `candidate.pid`, `live.env`, `new-api.db`,
  `schema-before.sha256`, `schema-after.sha256`, `logs/`) and the detached source worktree are gone;
  the candidate binary, `manifest.env` (`8e5ffd19...`), `runtime/cutover-backup.env` and both
  backup files are preserved.
- Production untouched: MainPID 1967502 still active with the same executable sha
  `8aca0713...`, both probes 200, served root content sha unchanged, journal err = 0.
- Local cleanup was skipped deliberately: `scripts/cleanup_local_release.ps1` is PowerShell and
  no `pwsh`/`powershell` exists on this Linux host, and no local release directory or build cache
  existed in the release worktree. No destructive cleanup was improvised.

## Next safe action

None outstanding for this release. rc02 is live and finalized; the rollback handle
(`scripts/rollback_release.sh 2026-10-07-routing-allocation-card-rc02`) stays available while the
backups are preserved. Do not chain rollback or finalize into a cutover; each is a separate
confirmed step.
