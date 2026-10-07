# Baseline defect inventory — model-routing allocation card

- Target: candidate `http://10.0.0.251:4003` (release `2026-10-07-routing-allocation-card-rc01`)
- Harness: `ops/visual/model-weights-verify.cjs` (see `ops/visual/README.md`)
- Run label: `rc01-baseline`, artifacts in `ops/visual/out/rc01-baseline/`
- Viewports: `1440x900`, `1920x1080`, `390x844`; locale `en`, light theme
- Data: 2 model cards (`deepseek-v4.1-flash` = 5 rows, `glm-5.3-flash` = 4 rows),
  3 priority tiers per card (`500`, `497`, `496`), 9 share inputs total.
- Every number below is read from `report.json` (captured with `page.evaluate`
  against the live DOM); none is estimated from a screenshot.

## Summary

| # | Defect | Severity | Headline measurement |
| - | ------ | -------- | -------------------- |
| D1 | Huge dead space between the channel name and the share control | high | 963–1003 px empty at 1440, 1443–1483 px at 1920 (grid gap is only 8 px) |
| D2 | Tier header is visually weaker than the rows it groups | medium | `Priority N` visual weight 37,740 vs row name 110,880 (0.34×) and model title 138,600 (0.27×) |
| D3 | Row pitch is inconsistent inside a card | low | 40 px within a tier, 68 px after a tier header, 155 px between cards (spread 115 px) |
| D4 | Mobile stacks name and share control | info | 9/9 rows stacked at 390x844; row height 60 px, share input full width (324.94 px) |
| D5 | Console/network noise on first load | info | `POST /api/user/auth/refresh` 401, `GET api.github.com/.../releases` 403 |

Non-defects verified (must stay true after the fix): the numeric columns are
already aligned — see "Numeric alignment" below.

---

## D1 — Dead space between channel name and share control (high)

The row is `div.grid.items-center.gap-2.sm:grid-cols-[minmax(0,1fr)_6rem]`.
The channel-name grid item stretches to the full `1fr` column, while the names
are only 66.77–106.97 px wide, so almost the whole name column is empty before
the share input starts at the far right edge of the card.

Measured `deadSpacePx = shareCell.left − nameGlyphRect.right`
(visible text right edge → share column left edge):

| Viewport | n | min | median | max |
| -------- | - | --- | ------ | --- |
| 1440x900 | 9 | 963.03 | 971.95 | 1003.23 |
| 1920x1080 | 9 | 1443.03 | 1451.95 | 1483.23 |
| 390x844 | 0 | — | — | — (rows stacked, see D4) |

Supporting DOM numbers at 1440x900:
- name cell width `1299 − 237 = 1062 px`; name glyph widths `[66.77, 75.44, 98.05, 106.97, 106.97, 66.77, 75.44, 106.97, 106.97]`.
- share column starts at `x = 1307` and is `80.94 px` wide (input) with the `%` sign after it.
- the actual CSS grid gap (`shareCell.left − nameCell.right`) is exactly `8 px`
  on all 9 rows — so the emptiness is not the grid gap, it is the stretched
  name column.

At 1920x1080 the dead space grows to ~1452 px median because the `1fr` column
absorbs the extra width; the layout has no maximum width or content-based
sizing for the name column.

## D2 — Tier header is visually weaker than the rows it groups (medium)

Computed styles (identical at all three viewports), read from the live DOM:

| Element | font-size | font-weight | color (computed) | contrast on bg | composite visual weight |
| ------- | --------- | ----------- | ---------------- | -------------- | ----------------------- |
| Tier header `Priority 500` | 12 px | 500 | `lab(40.84 …)` muted | 6.29:1 | **37,740** |
| Row channel name | 14 px | 400 | `lab(2.75 …)` near-black | 19.8:1 | **110,880** |
| Card model title | 14 px | 500 | `lab(2.75 …)` near-black | 19.8:1 | **138,600** |

`visualWeight = fontSize × fontWeight × contrast` (relative comparison only).

- tier / row-name = **0.34**
- tier / model-title = **0.27**

So the section header that separates priority tiers is about one third as
visually prominent as the plain row labels inside it and about a quarter as
prominent as the card title. The tier boundary is hard to scan.

## D3 — Row pitch is inconsistent inside a card (low)

Measured vertical distance between consecutive row tops (`rowPitchPx`), all 9 rows:

| Viewport | values (px) | median | spread |
| -------- | ----------- | ------ | ------ |
| 1440x900 | 40, 40, 68, 68, 155, 40, 68, 68 | 68 | 115 |
| 1920x1080 | 40, 40, 68, 68, 155, 40, 68, 68 | 68 | 115 |
| 390x844 | 68, 68, 96, 96, 247, 68, 96, 96 | 96 | 179 |

- Within a tier: pitch = 40 px (row height 32 px + 8 px `space-y-2`).
- Across a tier header: pitch = 68 px (the header line adds 28 px).
- Across two cards: pitch = 155 px (card border, header and `space-y-4`).
- Row height itself is uniform: exactly 32 px on all 9 desktop rows, 60 px on
  all 9 mobile rows.

## D4 — Mobile stacks name and share control (info, preserve-or-intend)

At `390x844` the `sm:grid-cols-[minmax(0,1fr)_6rem]` rule does not apply, so
every row becomes single-column: 9/9 rows report `stackedRows` (name box above
the share box). Row height becomes 60 px and the share input expands to
324.94 px starting at `x = 25` (the `%` sign is pinned to the right edge).
There is no horizontal scroll at any viewport
(`scrollWidth == clientWidth`: 1440/1440, 1920/1920, 390/390).

This is a layout mode, not necessarily a defect — but any fix must decide
explicitly whether the mobile stacking is intended.

## D5 — Console / failed-request noise on first load (info)

Captured on the first viewport only (`report.json → reports[0]`); viewports 2
and 3 had 0 console errors and 0 failed requests:

| Kind | Method | URL | Status |
| ---- | ------ | --- | ------ |
| console error + response | POST | `http://10.0.0.251:4003/api/user/auth/refresh` | 401 (`AUTH_UNAUTHORIZED`) |
| console error + response | GET | `https://api.github.com/repos/QuantumNous/new-api/releases?per_page=100` | 403 (GitHub rate limit) |

Both are pre-login / external and unrelated to the allocation card. They are
recorded here so a later run can be diffed against this baseline instead of
treating them as a regression.

---

## Numeric alignment (verified non-defects)

All numeric inputs were measured by left edge and width; `aligned = true`
means both spreads are ≤ 0.5 px.

| Column | 1440x900 | 1920x1080 | 390x844 |
| ------ | -------- | --------- | ------- |
| Share (9 inputs) | left 1307, w 80.94 | left 1787, w 80.94 | left 25, w 324.94 |
| Advanced Priority (9) | left 237, w 579 | left 237, w 819 | left 25, w 340 |
| Advanced Weight (9) | left 824, w 579 | left 1064, w 819 | left 25, w 340 |

- `shareAlignment.aligned = true`, `advancedPriorityAlignment.aligned = true`,
  `advancedWeightAlignment.aligned = true` at every viewport (leftSpread = 0,
  widthSpread = 0).
- Numbers are `text-right` and the trailing `%` is a separate span; both are
  consistent across rows. **Do not regress this while fixing D1–D3.**

---

## Behavior baseline (PATCH payload for one weight edit)

Deterministic edit performed on the first viewport after screenshots:
set `Share deepseek-v4.1-flash #9` from `77` → `78`, click **Save Changes**.

```
PATCH http://10.0.0.251:4003/api/option/request_policy
status: 200

{"options":{"model_weight_setting.weights":"[{\"channel_id\":9,\"model\":\"deepseek-v4.1-flash\",\"weight\":78,\"priority\":500},{\"channel_id\":47,\"model\":\"deepseek-v4.1-flash\",\"weight\":11,\"priority\":500},{\"channel_id\":46,\"model\":\"deepseek-v4.1-flash\",\"weight\":11,\"priority\":500},{\"channel_id\":9,\"model\":\"glm-5.3-flash\",\"weight\":90,\"priority\":500},{\"channel_id\":46,\"model\":\"glm-5.3-flash\",\"weight\":10,\"priority\":500},{\"channel_id\":36,\"model\":\"deepseek-v4.1-flash\",\"weight\":100},{\"channel_id\":21,\"model\":\"deepseek-v4.1-flash\",\"weight\":100}]"}}
```

Decoded `model_weight_setting.weights`:

```json
[{"channel_id":9,"model":"deepseek-v4.1-flash","weight":78,"priority":500},
 {"channel_id":47,"model":"deepseek-v4.1-flash","weight":11,"priority":500},
 {"channel_id":46,"model":"deepseek-v4.1-flash","weight":11,"priority":500},
 {"channel_id":9,"model":"glm-5.3-flash","weight":90,"priority":500},
 {"channel_id":46,"model":"glm-5.3-flash","weight":10,"priority":500},
 {"channel_id":36,"model":"deepseek-v4.1-flash","weight":100},
 {"channel_id":21,"model":"deepseek-v4.1-flash","weight":100}]
```

Observations for the follow-up behavior-equality check:

- Editing one share **redistributes derived weights inside the same priority
  tier**: `77 → 78` for channel 9 changed channel 46 from `weight 12` to
  `weight 11` so the tier still totals 100. The tier `500` weights before the
  edit were `77 / 11 / 12`; after the edit `78 / 11 / 11`.
- The `--restore` save (back to `77`) issued the exact inverse payload and
  returned channel 46 to `weight 12`, so the candidate DB was left unchanged.
- Determinism: three independent harness runs (`out/rc01-baseline`,
  `out/verify-harness`, and an earlier scratch run) produced **byte-identical**
  edit and restore PATCH bodies (entry 0 and entry 1 compare equal across all
  runs). A follow-up run on the fix branch must reproduce the same bodies for
  the same edit.

## Artifacts

- `report.json` — full DOM measurements, console/network captures, PATCH bodies.
- `patch-payloads.json` — the two PATCH payloads (edit + restore).
- `rc01-baseline-1440x900-{viewport,full,advanced-full}.png`
- `rc01-baseline-1920x1080-{viewport,full,advanced-full}.png`
- `rc01-baseline-390x844-{viewport,full,advanced-full}.png`
