# Validation Matrix

## Validation Levels

### standard
Run all of the checks below unless the user explicitly asked for a narrower scope.

## Core Availability Checks
1. `GET /` succeeds.
2. `GET /api/status` succeeds.
3. Authenticated `GET /v1/models` succeeds against the target runtime.
4. Authenticated `POST /v1/chat/completions` succeeds against the target runtime.
5. Authenticated `POST /v1/responses` succeeds against the target runtime.

## System Settings Surface
Use one of these in order:
1. Preferred: the authenticated admin settings page in a browser session loads without `500`.
2. Fallback: the authenticated backend endpoint that powers the settings surface responds without `500`.

If neither authenticated surface is available, fail closed and report the blocker.

## Patch And Override Checks
Confirm at least:
- stream scanner timeout customization is present,
- image-generation filtering customization is present,
- local option override manifest is present,
- expected local option override entries are present.

## Candidate-Specific Proofs
For candidate verification only:
- the expected candidate PID owns port `4003`,
- the candidate runtime database path points to `/opt/new-api/releases/<release-id>/runtime/new-api.db`,
- the candidate rehearsal is not accidentally using production `4002`,
- the candidate rehearsal is not accidentally using `/opt/new-api/data/new-api.db`.

## Channel Sample Checks
Use the fixed matrix from `channel-samples.md`.
Do not auto-pick substitutes.

## Required Provenance
- Current fork branch
- Current fork commit
- Current production commit
- Current release report path

## Pass/Fail Rules
- Pass only if every required check succeeds.
- Fail closed on missing scripts, missing manifests, missing auth context, ambiguous release identity, or missing runtime proof.
- Separate candidate results from production results in the summary.
- Include the exact failing check name and the next safe action.
