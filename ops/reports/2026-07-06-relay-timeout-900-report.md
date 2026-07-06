# 2026-07-06 RELAY_TIMEOUT Change

Scope: production NewAPI instance at `10.0.0.251:/opt/new-api`

## Change

- File changed: `/opt/new-api/.env`
- Setting changed: `RELAY_TIMEOUT=420` -> `RELAY_TIMEOUT=900`
- Reason: repeated long-running `/v1/responses` requests on `gpt-5.5` were hard-failing at `420s` with `scanner_error` and `context deadline exceeded (Client.Timeout or context cancellation while reading body)`.

## Backup

- Backup file: `/opt/new-api/.env.bak-relay-timeout-20260706-081028`

## Restart

- Service restarted: `new-api.service`
- Post-restart PID: `1959354`

## Verification

- `.env` now contains `RELAY_TIMEOUT=900`
- `GET /api/status` returned successfully after restart
- `journalctl -u new-api.service` showed fresh healthy traffic after restart, including:
  - `GET /v1/models` with `200`
  - multiple `POST /v1/responses` with `200`

## Notes

- This change removes the previous `420s` hard ceiling from the NewAPI-side relay client timeout path.
- It does not eliminate upstream `502/524` failures or client-side `client_gone` cancellations from Codex/Desktop.
- Keep this override in mind during future NewAPI upgrades and reapply it if `.env` is regenerated or replaced.
