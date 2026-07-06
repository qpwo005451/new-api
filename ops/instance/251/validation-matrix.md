# Validation Matrix

## Required Checks
1. `GET /api/status` returns `200`.
2. `GET /v1/models` returns `200` with auth.
3. `POST /v1/chat/completions` returns `200` with auth.
4. `POST /v1/responses` returns `200` with auth.
5. Admin settings surface does not return `500`.
6. Candidate verification proves port `4003` ownership and copied DB usage.

## Required Provenance
- Current fork branch
- Current fork commit
- Current production commit
- Current release report path
