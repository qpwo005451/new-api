# Virtual Pool Capacity 4003 Test Plan

Scope: validate the virtual-pool capacity work on `10.0.0.251:4003` only.
Production `4002` and `/opt/new-api/data/new-api.db` are read-only references
and must never be used as the test target.

## 1. Safety Gates

- Candidate binary must be built locally from an explicit commit.
- Candidate must run from `/opt/new-api/releases/<release-id>/bin/new-api`.
- Candidate must use `/opt/new-api/releases/<release-id>/runtime/new-api.db`.
- Candidate must not share production Redis or run master-only jobs.
- Full smoke and all relay calls must target loopback mock upstreams only.
- Any test that would reach a real provider is out of scope for this plan.
- Every test case records: request id, candidate PID, DB path, selected
  channel, selected account identity, active capacity, result, and cleanup.

## 2. Test Fixtures

Use a copied SQLite database, never production. Add:

- one virtual model, for example `vpool-capacity-lab`;
- three Input-like channels with the same model and one shared capacity group;
- one Ollama-like channel with its own shared capacity group;
- one CommandCode-like channel with an independent capacity group;
- loopback mock upstreams that record concurrent calls and can hold a request
  open on demand.

## 3. Functional Matrix

| ID | Scenario | Expected result |
| --- | --- | --- |
| F01 | Three Input channels share `capacity_groups.input-subscriptions.capacity=15` | 15 concurrent attempts are admitted across the group; the 16th waits then returns 429 |
| F02 | Same 15 slots under `round_robin` | admissions spread across all three Input channels instead of pinning channel A |
| F03 | Ollama group configured with `capacity=3` | 3 concurrent attempts admitted; the 4th waits then returns 429 |
| F04 | CommandCode group configured separately | its limit does not consume or block Input/Ollama slots |
| F05 | `targets[].channel_id` points at one channel | only that channel is selected for the target |
| F06 | Two same-name Input channels without `channel_id` | both remain distinct candidates and share the group counter |
| F07 | Runtime edit of group capacity 15 -> 16 | the next request sees 16 without restarting the candidate |
| F08 | Runtime edit of group capacity 16 -> 14 | the next request is capped at 14; existing leases drain or expire |
| F09 | Group member missing local `capacity` | it inherits the group ceiling, not 1 |
| F10 | Explicit group ceiling vs member capacity | explicit group ceiling wins |
| F11 | `rotation=ordered` | stable order is preserved while capacity still applies |
| F12 | `rotation=random` | candidates vary but never exceed the group ceiling |
| F13 | `health.enabled=true` and one member failing | cooling member is moved behind healthy members |
| F14 | Sticky session enabled | repeated session reuses one member while capacity remains |
| F15 | Sticky disabled | capacity group still enforces the shared limit |
| F16 | Previous response owner | continuation stays on the original candidate or fails closed |
| F17 | `health.disable_model=true`, repeated unavailable failures | the failing model candidate is skipped until cooldown expiry and never used as a last resort |
| F17b | A disabled candidate reaches cooldown expiry | the member is skipped during cooldown and is selectable again after expiry; use a short cooldown in the lab while production uses 300 seconds |

## 4. Failure And Boundary Cases

- Capacity 0: behaves as the default single slot.
- Negative capacity: option update is rejected.
- Empty group name: option update is rejected.
- Group name longer than 128 characters: option update is rejected.
- Target `channel_id` that does not exist: candidate is absent; no cross-pool
  fallback to an unconfigured upstream.
- Channel disabled mid-flight: new admissions stop; in-flight request follows
  existing lease/outcome rules.
- Candidate restart: in-memory leases are dropped; Redis mode is tested only
  with a dedicated loopback Redis instance.
- Redis unavailable with `redis_required_for_ready=true`: new admission fails
  closed rather than silently splitting capacity.
- Client disconnect while waiting for capacity: wait ends, no lease leaks.
- Upstream 429/500: health state changes only on real request outcomes.

## 5. Execution Order

1. Prove candidate identity, PID, port, binary hash, and copied DB path.
2. Run `fast` smoke: `/`, `/api/status`, authenticated `/v1/models`.
3. Configure mock-only fixture routes and mock upstreams.
4. Run F01-F06 sequentially, recording admission counts.
5. Run F07-F10 for runtime capacity changes.
6. Run F11-F13 for rotation and health.
7. Run F17-F17b for strict model-level cooldown.
8. Run F14-F16 for sticky/private-state behavior.
9. Run boundary cases and failure injection.
10. Re-run `fast` smoke and verify candidate still uses the copied DB.
11. Stop 4003, preserve logs and evidence, and report pass/fail/blockers.

## 6. Evidence And Stop Conditions

Evidence:

- candidate PID and `/proc/<pid>/exe`;
- `ss -ltnp` showing port 4003 owned by that PID;
- candidate env with `SQLITE_PATH` pointing at the copied DB;
- mock upstream call log with concurrency and selected-channel counts;
- option snapshots before and after each runtime change;
- exact HTTP status, `Retry-After`, and error body for rejected admissions.

Stop immediately if any test:

- reaches a real provider;
- writes to the production DB;
- admits more than the configured group ceiling;
- returns a success while the selected candidate violates `channel_id`;
- leaks a capacity lease after the request ends.

Rollback is simply stopping the 4003 candidate and removing only the temporary
mock fixtures. Do not touch 4002 or production data.
