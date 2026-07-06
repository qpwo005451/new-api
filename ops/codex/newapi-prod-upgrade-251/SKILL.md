---
name: newapi-prod-upgrade-251
description: Dedicated operational skill for the production NewAPI instance at 10.0.0.251:/opt/new-api and its fork-backed maintenance workflow. Use when checking upstream updates, preparing a candidate release, verifying candidate or production state, performing an explicitly confirmed production cutover, running an explicit rollback, or reporting on the latest prepared candidate state for this instance. Refuse requests that target any other host, path, port, or deployment.
---

# NewAPI Production Upgrade 251

1. Treat `10.0.0.251:/opt/new-api` as the only valid runtime target. Refuse requests that target any other host, root, port, or deployment immediately and do not suggest a substitute action.
2. Treat `https://github.com/qpwo005451/new-api.git` branch `prod/251` as the fork source of truth for durable production changes.
3. For in-scope requests, classify every request into exactly one action: `check`, `prepare`, `verify`, `cutover`, `rollback`, or `report`. Do this before checking scripts, manifests, or runtime proof. If the request is ambiguous, choose the safer earlier action.
4. Before doing work, read `references/instance-profile.md`, `references/repository-policy.md`, `references/action-contracts.md`, and `references/artifact-locations.md`.
5. For `verify`, also read `references/validation-matrix.md` and `references/channel-samples.md`.
6. After classifying the action, verify the required remote scripts, manifests, and fork provenance exist before continuing.
7. Never treat "latest prepared" as a valid execution selector for `prepare`, `verify`, `cutover`, or `rollback`. Require an explicit release identity for `cutover`, `rollback`, and release-specific `report`. A non-mutating `report` request about the latest prepared candidate state stays `report`, and may note the missing release identity as a caveat.
8. Never chain `verify` into `cutover`.
9. Never perform skill-level autonomous rollback.
10. For `check`, collect the live baseline, current fork provenance when available, and upstream recommendation only.
11. For `prepare`, require an explicit target version, build the candidate, stage the candidate runtime on `4003`, and capture the release identity manifest.
12. For `verify`, run the `standard` validation set. If required proof is missing, report the verify blockers rather than changing the action. Then prove candidate PID ownership of `4003` and prove the candidate runtime uses the copied runtime database instead of `/opt/new-api/data/new-api.db`.
13. For `cutover`, stop unless the user has explicitly confirmed production cutover in the current thread. Then promote the exact release identity and run fast post-cutover smoke checks.
14. For `rollback`, require an explicit operator request and an explicit rollback handle or release identity, then run minimal health checks.
15. For `report`, summarize the current state, fork branch and commit provenance when available, release identity, validation status, blocker list, and next safe action without mutating production.
16. Use the fixed matrix from `references/channel-samples.md`. Do not auto-pick substitute channels or models.
17. When detailed operational rules are needed, use the reference files instead of inventing procedures.
