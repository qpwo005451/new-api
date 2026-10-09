# Routing card + affinity tier-pin release: spec and tickets

Date: 2026-10-09 (CST)
Coordinator: PrimeAgent session in `/home/ra/orca/workspaces/Newapi/prod251-release`
Discipline: orca-matt-orchestration (spec -> tickets -> dispatch -> verify -> two-axis audit)

## Objective

Merge the two verified, unpushed branches into `prod/251`, build a Linux amd64
release candidate locally, verify it on the isolated 4003 instance, then cut the
production instance on `10.0.0.251` over to it after explicit user approval, and
close the release out (finalize + local cleanup).

Originating requests (user, this thread):

1. "那个数字后面的滚动条特别丑" - the native number-input stepper in the routing
   card's percentage field must go.
2. "路由分配卡片整体还是很丑" / "先做2" - redesign the model routing allocation
   card.
3. "我改了路由权重，ds4.1f应该不优先走ollama为什么还有好多请求走了ollama渠道"
   and "单模型路由权重更本没生效" - a sticky session binding must follow a later
   routing weight/priority edit instead of keeping the channel the policy no
   longer prefers.
4. "这两个一起合并到251以后做部署修复" - merge both branches into `prod/251` and
   deploy.

## Fixed points

- base: `ec4fba9c2` (`prod/251` tip, already deployed)
- `codex/routing-card-polish` = `37bd7d389` (also `e18973534`)
- `codex/fix-affinity-tier-pin` = `ba9f842e5`

## Tickets

| id | deliverable | owner | blockers | acceptance evidence |
| --- | --- | --- | --- | --- |
| T1 | Spec-axis review of both branch diffs against the requests above | reviewer worker `review-spec` (read-only) | none | written findings with file:line, or an explicit "no spec gap" plus the checks run |
| T2 | Standards-axis review of both branch diffs against `AGENTS.md` + `web/AGENTS.md` | reviewer worker `review-standards` (read-only) | none | written findings with file:line and the rule violated, or an explicit "no hard violation" plus the checks run |
| T3 | `prod/251` carries both branches, merged without conflict | coordinator | T1, T2 clean | `git log --oneline -3 prod/251`, merged tree passes `go test` + `bun run typecheck` + feature vitest |
| T4 | Local release candidate for the merged commit | coordinator | T3 | `build_release_candidate.sh` exit 0, binary sha256, embedded markers (stepper CSS rule, `channelAffinityPinStillPreferred`, new label) |
| T5 | Candidate staged on the isolated 4003 instance and verified | coordinator | T4 | `smoke_release.sh` ok, schema hash unchanged, 4002 untouched, pin-rebind behavior reproduced on 4003 |
| T6 | Production cutover | coordinator | T5 + itemized user approval of the release id | cutover exit 0, `smoke fast ok`, live binary sha256 == manifest, service active with `NRestarts=0` |
| T7 | Finalize + local cleanup + report | coordinator | T6 | `finalize_release.sh` exit 0, 4003 released, 4002 healthy, candidate DB/logs/dependency trees removed |

## Ownership

- Reviewer workers own nothing: they must not write, commit, stash or fix.
- The coordinator owns `ops/`, the release tooling, the production host, and the
  merge into `prod/251`.

## Wave

Width 2 (the two read-only reviews, independent, no shared files).

## Review round 1 (2026-10-09 ~10:22-10:27 CST)

Two read-only reviewers (codex + glm-5.3-flash, effort max), one per axis, on
`37bd7d389` and `ba9f842e5`.

Spec axis (`review-spec`, dispatch `ctx_fec64b289876`): no missing feature, three
gaps reported - auto-group pins stay stale because the candidate set cannot be
scoped for `usingGroup == "auto"`; a candidate-lookup error fails open to the
stale pin with no log line; strict-mode pins are retained although the literal
request says a session must follow the policy.

Standards axis (`review-standards`, dispatch `ctx_9dbc5b49 4bc`): two hard
findings - the share bar hand-rolled markup and clamping that
`@/components/ui/progress` already provides (AGENTS.md reuse rule); the stepper
CSS fix and the new bar had no regression test (web/AGENTS.md 3.14). Three
judgement calls - the candidate-lookup error was swallowed without logging, the
per-session request fixture was duplicated in the placement tests, and one test
name was stale after its expected total became hidden.

## Fix forward (both axes, before any merge)

- `7aac42e04` (UI): the share bar is now `Progress` from `@/components/ui/progress`;
  a regression test asserts each row's bar value; a new stylesheet test asserts
  the number-stepper rules (following the existing `background-refresh.test.ts`
  precedent); the stale test name is corrected.
- `93531cfc6` (backend): a candidate-lookup failure is reported with
  `common.SysError` before the pin is kept; the duplicated session fixture is one
  helper.

Responses to the two findings that are deliberate, with evidence:

- auto-group: the deployment has no `auto` group (tokens `default` x3, `svip` x1;
  the only user is `svip`; all 730 requests of a 30 minute window used `svip`), so
  no request reaches that branch, and the code comment states the limitation.
- strict mode: dropping the pin for a strict session makes the request fail with
  `strict_session_binding_unavailable` instead of moving, and the deployment has
  no strict rule (`channel_affinity_setting.session_mode = prefer`; rules are
  off/off/prefer).

## Review round 2 (running)

Re-reviews on `7aac42e04` / `93531cfc6`: `rereview-spec`
(`ctx_43e3ab6c0240`) and `rereview-standards` (`ctx_cb29acf595c5`). The merge into
`prod/251` is gated on both coming back without an unresolved hard violation.

## Review round 2 (clean, 2026-10-09 ~10:42-10:50 CST)

`rereview-standards` (`ctx_cb29acf595c5`): all five fixes PASS - shared `Progress`
in use with no hand-rolled bar or clamp left, stylesheet contract test plus the
per-row bar value test present, candidate-lookup failure reported with
`common.SysError`, one shared session fixture, test name corrected. Verdict: "no
remaining hard violation or standards findings". It also independently confirmed
the `format:check` drift is pre-existing and repo-wide (38 files at `93531cfc6`,
both routing-card files already drifted at `ec4fba9c2`, the new stylesheet test
and CSS clean).

`rereview-spec` (`ctx_43e3ab6c0240`): requirements 1-3 verified. Auto-group and
strict-mode retention accepted as deliberate and commented; the reviewer notes it
could not independently verify the production group evidence, and that the
auto-group comment states the scoping limit without spelling out that an existing
pin is kept. Lookup-failure fix verified, fail-open unchanged. The only
outstanding item is requirement 4 - merge and deployment - which is the next
step, not a defect.

Merge gate satisfied: Spec axis has no unresolved requirement failure, Standards
axis has no unresolved hard violation.
