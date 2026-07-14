# Action Contracts

## Shared Rules
- Map every request to exactly one action: `check`, `prepare`, `verify`, `cutover`, `rollback`, `finalize`, or `report`.
- If the request is ambiguous, choose the safer earlier action.
- Refuse alternate hosts, roots, ports, or deployments immediately; do not substitute a fallback action.
- Classify the action before checking missing scripts, manifests, runtime proof, or fork provenance.
- Before Task 6 fixes branch tracking, never rely on implicit `git pull` / `git push` defaults on `prod/251`; use explicit remote and branch arguments for networked git actions.
- Treat branch tracking mismatch, including `prod/251` tracking `origin/main`, as a risk for `check` and `report`, and as a blocker for networked git actions until it is explicitly handled.

## check
**Returns**
- Current production baseline
- Fork branch and commit provenance when repository context is available
- Branch tracking risk or blocker when repository context shows a mismatch
- Recommended target version or blocker list
- Next safe action

## prepare
**Requires**
- Explicit target version or tag
- Remote build and stage scripts present

## verify
**Requires**
- Explicit release identity for candidate verification, or explicit production scope for production verification
- Validation matrix and channel sample matrix

**Does**
- Run the `standard` validation set for the requested scope.
- For candidate verify only, prove the expected candidate PID owns port `4003` and the candidate runtime database path points to the copied runtime DB.

**Must Not**
- Require candidate-only runtime proofs for an explicit production-scope verify.
- Fail an explicit production-scope verify solely because candidate-only evidence is absent.

**Returns**
- Pass or fail summary for the requested verify scope
- Candidate runtime proof when the request is candidate verify
- Risk list and verify blockers
- Next safe action

## cutover
**Requires**
- Explicit release identity
- Successful recent `verify`
- Explicit human confirmation in the current thread

## rollback
**Requires**
- Explicit operator request
- Explicit backup handle or release identity

## finalize
**Requires**
- Explicit release identity
- Operator confirmation that the production release is stable
- Live binary hash matching the release manifest

**Does**
- Stop the matching candidate process on port `4003` when still running.
- Remove the detached source worktree and transient candidate runtime files.
- Preserve the candidate binary, manifest, and any cutover rollback metadata and backups.

## report
**Returns**
- Current production state
- Fork branch and commit provenance when available
- Branch tracking risk or blocker when repository context shows a mismatch
- Release identity when relevant
- Validation status
- Blockers and next safe action
