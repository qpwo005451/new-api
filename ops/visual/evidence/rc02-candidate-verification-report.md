# Candidate VERIFY — release `2026-10-07-routing-allocation-card-rc02`

**Overall verdict: PASS** (candidate scope + read-only production baseline).
Action class `verify` only. No cutover/rollback/finalize was run.

- Target: `10.0.0.251:/opt/new-api` (ssh `root`, `ssh -o BatchMode=yes`).
- Release dir: `/opt/new-api/releases/2026-10-07-routing-allocation-card-rc02/`
- Source commit: `6c2ae0d56330556d695bd88ded0616485687cc4a`
- Candidate: port `4003`; production: port `4002` (untouched).

## Release identity (re-derived from the manifest)

`cat /opt/new-api/releases/2026-10-07-routing-allocation-card-rc02/manifest.env`

| Field | Manifest | Observed |
| --- | --- | --- |
| RELEASE_ID | 2026-10-07-routing-allocation-card-rc02 | same |
| RELEASE_COMMIT / SOURCE_TREE_COMMIT | 6c2ae0d56330556d695bd88ded0616485687cc4a | same |
| BINARY_SHA256 | 8aca07135ff8f45e1e80708d63116efc9750eb96326360268e1f70bd445cf0d7 | `sha256sum bin/new-api` = same (PASS) |
| LOCAL_OPTION_OVERRIDES_SHA256 | cc1e8fffa62bb9b7ca3d3af13b972df6522c6e1098635c790dc9d25cca8683b0 | host file sha256 = same (PASS) |

## A. Candidate runtime proofs — PASS

| Check | Command | Observed |
| --- | --- | --- |
| PID owns 4003 | `ss -ltnp '( sport = :4003 )'` | `users:(("new-api",pid=1887027,fd=12))`; `runtime/candidate.pid` = 1887027 |
| Process identity | `ps -o pid,ppid,lstart,cmd -p 1887027` | `/opt/new-api/releases/2026-10-07-routing-allocation-card-rc02/bin/new-api --port 4003` |
| Runtime DB, not production | `grep -E '^(PORT|SQLITE_PATH)=' runtime/candidate.env` + `/proc/1887027/environ` + `ls -l /proc/1887027/fd` | `PORT=4003`, `SQLITE_PATH=/opt/new-api/releases/.../runtime/new-api.db`; fds 7/8/13 → runtime DB; cwd = `.../runtime`; **0** references to `/opt/new-api/data/new-api.db` |
| Not using 4002 | `ss -ltnp '( sport = :4002 )'` | owned by separate PID 1764190 (production); candidate PORT=4003 |

## B. Standard validation set (executed on the host, `http://127.0.0.1:4003`) — PASS

Token read the way `scripts/smoke_release.sh` does: `select key from tokens where status = 1 order by id limit 1;` (token value never printed).

| # | Check | Command (essence) | Result |
| --- | --- | --- | --- |
| 1 | `GET /` | `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:4003/` | **200** |
| 2 | `GET /api/status` | `curl ... /api/status` | **200** |
| 3 | authenticated `GET /v1/models` | `curl -H "Authorization: Bearer <token>" /v1/models` | **200**, 62 models; `deepseek-v4.1-flash` present, `gpt-oss:20b` present |
| 4 | authenticated `POST /v1/chat/completions` | see fixed samples below | **200** |
| 5 | authenticated `POST /v1/responses` | `{"model":"deepseek-v4.1-flash","input":"ping","max_output_tokens":8}` | **200** (`status":"incomplete"`, expected for the 8-token cap) |
| 6 | System settings surface | `GET /api/option/` and `/api/option/request_policy` with the root dashboard PAT from the candidate DB | **200** (58870 / 4445 bytes); `GET /api/user/self` also 200. A relay API token returns 401 on these RootAuth routes — expected, not a failure. |

### Fixed channel samples — PASS (no substitution)

| Sample | Model | Endpoint | Result |
| --- | --- | --- | --- |
| deepseek-baseline | `deepseek-v4.1-flash` | `POST /v1/chat/completions` | **200** (`choices[0].finish_reason=length`) |
| ollama-baseline | `gpt-oss:20b` | `POST /v1/chat/completions` | **200** (`usage.total_tokens=69`) |

### Patch and override checks — PASS

| Check | Command | Observed |
| --- | --- | --- |
| stream-scanner timeout customization | `grep -a -c STREAM_SCANNER_MAX_BUFFER_MB bin/new-api`, `STREAMING_TIMEOUT` | present (1 each); source `common/init.go` has `GetEnvOrDefault("STREAM_SCANNER_MAX_BUFFER_MB", 128)` / `STREAMING_TIMEOUT` |
| image-generation filtering | `grep -a -c filterImageGenerationTool bin/new-api` | present (1); source `relay/responses_handler.go` calls it |
| local option override manifest | `ls` + `sha256sum /opt/new-api/patches/local-option-overrides.json` | present; sha256 `cc1e8fff…` = manifest `LOCAL_OPTION_OVERRIDES_SHA256` |
| expected override entries | candidate DB `options` table | `ModelRatio`: deepseek-v4.1-flash=0.15, gpt-oss:20b=0.009, glm-5.3-flash=0.075; `CompletionRatio`: 4 / 5; `CacheRatio`: 0.02 / 0.5; `GroupRatio`: {default:0,vip:0,svip:0}; `billing_setting.billing_mode` includes `kimi-k2.7-code: tiered_expr`; `billing_setting.billing_expr` includes the kimi expression — all match the manifest |

### `scripts/smoke_release.sh full` — PASS

```
cd /opt/new-api && VIRTUAL_POOL_SMOKE_ISOLATED=1 SMOKE_MODEL=deepseek-v4.1-flash \
  RELEASE_ID=2026-10-07-routing-allocation-card-rc02 timeout 300s \
  scripts/smoke_release.sh http://127.0.0.1:4003 \
  /opt/new-api/releases/2026-10-07-routing-allocation-card-rc02/runtime/new-api.db full
```
Observed: `smoke full ok: http://127.0.0.1:4003 model=deepseek-v4.1-flash`, exit 0.

## D. Served-bundle proof (candidate carries THIS release's frontend) — PASS

Desktop entry from `GET /`:

| Bundle | sha256 | `Average split` | `No enabled channel serves this model.` | locked-tier copy |
| --- | --- | --- | --- | --- |
| `/static/js/index.057de9ae8e.js` | `5754317cce360c947e6b662f8333adf04f97c0be2a0dddf16a77b6430fa2f50a` | 2 | 2 | 1 |

Mobile shell `GET /m/`:

| Bundle | sha256 | locked-tier copy |
| --- | --- | --- |
| `/m/static/js/index.c0a26bbaa1.js` (entry) | `36f5098a1338c6468c985af2b2b047117b39a3c3c8cb95788caf4557ecd86cd2` | 1 |
| `/m/static/js/616.c0eabd7048.js` | `37465cdf617b10e15dd17e904444f8ecc343c4bfee81a51a474ed917b4a51759` | 0 |
| `/m/static/js/lib-react.c603f97b2f.js` | `9116c7b1fca6c735c818cc9ecdc6ec114a261cc725edd0d63676f64219226e06` | 0 |

`Channel data is unavailable, so this tier keeps its current weights.` is present in the desktop entry and the mobile entry. The desktop entry hash differs from the rc01 production entry (`index.cba5c67ce7.js`), i.e. the candidate serves the rc02 build.

## Real-browser render check (mandatory acceptance) — PASS

Harness `ops/visual/model-weights-verify.cjs` against `http://10.0.0.251:4003`, one reused browser context, real UI login (`visual-qa`), 3 viewports, same `+1` edit + restore as the rc01 baseline. Artifacts: `ops/visual/out/rc02-6c2ae0d56/` (9 PNGs, `report.json`, `patch-payloads.json`).

| Acceptance item | rc01 baseline (broken) | rc02 candidate | Verdict |
| --- | --- | --- | --- |
| No dead space name ↔ share | median 971.95 px @1440 / 1451.95 px @1920 | **20.92 px** median (min 12, max 52.2) at all viewports | PASS |
| Tier header stronger than rows | tier/row visual weight 0.34 | **1.50** (tier 166,320 = 14 px / 600 / contrast 19.8 vs row 110,880 = 14 px / 400 / 19.8) | PASS |
| Numeric columns aligned | share aligned | share / advanced-priority / advanced-weight all **aligned, left+width spread 0** at 1440x900, 1920x1080, 390x844 | PASS |
| Mobile rows single-line | 9/9 stacked, 60 px rows | **0/9 stacked**, row height 40–41 px, share at x 144.97 | PASS |
| No horizontal scroll | none | none (`scrollWidth == clientWidth`) | PASS |
| Console errors | 2 (pre-login/external) | 2 on viewport 1 only — `POST /api/user/auth/refresh` 401 (pre-login) and `GET api.github.com/.../releases` 403 (external). Viewports 2/3: **0**. Supplementary post-login probe `ops/visual/console-clean-check.cjs` (1440x900) reset the collectors after login and measured the card route: **0 console errors, 0 failed requests** (`console-clean-check.json`, exit 0). | PASS |
| Behavior equality | — | edit and restore PATCH bodies **byte-identical** to `rc01-baseline/patch-payloads.json` (and to the fix dev-server run); HTTP 200 / 200; edit 77→78 redistributes tier weight 12→11, restore returns it | PASS |

Screenshot content check: all 9 PNGs are non-blank (837–1019 distinct colors); rc02 vs the verified fix dev-server render is **99.68 % pixel-identical** (4164/1,296,000 differing pixels), while rc02 vs the rc01 broken baseline differs substantially (RMSE 0.057–0.109) — the candidate carries the redesigned card.

DOM facts: 2 model cards (`deepseek-v4.1-flash` 5 rows, `glm-5.3-flash` 4 rows), 3 tiers each, 9 share inputs, row pitch `[41,41,74,74,148,41,74,74]`.

## C. Read-only production baseline

| Item | Observed |
| --- | --- |
| `systemctl is-active new-api` | `active` |
| ActiveEnterTimestamp | `Wed 2026-10-07 15:07:36 CST` |
| `journalctl -u new-api -p err --since <ts> -q` count | **0** entries |
| PID owning 4002 | `1764190` (`new-api`) |
| live `/opt/new-api/new-api` sha256 | `7372604e4112ea978aac0835a91bb5195de7155b67290f94e782f1171ff0d57c` = rc01 manifest `BINARY_SHA256` |
| `/opt/new-api` HEAD / branch | `583e8ddf9c09f816e90be4e04d936e15b65d07b8` / `prod/251` |
| working tree | ` M watchdog.sh` (pre-existing local modification, no other changes) |
| release dirs | 7; finalized (`finalized.env`): claude-effort-rc01, rc40, usage-log-refresh-rc01, routing-ratio-rc01. Not finalized: model-health-rc01 (no runtime), routing-allocation-card-rc01 (candidate dead), **routing-allocation-card-rc02 (this candidate)** |
| listeners | Among new-api ports only `4002` (pid 1764190) and `4003` (pid 1887027). The host also runs unrelated services on 22/53/8000/8081/8090/…; no other new-api listener. |

No API calls were made to 4002; no production file or DB was written.

## Deviations / assumptions (recorded, candidate-only)

1. **Sandbox account was missing.** The rc02 candidate DB (a fresh production copy) contained only the `admin` user; `visual-qa` (expected at `user_id=2`) was absent, so the mandatory browser check could not log in. I seeded the documented sandbox account into the **candidate disposable runtime DB only** (`INSERT ... username='visual-qa', role=100, status=1`, same hash as the rc01 sandbox flow), then verified `POST /api/user/login` → 200 `success=true`. Production DB/files were not touched. This mirrors the brief's own candidate-only session remediation.
2. **"0 console errors"** is confirmed for the card page. Viewports 2–3 of the main harness: 0 console errors / 0 failed requests. The first (login) viewport records the same two pre-login/external errors as the rc01 baseline (`/api/user/auth/refresh` 401 and the GitHub releases 403); neither is a card error. The supplementary `console-clean-check.cjs` probe resets the collectors after login and then measures the card route: **0 console errors, 0 failed requests** (`console-clean-check.json`).
3. The manifest's `LOCAL_OPTION_OVERRIDE_HELPER_SHA256` (`d6da0df8…`) matches the source-tree helper at commit `6c2ae0d56`, not the host's `/opt/new-api/patches/apply-local-option-overrides.py` (`74c56d78…`, the instance's own copy). This is expected: the manifest describes build inputs. The manifest's `LOCAL_OPTION_OVERRIDES_SHA256` matches the host file exactly.

## Next safe action

Candidate verify is green for `2026-10-07-routing-allocation-card-rc02`. Next safe action is `cutover` **only after explicit human confirmation in the coordinator thread** (action contract: successful recent verify + explicit operator confirmation). Do not chain verify into cutover. The 4003 candidate is left running and untouched; no dev server, worktree, or host process was left behind.
