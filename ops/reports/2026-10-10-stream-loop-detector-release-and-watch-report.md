# Stream-loop detector release and false-positive watch report

Release: `2026-10-10-stream-loop-detector`
Operations and observation record: 2026-10-10 CST

## What ships

The detector branch ships one focused relay change: when a whitelisted OpenAI
chat-completions SSE response degenerates into a short repeated unit, the relay
stops the upstream stream instead of letting it run to the output cap.

| branch | commit | scope |
| --- | --- | --- |
| `codex/stream-loop-detector` | `a10943d17` | 4 files, +610/-1: `relay/channel/openai/relay-openai.go` and `relay/common/stream_status.go` changed; `relay/helper/stream_loop_detector.go` and `relay/helper/stream_loop_detector_test.go` added |

The detector watches decoded `content` and `reasoning_content` only. It uses a
4 KiB rolling window, accepts repeated units of at most 256 bytes, and requires
at least four complete repeats, at least 600 bytes in the repeated run, and at
least 80 percent of the window suffix. Period boundaries respect UTF-8 rune
boundaries, and the detector reports the smallest viable period.

On a trigger, the handler sets `StreamEndReasonUpstreamLoop`, marks the stream
failed, calls `sr.Stop` to interrupt the upstream stream, and closes the
upstream response body. The attempt is marked as an
`AttemptFailureProtocol` failure with downstream committed. The client
receives an OpenAI-style SSE error chunk with `type=upstream_stream_error` and
`code=upstream_loop_detected`, rather than a normal `[DONE]`; an agent can then
recognize it as a stream error and retry. Detection applies only to
`RelayModeChatCompletions`; tool-call argument deltas are not scanned and no
billing path is changed.

## Rollout

Merge and deploy the reviewed commit as follows:

1. Start from up-to-date `prod/251` and confirm it is at `6951f198d` before the
   merge.
2. Run `git merge --ff-only a10943d17`. The branch is a single reviewed commit
   on top of that base, so fast-forward keeps the reviewed commit id unchanged
   and avoids adding an unnecessary merge commit. As actually executed on
   2026-10-10, `prod/251` had gained the same-day `db62eb538` ops report after
   the branch was cut, so fast-forward was impossible; the merge used
   `--no-ff` instead (merge commit `a3f63a1ce`). The reviewed commit ids
   `a10943d17` and the same-day whitelist extension `f201f21c5` are unchanged
   in the merged history.
3. Build on the local workstation with `go build ./...` (or the normal local
   release-candidate flow when a full artifact is required), deploy the built
   binary, and restart `new-api.service`.
4. Confirm startup, `/api/status`, and the signatures in the next section after
   restart.

Rollback is a single-code revert because the detector has no schema, option, or
frontend dependency: run `git revert --no-edit a10943d17`, rebuild locally,
deploy that binary, and restart the service. There is no database rollback and
no persisted detector state to remove.

## Monitoring

Watch the server log for the trigger signature emitted with
`logger.LogError`:

```text
upstream stream loop detected: model=... period=... repeats=... snippet=...
```

The model, byte-length period, complete-repeat count, and an 80-byte rune-safe
snippet are recorded at trigger time. The downstream SSE signature is the error
chunk code `upstream_loop_detected`. The end reason is `upstream_loop`, which is
deliberately outside `IsNormalEnd`, so health sampling counts the attempt as an
upstream failure.

Suggested watch metrics:

| metric | purpose |
| --- | --- |
| daily trigger count grouped by model | detect a sudden provider-side loop or an unexpectedly broad model match |
| agent retry success rate after `upstream_loop_detected` | verify the intended recovery path works in production |
| false-positive candidates | flag triggers whose snippet looks like valid repeated content, such as a long repeated code block or table |

`journalctl -u new-api.service -g 'upstream stream loop detected'` is the
quickest daily triage command.

## False-positive triage

A real degenerate loop usually shows the same short phrase fragment, such as
`let me `, or the same short sentence repeated in the logged snippet. If the
snippet is structural repetition, such as a code block, divider, or table row,
and the response would otherwise have finished normally, record it as a
false-positive candidate before changing thresholds.

For a confirmed false-positive pattern, make the detector more conservative by
following the tuning comments at the top of
`relay/helper/stream_loop_detector.go`: increase
`loopDetectorMinRepeats`, `loopDetectorMinTotalBytes`, or
`loopDetectorMinSuffixPercent`, or decrease `loopDetectorMaxPeriodBytes`.
Collect several snippets first so a single example does not drive the change.

## Threshold tuning table

| constant | current value | purpose | larger value effect | smaller value effect |
| --- | ---: | --- | --- | --- |
| `loopDetectorWindowBytes` | 4096 | bounds the rolling body-text window scanned for periodicity | retains more recent output; tune with care because this is context, not a direct loop trigger | forgets older output sooner |
| `loopDetectorMaxPeriodBytes` | 256 | largest repeated unit considered | more conservative: fewer false positives and a later stop | stops sooner but risks flagging legitimate short repetition |
| `loopDetectorMinRepeats` | 4 | minimum complete back-to-back repeats | more conservative: fewer false positives and a later stop | stops sooner but risks legitimate repeats |
| `loopDetectorMinTotalBytes` | 600 | minimum byte length of the repeated run | more conservative: fewer false positives and a later stop | stops sooner but lets short accidental repeats through |
| `loopDetectorMinSuffixPercent` | 80 | minimum share of the rolling window covered by the repeated run | stronger suffix domination, guarding normal responses that merely end with a long repeated block | lower domination requirement and higher false-positive risk |

`loopDetectorSnippetBytes` is fixed at 80 for the log excerpt and is not a
detection threshold.

## Whitelist expansion

Detection is opt-in through `loopDetectorModelPrefixes` in
`relay/helper/stream_loop_detector.go`. Matching is case-insensitive, trims
input, strips an optional organization prefix at the last `/`, and tests each
entry with `strings.HasPrefix`.

Keep the more specific v4.1 entries before broader v4 entries: `HasPrefix` is
evaluated entry by entry and returns on the first match, so ordering keeps the
intended family distinction visible rather than letting a broad `deepseek-v4`
prefix swallow the v4.1 match. To add a family, append the model prefix and add
matching positive and negative cases to `TestLoopDetectorEnabledWhitelist`.
The `deepseek-v4-flash` family expansion is handled by the same-day T2 task and
is intentionally outside this document-only task.

## Billing decision

A detected loop bills the tokens already consumed by the response. The handler
continues to use valid upstream usage when present; otherwise it estimates from
the accumulated response text. Decision (2026-10-10): billing consumed tokens
as-is is final — refund and half-price treatment are deliberately not wanted
(the deployment is single-user personal use), so this record closes the
decision instead of deferring it. This release does not touch any billing path
or pricing rule.

## Verification

The checks below were reproduced from the reviewed commit in a temporary
detached evidence worktree. This task itself only adds the report; the
coordinator should re-run build and tests on the integrated tree.

| check | result |
| --- | --- |
| `go build ./...` | passes with `web/dist` present; the bare evidence worktree first returned the expected `pattern web/dist: no matching files found` embed error |
| `go test -count=1 ./relay/helper ./relay/channel/openai ./relay/common` | all three packages pass |
| `gofmt -d <four changed Go files>` | no diff |
| `cd relaykit && GOWORK=off go build ./...` | passes |
| ownership | exactly the four files in the commit table; no unrelated files |
| two-axis audit | Standards has two judgement findings and no hard finding; Spec R1-R8 covered |

## Cutover and finalization (2026-10-10 16:06 CST)

The user approved the cutover for this exact release id in-thread, so
`cd /opt/new-api && scripts/cutover_release.sh 2026-10-10-stream-loop-detector-rc01`
was run. Exit 0, `smoke fast ok: http://127.0.0.1:4002`.

- live binary `sha256sum /opt/new-api/new-api` = `35e08a12103ee216f7685be7b3ceec11bb9d47f8c48c4ff8370acad7dc1bed4d`, equal to the manifest.
- `new-api.service` active, new MainPID 2867566, `NRestarts=0`, started 2026-10-10 16:06:18 CST; `/api/status` and `/` both 200.
- the detector strings `upstream_loop_detected`, `upstream stream loop detected` and `degenerate repetition loop` are embedded in the deployed binary; the `affinity_pin_not_preferred` fix from rc02 is still embedded.
- candidate verification before cutover: staging on port 4003 with `SQL_DSN=local`, schema hash unchanged before and after startup, `smoke fast ok` and `smoke full ok` with `SMOKE_MODEL=glm-5.3-flash` (the default-model 502 is the known external `input.codes` upstream outage, unchanged from rc02), production untouched throughout.
- rollback handles recorded in `runtime/cutover-backup.env`: `PREVIOUS_BINARY_SHA256=bc476833d0ac54322ea8c8036930dddd51219da5827cb72769e829a4d5c136de`, `BACKUP_BIN=runtime/live-new-api.20261010-080613.bak`, `BACKUP_DB=runtime/live-new-api.db.20261010-080613.bak`, `LIVE_SCHEMA_SHA256=5dfc39a37abe259ce99e0aeb384875a89853966fe120275fe89a9c331bac9a2b`. Rollback: `cd /opt/new-api && scripts/rollback_release.sh 2026-10-10-stream-loop-detector-rc01`.
- `scripts/finalize_release.sh 2026-10-10-stream-loop-detector-rc01` exit 0: candidate on 4003 stopped and the port released, production still active with `NRestarts=0`, and the candidate binary, `cutover-backup.env`, the previous binary and the previous database backup are preserved for rollback.
- production behavior immediately after cutover, read-only: no `upstream stream loop detected` line in the journal in the first minutes after the restart, which is expected because no deepseek-v4/v4.1-flash traffic triggered a loop yet; watch the metrics in Monitoring above from this timestamp.
