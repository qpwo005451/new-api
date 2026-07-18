# In-Flight Usage Logging

This context defines how a client request becomes visible in usage logs while it is still being sent to, or waiting on, an upstream provider.

## Language

**In-Flight Usage Log**:
A usage-log row created after billing pre-consume succeeds (or is skipped for a free model) and before the request finishes. It represents a request that is accepted and is on the path to, or currently with, an upstream provider.
_Avoid_: Live request, active request panel, debug trace, proxy log

**Pending Log Type**:
The dedicated usage-log type value for an In-Flight Usage Log. It is distinct from consume and error terminal types so quota statistics never treat in-flight rows as completed spend.
_Avoid_: Unknown type, consume-with-status-flag

**Pending Finalization**:
The update of one In-Flight Usage Log into a terminal consume or error row for the same request identity, without inserting a second usage row for that request.
_Avoid_: Delete-and-reinsert, paired start/end logs

**Request Identity**:
The stable request identifier assigned at ingress and carried on the In-Flight Usage Log and its finalized terminal row.
_Avoid_: Database log id alone, upstream request id alone

**Stale In-Flight Log**:
An In-Flight Usage Log whose age exceeds the stale threshold without Pending Finalization. It is treated as an abandoned request for log hygiene.
_Avoid_: Hung stream, still-valid long request without qualification

**Stale Threshold**:
The maximum age of an In-Flight Usage Log before it becomes a Stale In-Flight Log. The default is 30 minutes and may be configured later.
_Avoid_: Request timeout, upstream first-byte timeout

**Stale Finalization**:
The conversion of a Stale In-Flight Log into a terminal error row with zero billed quota, without attempting balance refund in the sweeper.
_Avoid_: Stale refund, hard delete of pending rows

**Pre-Consume Gate**:
The billing checkpoint after which an In-Flight Usage Log may be created. Requests rejected before this gate do not create usage rows.
_Avoid_: Auth gate, distribute gate

**In-Flight Log Visibility**:
Administrators may see all In-Flight Usage Logs; a non-admin user may see only their own. Channel names and admin-only fields remain hidden from non-admin views.
_Avoid_: Admin-only pending, anonymous live feed

**In-Flight Log Switch**:
An operator setting that enables or disables creation of In-Flight Usage Logs. It defaults to enabled and has no effect when consume logging is disabled.
_Avoid_: Hard-coded always-on, environment-only toggle without an option

**Usage Log Auto Refresh**:
An optional client-side refresh of the usage-log list, default off, with a three-second interval when enabled, so In-Flight Usage Logs can be observed while they remain pending.
_Avoid_: Server push, forced page polling

**Pending Claim**:
The lookup that finds the In-Flight Usage Log for a finishing request, preferring the pending log id stored on the request context and otherwise matching request identity plus pending type and user.
_Avoid_: Blind insert of a second terminal row when a pending row exists

**Best-Effort Log Write**:
A logging write that records system errors on failure but does not fail or block the client request or billing outcome. Missing pending rows fall back to ordinary terminal inserts.
_Avoid_: Hard dependency on log database availability
