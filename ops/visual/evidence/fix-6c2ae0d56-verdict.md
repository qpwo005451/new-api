# Fix verification — `qpwo005451/ui-card-rows` @ `6c2ae0d56`

- Verified commit: `6c2ae0d56330556d695bd88ded0616485687cc4a` (detached scratch worktree
  `/home/ra/orca/workspaces/Newapi/ui-fix-verify`, single commit, touches only
  `model-weight-group.tsx` and `model-weight-section.test.tsx`).
- Under test: dev server `http://127.0.0.1:3000` (`env -u NODE_ENV
  VITE_REACT_APP_SERVER_URL=http://10.0.0.251:4003 bun run dev`), proxying `/api`
  to the staged candidate `:4003`.
- Harness: `ops/visual/model-weights-verify.cjs` (unchanged baseline harness).
- Fix artifacts: `ops/visual/out/fix-6c2ae0d56/` (label `fix-6c2ae0d56`).
- Baseline artifacts: `ops/visual/out/rc01-baseline/` (label `rc01-baseline`).
- All numbers are DOM measurements from `report.json` (`page.evaluate`), never
  eyeballed. **Limitation: this model cannot view images.** Screenshots were
  validated as non-blank (2809–2992 distinct colors each) and differ from the
  baseline (RMSE 8.4 % @1440, 7.0 % @1920, 14.7 % @390), but the visual
  judgement below rests on the DOM measurements, not on looking at the PNGs.

## Verdicts

| # | Defect | Verdict | Key numbers |
| - | ------ | ------- | ----------- |
| D1 | Dead space name ↔ share | **FIXED** | 1440 median 971.95 → 20.92 px; 1920 median 1451.95 → 20.92 px |
| D2 | Tier header weaker than rows | **FIXED** | ratio tier/row 0.34 → 1.50 (tier 166,320 vs row 110,880) |
| D3 | Row pitch consistency | **PARTIALLY FIXED** | spread 115 → 107 px (desktop), 179 → 171 (mobile); within-tier pitch uniform 41 px |
| D4 | Mobile stacking at 390x844 | **FIXED** | stacked rows 9/9 → 0/9; row height 60 → 40–41 px |
| D5 | Console/network noise | reported | first viewport only; same 401 refresh, GitHub request aborts in dev |
| NR | Numeric column alignment | **PASS** | share/priority/weight left+width spread = 0 at all 3 viewports |
| BQ | Behavior equality | **PASS** | both PATCH bodies byte-identical to baseline, status 200 |

## D1 — dead space (FIXED)

`deadSpacePx = shareCell.left − nameGlyphRect.right` (visible name text → share control):

| Viewport | baseline median (min–max) | fix median (min–max) |
| -------- | ------------------------- | -------------------- |
| 1440x900 | 971.95 (963.03–1003.23) | **20.92 (12–52.2)** |
| 1920x1080 | 1451.95 (1443.03–1483.23) | **20.92 (12–52.2)** |
| 390x844 | n/a (rows stacked) | **20.92 (12–52.2)** |

Cause is removed: the name column is now `fit-content(14rem)` and hugs the
longest name — name cell width `1062 px → 118.97 px` @1440 and
`1542 px → 118.97 px` @1920, identical at 390. No name is truncated
(longest glyph 106.97 px + 12 px `ps-3` < 118.97 px). The remaining 12–52 px is
the intentional `gap-x-3` (12 px) plus per-name slack, not dead layout space.

## D2 — tier-header visual weight (FIXED)

| Element | baseline | fix |
| ------- | -------- | --- |
| Tier header `Priority N` | 12 px / weight 500 / muted, contrast 6.29 → **37,740** | 14 px / weight **600** / near-black, contrast 19.8 → **166,320** |
| Row channel name | 14 px / weight 400 / near-black → 110,880 | unchanged → 110,880 |
| Card model title | 14 px / weight 500 → 138,600 | unchanged → 138,600 |

- `tier / row-name` = **0.34 → 1.50**; `tier / model-title` = 0.27 → **1.20**.
- The tier header is now stronger than the rows it groups, and the new tier band
  also has a `bg-muted/50` full-width background and a role badge, neither of
  which the span-only metric counts.

## D3 — row pitch (PARTIALLY FIXED)

`rowPitchPx` between consecutive row tops:

| Viewport | baseline | fix |
| -------- | -------- | --- |
| 1440x900 | 40, 40, 68, 68, **155**, 40, 68, 68 (spread 115) | 41, 41, 74, 74, **148**, 41, 74, 74 (spread 107) |
| 1920x1080 | same as 1440 | same as 1440 |
| 390x844 | 68, 68, 96, 96, **247**, 68, 96, 96 (spread 179) | 41, 41, 74, 74, **212**, 41, 74, 74 (spread 171) |

- Within-tier pitch is now uniform at **41 px** on every row (was 40 px); row
  height is 40–41 px (was 32 px), so rows are roomier and internally even.
- The remaining 41 / 74 / 148 variation is the deliberate tier band (+33 px)
  and the card boundary, not inconsistent row spacing. Raw spread only fell
  115 → 107 px (desktop), so this is recorded as *improved, not numerically
  eliminated*.

## D4 — mobile single-line rows (FIXED)

At `390x844`:

| | baseline | fix |
| - | -------- | --- |
| `stackedRowCount` | 9 / 9 | **0 / 9** |
| row height | 60 px | **40–41 px** |
| share control | full-width 324.94 px @ x 25 | 73.09 px @ x 144.97 (inside the row) |
| horizontal scroll | none | none (`scrollWidth == clientWidth == 390`) |

All nine rows keep the channel name and share control on one line; the grid is
`[fit-content(50%)_6rem_minmax(0,1fr)]` at mobile widths.

## D5 — console / failed-request noise (reported)

- 1440x900 (first/login viewport): 1 console error + 3 failed requests —
  `POST /api/user/auth/refresh` → 401 (same pre-login noise as baseline) and two
  `GET https://api.github.com/repos/QuantumNous/new-api/releases?per_page=100`
  → `net::ERR_ABORTED` (external; dev double-invoke, baseline saw 403 here).
- 1920x1080 and 390x844: 0 console errors, 0 failed requests.
- No card-related console error or failed request.

## Non-regression — numeric column alignment (PASS)

`aligned = leftSpread ≤ 0.5 && widthSpread ≤ 0.5`:

| Column | 1440x900 | 1920x1080 | 390x844 |
| ------ | -------- | --------- | ------- |
| Share (9) | aligned, spread 0 | aligned, spread 0 | aligned, spread 0 |
| Advanced Priority (9) | aligned, spread 0 | aligned, spread 0 | aligned, spread 0 |
| Advanced Weight (9) | aligned, spread 0 | aligned, spread 0 | aligned, spread 0 |

Absolute positions changed with the redesign (share moved from x≈1307 to
x≈356.97 @1440; priority/weight columns now sit after the name column), but
every role's 9 inputs share an identical left edge and width at every viewport.
No horizontal scroll at any viewport.

## Behavior equality (PASS — byte-identical)

Same edit as baseline: `Share deepseek-v4.1-flash #9` `77 → 78`, **Save Changes**,
then `--restore` back to `77`.

| | baseline (`:4003`) | fix (`127.0.0.1:3000` → `:4003`) |
| - | ------------------ | -------------------------------- |
| edit PATCH `postData` | `…"weight":78…` | **byte-identical** |
| restore PATCH `postData` | `…"weight":77…` | **byte-identical** |
| full parsed `json` body | — | identical for both entries |
| response status | 200 / 200 | 200 / 200 |
| edit result | original 77, target 78, saved 78, restored 77 | identical |

Decoded edit body (identical in both runs):

```json
{"options":{"model_weight_setting.weights":"[{\"channel_id\":9,\"model\":\"deepseek-v4.1-flash\",\"weight\":78,\"priority\":500},{\"channel_id\":47,\"model\":\"deepseek-v4.1-flash\",\"weight\":11,\"priority\":500},{\"channel_id\":46,\"model\":\"deepseek-v4.1-flash\",\"weight\":11,\"priority\":500},{\"channel_id\":9,\"model\":\"glm-5.3-flash\",\"weight\":90,\"priority\":500},{\"channel_id\":46,\"model\":\"glm-5.3-flash\",\"weight\":10,\"priority\":500},{\"channel_id\":36,\"model\":\"deepseek-v4.1-flash\",\"weight\":100},{\"channel_id\":21,\"model\":\"deepseek-v4.1-flash\",\"weight\":100}]"}}
```

The edit still redistributes derived weights inside the priority tier
(channel 46 `12 → 11`) and the restore returns it to `12`, so preset/lock
semantics are unchanged. As corroboration, the commit's own test file passes:
`vitest run …/model-weight-section.test.tsx` → **18/18 passed**.

## Artifacts

- `report.json`, `patch-payloads.json`
- `fix-6c2ae0d56-1440x900-{viewport,full,advanced-full}.png`
- `fix-6c2ae0d56-1920x1080-{viewport,full,advanced-full}.png`
- `fix-6c2ae0d56-390x844-{viewport,full,advanced-full}.png`
