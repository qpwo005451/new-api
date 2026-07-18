# In-Flight Usage Logs as first-class usage-log rows

Usage logs only recorded terminal consume/error rows, so operators could not see requests still talking to upstream. We will add a pending usage-log type written after the pre-consume gate, finalize that same row into consume/error by request identity, and stale-finalize abandoned pending rows after 30 minutes without refunding in the sweeper. Pending writes are best-effort and never block the relay path; relational log databases update in place, and ClickHouse is a safe degradation path rather than a hard requirement for v1.

## Status

accepted

## Considered Options

- Memory-only active request panel
- Separate debug-trace tables like MetAPI
- Pending row delete-and-reinsert on completion
- Side `request_runtime` table merged only in the API

We rejected those for v1 because the product goal is visibility inside the existing usage-log page with one row per request, while keeping quota statistics free of in-flight noise.

## Consequences

- `RecordConsumeLog` / `RecordErrorLog` become pending-aware finalizers with insert fallback
- Operators get an `InFlightUsageLogEnabled` option (default on, gated by consume logging)
- Stale pending rows become zero-quota error rows after 30 minutes
- First release covers main HTTP Relay only, not realtime/websocket or async task platforms
