# NewAPI Production Instance 251

This directory documents the production NewAPI instance at `10.0.0.251:/opt/new-api`.

## Source Of Truth
- GitHub fork: `https://github.com/qpwo005451/new-api.git`
- Upstream repo: `https://github.com/QuantumNous/new-api.git`
- Production branch policy: `prod/251` only

## Runtime Scope
- Host: `10.0.0.251`
- App root: `/opt/new-api`
- Stable release checkout: `/opt/new-api-release-runner`
- Service: `new-api.service`
- Production port: `4002`
- Candidate port: `4003`

## Guardrails
- Durable source changes belong in git, not only on the server.
- Secrets stay only on the server.
- Production cutover still requires prepare, verify, and explicit confirmation.
- A confirmed stable release must be finalized with `scripts/finalize_release.sh <release-id>` so build and candidate runtime files do not accumulate.
