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

- Build both frontend themes, run relevant tests, and compile the Linux `amd64` Go binary on the operator workstation.
- Use `scripts/build_release_candidate_local.ps1 -ReleaseId <id> -ReleaseTag <commit>`. It uses the project-private `.local-tools` toolchain, targets `linux/amd64` with CGO disabled, verifies the ELF output, and removes build caches by default.
- Upload only the candidate binary and manifest. Remote-only steps are copying the live environment and database for the isolated `4003` candidate, smoke testing, cutover, rollback, and finalization.
- Do not run dependency installation, frontend builds, Go builds, or repository tests on the production host.
- A remote build is allowed only as an explicitly confirmed emergency fallback. It must run from `/opt/new-api-release-runner`, must not create an outer worktree per release, and must be finalized in the same release workflow.
- After upload and acceptance, run `scripts/cleanup_local_release.ps1 -ReleaseId <id>` to remove the local release directory, detached worktrees, project Go caches, frontend dependency/build directories, and local toolchain caches.

## Finalization

After the operator confirms the release is stable, run:

```bash
scripts/finalize_release.sh <release-id>
```

Finalization verifies that the live binary hash matches the release manifest, stops the matching `4003` candidate if needed, and removes the detached source worktree, candidate database, copied environment, schemas, and candidate logs. It preserves the release manifest, candidate binary, and any cutover rollback metadata and backups.
