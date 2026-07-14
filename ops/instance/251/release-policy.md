# Release Policy

## Interim Warning

Until Task 6 publishes `origin/prod/251` and fixes upstream tracking, implicit `git pull` / `git push` on `prod/251` is not safe.
Use explicit remote and branch arguments for any networked git command on `prod/251`, and inspect branch tracking before relying on pull or push defaults.

1. Work in `topic/*` branches from `prod/251`.
2. Reconcile upstream changes in the topic branch first.
3. Merge approved work back into `prod/251`.
4. Push `prod/251` to `origin`.
5. On production, fetch the fork and run prepare, verify, and explicit cutover.
6. Never deploy directly from a personal branch.

## Build Placement

- Prefer building both frontend themes and the Linux `amd64` Go binary on the operator workstation, then upload only the candidate binary and manifest.
- Remote-only steps are copying the live environment and database for the isolated `4003` candidate, smoke testing, cutover, rollback, and finalization.
- If the operator workstation does not have Go and Bun, the production build helper may be used as a fallback. Its detached source worktree is removed immediately after a successful build and on build failure.
- Remote fallback builds must run from the stable `/opt/new-api-release-runner` checkout. Do not create an outer worktree per release; the build helper already creates and removes the only source worktree it needs.

## Finalization

After the operator confirms the release is stable, run:

```bash
scripts/finalize_release.sh <release-id>
```

Finalization verifies that the live binary hash matches the release manifest, stops the matching `4003` candidate if needed, and removes the detached source worktree, candidate database, copied environment, schemas, and candidate logs. It preserves the release manifest, candidate binary, and any cutover rollback metadata and backups.
