# Virtual Pool Client Capture

Use `scripts/capture_virtual_pool_client.py` only with a loopback bind and a
temporary client profile:

```bash
python3 scripts/capture_virtual_pool_client.py \
  --bind 127.0.0.1 \
  --port 18081 \
  --output /tmp/virtual-pool-capture.jsonl
```

Point the client under test at `http://127.0.0.1:18081/v1`. The server accepts
only loopback binds, stores only session-affinity headers, and records both
`session_id` and `prompt_cache_key` as type/presence markers rather than their
values.

Record one JSONL row per request. Compare new, resume, fork, compact, and
subagent requests separately for each installed client version. A missing
session field is a gap, not a pass. Do not use a real API key or production
endpoint.

## Installed-version results

Sanitized fixtures live in `docs/testing/capture-fixtures/`. Each records field
presence and only salted digest relations, never raw session IDs, prompts, or
credentials.

| Client | Installed version | Surface | Default session field | Injected header | Notes |
| --- | --- | --- | --- | --- | --- |
| Codex | 0.157.1 | Responses | `session-id`/`thread-id`/`x-client-request-id` + `prompt_cache_key` | none | resume/fork/compact verified; subagent unproven |
| Pi | 0.87.1 | Responses | `session_id` + `x-client-request-id` + `prompt_cache_key` | `X-NewAPI-Session-ID` via `before_provider_headers` | completions has no default session field |
| Pi | 0.87.1 | Chat Completions | none | `X-NewAPI-Session-ID` via `before_provider_headers` | header is the only affinity source |
| Prime Agent | 0.9.6 | Responses | `session_id` + `x-client-request-id` + `prompt_cache_key` | none (no header hook) | resume/fork/compact verified |
| Prime Agent | 0.9.6 | Chat Completions | none | none | no header hook in installed release |
| Hermes | 0.21.3 | Responses | `prompt_cache_key` only | none | no outbound session-affinity header |
| Hermes | 0.21.3 | Chat Completions | none | none | no outbound session-affinity header |

Hermes was captured both from the local 0.21.3 install and from the deployed
`10.0.0.3` image `hermes-agent:supersearch-lane-v26-20260928-r1` in a disposable
container that never touched production volumes.

## Source repositories vs installed binaries

Client source repositories answer most of the field-behavior questions and are
the right place to derive the extraction contract: header/body field names, when
caching is disabled, and how resume/fork/compact change IDs. They do not prove
which binary a machine actually runs, which fields the installed runtime emits
under a real profile, or whether deployment configuration suppresses them. The
installed versions observed here (Codex 0.157.1, Pi 0.87.1, Prime Agent 0.9.6,
Hermes 0.21.3) differ from several versions recorded during source research, so
source alone cannot stand in for C5 evidence.
