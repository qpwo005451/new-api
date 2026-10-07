# Model-routing allocation card — visual verification harness

`model-weights-verify.cjs` is an independent, read-only visual-verification
harness for the **Model Routing** allocation card on
`/system-settings/request-policies/model-weights`. It drives a real Chromium
through the real login form, navigates to the route, waits for the card to be
populated, captures screenshots, and records layout measurements and the exact
PATCH/PUT body the page sends when a weight is edited.

It never edits product code. Its only writes are the screenshots and reports it
is asked to produce.

## Requirements

- Node.js (tested with v24.19.0).
- Playwright from the shared global install:
  `/home/ra/.nvm/versions/node/v24.19.0/lib/node_modules/playwright`
  (override with `PLAYWRIGHT_MODULE`).
- Chromium already downloaded under `~/.cache/ms-playwright`.
- No `bun install` / `web/node_modules` is needed: the script is standalone and
  drives a remote (or locally served) build.

## Usage

```sh
node ops/visual/model-weights-verify.cjs \
  --base-url http://10.0.0.251:4003 \
  --viewport 1440x900 --viewport 1920x1080 --viewport 390x844 \
  --username visual-qa \
  --password-file /tmp/orca-recov/visual_login.txt \
  --label rc01-baseline \
  --out-dir ops/visual/out/rc01-baseline \
  --edit-first-share +1 --restore --expand-advanced
```

### Inputs

| Flag | Default | Meaning |
| --- | --- | --- |
| `--base-url <url>` | `$BASE_URL` or `http://10.0.0.251:4003` | App under test. |
| `--route <path>` | `/system-settings/request-policies/model-weights` | Route to render. |
| `--viewport <WxH>` | `1440x900` | Repeatable. One measurement per viewport. |
| `--username <name>` | `$VIS_USER` or `visual-qa` | UI login user. |
| `--password <pass>` | `$VIS_PASS` | UI login password. |
| `--password-file <file>` | — | File with a bare password or `password=…`. |
| `--out-dir <dir>` | `ops/visual/out/<label>` | Artifact directory (created). |
| `--label <name>` | `run` | Artifact filename prefix. |
| `--timeout <ms>` | `60000` | Selector/navigation timeout. |
| `--nav-attempts <n>` | `4` | Retries when a navigation returns 429/blank. |
| `--max-retry-wait <ms>` | `190000` | Cap on `Retry-After` backoff. |
| `--between-viewports <ms>` | `2000` | Pause between viewports. |
| `--edit-first-share <n>` | off | After screenshots, set the first share input to `n` (or `+n`/`-n`) and click Save, capturing the PATCH/PUT body. |
| `--restore` | off | After the edit, set the original value back and save again, so the candidate DB is left unchanged. |
| `--expand-advanced` | off | Also click every **Advanced** toggle, measure/screenshot the expanded rows. |
| `--no-screenshots` | off | Metrics/report only. |
| `--headed` | off | Run Chromium headed. |

Credentials are never written to disk; only the artifact directory is.

## Outputs

For each viewport `WxH` and label `L`:

- `L-WxH-viewport.png` — viewport screenshot (collapsed card).
- `L-WxH-full.png` — full-page screenshot (collapsed card).
- `L-WxH-advanced-full.png` — full-page screenshot with Advanced expanded.
- `report.json` — full machine-readable run: per-viewport measurements,
  console errors, failed requests (`status >= 400` and `requestfailed`), and
  every PATCH/PUT request with its parsed JSON body and response.
- `patch-payloads.json` — flat list of the captured PATCH/PUT requests.

`report.json` is the source of truth; the numbers below are read from it.

## What is measured (from the live DOM via `page.evaluate`)

- `rowHeightPx` / `rowPitchPx` — bounding-box height of each row and the
  vertical distance between consecutive row tops.
- `columnGapPx` — `shareCell.left - nameCell.right` (the CSS grid gap).
- `deadSpacePx` — `shareCell.left - nameGlyphRect.right`, i.e. the empty space
  between the *visible* channel name and the share column. `nameGlyphRect` is a
  `Range.selectNodeContents` box, so it is text width, not cell width.
- `shareAlignment` / `advancedPriorityAlignment` / `advancedWeightAlignment` —
  spread of the left edges and widths of every numeric input, per role.
- `tierHeader` / `rowName` / `modelTitle` — font size, weight, computed sRGB
  color, WCAG contrast on the effective background, and a composite
  `visualWeight = fontSize × fontWeight × contrast` used only for relative
  comparison.
- `page.hasHorizontalScroll`, `shareCount`, card/row/tier counts.

## Notes

- **Rate limiting:** the candidate answers HTML/static assets with
  `429 + Retry-After: 180` when hammered. The harness logs in once, reuses one
  browser context for every viewport (only `setViewportSize` changes), and
  retries a 429/blank navigation using `Retry-After`. Keep runs spaced out.
- **Candidate only:** the `visual-qa` credential exists only in the disposable
  candidate DB. Never point this at production (`:4002`).
- **Behavior equality:** `--edit-first-share +1 --restore` produces a
  deterministic edit PATCH and a restore PATCH. Comparing
  `patch-payloads.json` across branches is the behavior-equality check.
- **Clean console-error window:** `console-clean-check.cjs` logs in through the
  real UI, resets the console/failed-request collectors after login, then
  navigates to the allocation card and reports errors for the card page only
  (the main harness also counts the pre-login `/api/user/auth/refresh` 401 and
  the external GitHub release check). Exit 0 means 0 errors and 0 failed
  requests. Usage:
  `node ops/visual/console-clean-check.cjs --base-url <url> --width 1440 --height 900`.
