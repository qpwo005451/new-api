# NewAPI Production Instance 251

This directory documents the production NewAPI instance at `10.0.0.251:/opt/new-api`.

## Source Of Truth
- GitHub fork: `https://github.com/qpwo005451/new-api.git`
- Upstream repo: `https://github.com/QuantumNous/new-api.git`
- Production branch policy: `prod/251` only

## Runtime Scope
- Host: `10.0.0.251`
- App root: `/opt/new-api`
- Stable release checkout: `/opt/new-api-release-runner`
- Service: `new-api.service`
- Production port: `4002`
- Candidate port: `4003`

## Guardrails
- Durable source changes belong in git, not only on the server.
- Secrets stay only on the server.
- Builds and repository tests run locally; production receives release artifacts only.
- Production cutover still requires prepare, verify, and explicit confirmation.
- A confirmed stable release must be finalized with `scripts/finalize_release.sh <release-id>` so build and candidate runtime files do not accumulate.
- The matching local release must be removed with `scripts/cleanup_local_release.ps1 -ReleaseId <release-id>` after remote acceptance.

## Release History (host 251)

Binary hashes observed live on the production host, newest first. Use this when a live binary hash must be tied to a
release and commit.

| Release id | Source commit | Binary sha256 (first 8) | Notes |
| --- | --- | --- | --- |
| `2026-09-29-rc40` | `f053d74e3` | `43858fcb` | rc.26 ... rc.40 upstream merge plus the Opus 5 thinking-disable guard; explicit schema migration, cutover and finalization on 2026-09-29 20:47. Rollback handle: `releases/2026-09-29-rc40/runtime/cutover-backup.env`. |
| `2026-09-29-claude-effort-rc01` | `99510d05f` | `7129c19f` | Claude family default-effort change (PR #20); cutover 2026-09-29 21:16 after candidate verify on 4003. Rollback handle: `releases/2026-09-29-claude-effort-rc01/runtime/cutover-backup.env` (previous binary `43858fcb`). |
| `2026-09-29-affinity-circuit-breaker-rc02` | `8341cae96` (`prod/251` head) | `ed5ec59a` | Live until the rc40 cutover. Manifest and candidate binary are kept under `/opt/new-api-release-runner/releases/2026-09-29-affinity-circuit-breaker-rc02/`, not in `/opt/new-api-release-archive` (archive directory is empty). |
| `2026-09-29-model-health-rc01` | `cca4fdc4c` | `522f3b02` | Cutover 2026-09-29 07:45; rollback metadata under `/opt/new-api-release-runner/releases/2026-09-29-model-health-rc01/runtime/cutover-backup.env`. |
| (release id not recorded) | `054c5b32a` | `b1654690` | Previous binary recorded by the rc01 cutover backup; the archiving policy was not followed for it. |

### Observed deviations

- `/opt/new-api-release-archive` (the documented retained-binary location) is empty; recent release binaries and manifests
  live in `/opt/new-api-release-runner/releases/` instead, and per-release directories under `/opt/new-api/releases/` are
  removed after finalization.
- The `/opt/new-api-release-runner` checkout has uncommitted modifications to `scripts/smoke_release.sh` and
  `scripts/stage_release_runtime.sh`. The content matches the current fork scripts (isolation acknowledgements and
  `NODE_TYPE=slave` for candidates), so it is drift from a manual sync rather than a divergent change. Align the checkout
  before the next emergency remote build.
