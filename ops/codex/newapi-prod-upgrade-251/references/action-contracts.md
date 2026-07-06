# Action Contracts

## Shared Rules
- Map every request to exactly one action: `check`, `prepare`, `verify`, `cutover`, `rollback`, or `report`.
- If the request is ambiguous, choose the safer earlier action.
- Refuse alternate hosts, roots, ports, or deployments immediately; do not substitute a fallback action.
- Classify the action before checking missing scripts, manifests, runtime proof, or fork provenance.

## check
**Returns**
- Current production baseline
- Fork branch and commit provenance when repository context is available
- Recommended target version or blocker list
- Next safe action

## prepare
**Requires**
- Explicit target version or tag
- Remote build and stage scripts present

## verify
**Requires**
- Explicit release identity for candidate verification, or explicit production scope
- Validation matrix and channel sample matrix

## cutover
**Requires**
- Explicit release identity
- Successful recent `verify`
- Explicit human confirmation in the current thread

## rollback
**Requires**
- Explicit operator request
- Explicit backup handle or release identity

## report
**Returns**
- Current production state
- Fork branch and commit provenance when available
- Release identity when relevant
- Validation status
- Blockers and next safe action
