#!/usr/bin/env node
/*
 * Independent visual-verification harness for the model-routing allocation card.
 *
 * Owned by the ui-render-verify worker. Read-only with respect to product code:
 * this script only drives a browser, it never writes to web/src/**.
 *
 * It logs in through the real UI, navigates to the model-weights route, waits
 * for the allocation card to be populated, saves viewport + full-page
 * screenshots, records console errors / failed requests / PATCH+PUT bodies
 * machine-readably, and measures the card layout from the live DOM.
 *
 * Usage:
 *   node ops/visual/model-weights-verify.cjs \
 *     --base-url http://10.0.0.251:4003 \
 *     --viewport 1440x900 --viewport 1920x1080 --viewport 390x844 \
 *     --username visual-qa --password-file /tmp/orca-recov/visual_login.txt \
 *     --label rc01-baseline --out-dir ops/visual/out/rc01-baseline
 *
 * See ops/visual/README.md for the full input/output contract.
 */
'use strict';

const fs = require('fs');
const path = require('path');

const PLAYWRIGHT_MODULE =
  process.env.PLAYWRIGHT_MODULE ||
  '/home/ra/.nvm/versions/node/v24.19.0/lib/node_modules/playwright';
const { chromium } = require(PLAYWRIGHT_MODULE);

const DEFAULTS = {
  baseUrl: process.env.BASE_URL || 'http://10.0.0.251:4003',
  route: '/system-settings/request-policies/model-weights',
  viewports: ['1440x900'],
  username: process.env.VIS_USER || 'visual-qa',
  password: process.env.VIS_PASS || '',
  passwordFile: '',
  outDir: '',
  label: 'run',
  timeout: 60000,
  navAttempts: 4,
  maxRetryWait: 190000,
  betweenViewports: 2000,
  shareLabelPrefix: 'Share ',
  saveLabel: 'Save Changes',
  editFirstShare: null,
  restore: false,
  expandAdvanced: false,
  headed: false,
  noScreenshots: false,
};

function parseArgs(argv) {
  const opts = { viewports: [] };
  const need = (i, flag) => {
    if (i + 1 >= argv.length) throw new Error(`missing value for ${flag}`);
    return argv[i + 1];
  };
  for (let i = 0; i < argv.length; i += 1) {
    const a = argv[i];
    switch (a) {
      case '--base-url': opts.baseUrl = need(i, a); i += 1; break;
      case '--route': opts.route = need(i, a); i += 1; break;
      case '--viewport': opts.viewports.push(need(i, a)); i += 1; break;
      case '--username': opts.username = need(i, a); i += 1; break;
      case '--password': opts.password = need(i, a); i += 1; break;
      case '--password-file': opts.passwordFile = need(i, a); i += 1; break;
      case '--out-dir': opts.outDir = need(i, a); i += 1; break;
      case '--label': opts.label = need(i, a); i += 1; break;
      case '--timeout': opts.timeout = parseInt(need(i, a), 10); i += 1; break;
      case '--nav-attempts': opts.navAttempts = parseInt(need(i, a), 10); i += 1; break;
      case '--max-retry-wait': opts.maxRetryWait = parseInt(need(i, a), 10); i += 1; break;
      case '--between-viewports': opts.betweenViewports = parseInt(need(i, a), 10); i += 1; break;
      case '--share-label-prefix': opts.shareLabelPrefix = need(i, a); i += 1; break;
      case '--save-label': opts.saveLabel = need(i, a); i += 1; break;
      case '--edit-first-share': opts.editFirstShare = need(i, a); i += 1; break;
      case '--restore': opts.restore = true; break;
      case '--expand-advanced': opts.expandAdvanced = true; break;
      case '--headed': opts.headed = true; break;
      case '--no-screenshots': opts.noScreenshots = true; break;
      case '--help': case '-h': opts.help = true; break;
      default:
        throw new Error(`unknown argument: ${a}`);
    }
  }
  return { ...DEFAULTS, ...opts, viewports: opts.viewports.length ? opts.viewports : DEFAULTS.viewports };
}

function usage() {
  return [
    'node model-weights-verify.cjs [options]',
    '',
    'Required (or env):',
    '  --base-url <url>            candidate base URL            (BASE_URL)',
    '  --username <name>           login username                (VIS_USER)',
    '  --password <pass>           login password                (VIS_PASS)',
    '  --password-file <file>      file containing the password; accepts a bare',
    '                              password or a line starting with password=',
    '',
    'Optional:',
    '  --route <path>              default /system-settings/request-policies/model-weights',
    '  --viewport <WxH>            repeatable; default 1440x900',
    '  --out-dir <dir>             default ops/visual/out/<label>',
    '  --label <name>              artifact prefix; default run',
    '  --timeout <ms>              navigation/selector timeout; default 60000',
    '  --nav-attempts <n>          navigation retries on blank/429 page; default 4',
    '  --max-retry-wait <ms>       cap for Retry-After backoff; default 190000',
    '  --between-viewports <ms>    pause between viewports; default 2000',
    '  --edit-first-share <n>      after screenshots: set first share input to n,',
    '                              click Save, capture the PATCH/PUT body. Also',
    '                              accepts a relative step: "+5" or "-5".',
    '  --restore                   after the edit, set the original value back and',
    '                              save again so the candidate DB is unchanged',
    '  --expand-advanced           also measure/screenshot the Advanced panel',
    '  --no-screenshots            skip PNG output (metrics/report only)',
    '  --headed                    run Chromium headed',
    '  --help                      print this help',
  ].join('\n');
}

function readPassword(opts) {
  if (opts.password) return opts.password;
  if (opts.passwordFile) {
    const raw = fs.readFileSync(opts.passwordFile, 'utf8');
    for (const line of raw.split(/\r?\n/)) {
      const m = line.match(/^\s*(?:password|pass)\s*[=:]\s*(.+?)\s*$/i);
      if (m) return m[1];
    }
    const first = raw.split(/\r?\n/).find((l) => l.trim() !== '');
    if (first) return first.trim();
  }
  throw new Error('no password supplied: use --password, --password-file or VIS_PASS');
}

function parseViewport(spec) {
  const m = String(spec).match(/^(\d+)x(\d+)$/);
  if (!m) throw new Error(`bad viewport "${spec}", expected WxH`);
  return { width: parseInt(m[1], 10), height: parseInt(m[2], 10), spec: `${m[1]}x${m[2]}` };
}

const round = (n, d = 2) => (Number.isFinite(n) ? +n.toFixed(d) : n);

function stats(values) {
  const clean = values.filter((v) => Number.isFinite(v));
  if (clean.length === 0) return { count: 0, min: null, max: null, median: null, spread: null, values: [] };
  const sorted = [...clean].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  const median = sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
  return {
    count: clean.length,
    min: round(sorted[0]),
    max: round(sorted[sorted.length - 1]),
    median: round(median),
    spread: round(sorted[sorted.length - 1] - sorted[0]),
    values: clean.map((v) => round(v)),
  };
}

/*
 * In-page measurement. Serialized into the browser, so it must not close over
 * any Node variable. `prefix` is the aria-label prefix for the share inputs.
 */
function measureInPage({ prefix }) {
  const R = (el) => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return {
      x: +r.x.toFixed(2), y: +r.y.toFixed(2), w: +r.width.toFixed(2), h: +r.height.toFixed(2),
      right: +r.right.toFixed(2), bottom: +r.bottom.toFixed(2),
    };
  };
  const glyphRect = (el) => {
    if (!el) return null;
    try {
      const range = document.createRange();
      range.selectNodeContents(el);
      const r = range.getBoundingClientRect();
      return { x: +r.x.toFixed(2), right: +r.right.toFixed(2), w: +r.width.toFixed(2), h: +r.height.toFixed(2) };
    } catch (e) { return null; }
  };
  // Chromium reports computed colors for Tailwind v4 tokens as lab()/oklab(),
  // which the old regex parser could not read (contrast came back null). Paint
  // the CSS color into a 1x1 canvas and read the sRGB pixel instead, so contrast
  // and the composite "visual weight" are real numbers at every viewport.
  const colorCanvas = document.createElement('canvas');
  colorCanvas.width = 1;
  colorCanvas.height = 1;
  const colorCtx = colorCanvas.getContext('2d', { willReadFrequently: true });
  const parseRgbFallback = (value) => {
    const m = String(value).match(/rgba?\(([^)]+)\)/);
    if (!m) return null;
    const parts = m[1].split(/[,\s/]+/).map((p) => parseFloat(p));
    return { r: parts[0], g: parts[1], b: parts[2], a: parts.length > 3 ? parts[3] : 1 };
  };
  const toRgba = (value) => {
    if (!value) return null;
    try {
      colorCtx.clearRect(0, 0, 1, 1);
      colorCtx.fillStyle = '#000000';
      colorCtx.fillStyle = value;
      colorCtx.fillRect(0, 0, 1, 1);
      const d = colorCtx.getImageData(0, 0, 1, 1).data;
      return { r: d[0], g: d[1], b: d[2], a: +(d[3] / 255).toFixed(4) };
    } catch (e) {
      return parseRgbFallback(value);
    }
  };
  const luminance = ({ r, g, b }) => {
    const f = (v) => { const s = v / 255; return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4); };
    return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
  };
  const contrastRatio = (fg, bg) => {
    const l1 = luminance(fg), l2 = luminance(bg);
    const hi = Math.max(l1, l2), lo = Math.min(l1, l2);
    return +((hi + 0.05) / (lo + 0.05)).toFixed(2);
  };
  const effectiveBackground = (el) => {
    let node = el;
    while (node) {
      const rgb = toRgba(getComputedStyle(node).backgroundColor);
      if (rgb && rgb.a > 0.05) return rgb;
      node = node.parentElement;
    }
    return { r: 255, g: 255, b: 255, a: 1 };
  };
  const textStyle = (el, bg) => {
    if (!el) return null;
    const cs = getComputedStyle(el);
    const color = toRgba(cs.color);
    const fontWeight = parseInt(cs.fontWeight, 10) || 400;
    const fontSize = parseFloat(cs.fontSize) || 0;
    return {
      fontSizePx: fontSize,
      fontWeight,
      lineHeight: cs.lineHeight,
      color: cs.color,
      colorRgb: color ? `rgb(${color.r}, ${color.g}, ${color.b})` : null,
      contrast: color && bg ? contrastRatio(color, bg) : null,
      visualWeight: color && bg ? +(fontWeight * fontSize * contrastRatio(color, bg)).toFixed(0) : +(fontWeight * fontSize).toFixed(0),
    };
  };

  const shareSelector = `input[aria-label^="${prefix.replace(/"/g, '\\"')}"]`;
  const shares = Array.from(document.querySelectorAll(shareSelector));
  const parseLabel = (inp) => {
    const label = inp.getAttribute('aria-label') || '';
    const m = label.match(new RegExp(`^${prefix.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(.*)\\s+#(\\d+)$`));
    return { label, model: m ? m[1] : null, channelId: m ? m[2] : null };
  };

  // Group share inputs by model parsed out of the aria-label.
  const groups = new Map();
  for (const inp of shares) {
    const parsed = parseLabel(inp);
    const key = parsed.model || '__all__';
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push({ inp, parsed });
  }

  const ancestors = (el) => {
    const list = [];
    let node = el;
    while (node && node !== document.body) { list.push(node); node = node.parentElement; }
    return list;
  };

  const findRow = (inp) => {
    const parent = inp.parentElement;
    let node = parent;
    let fallback = parent;
    while (node && node !== document.body) {
      const shareCount = node.querySelectorAll(shareSelector).length;
      if (shareCount !== 1) break;
      const candidates = Array.from(node.querySelectorAll('*')).filter((el) => {
        if (el.children.length > 0) return false;
        const t = (el.textContent || '').trim();
        if (!t || t === '%') return false;
        if (el.closest('button')) return false;
        if (parent && parent.contains(el)) return false; // exclude the input's own wrapper
        const r = el.getBoundingClientRect();
        return r.width > 0 && r.height > 0;
      });
      if (candidates.length > 0) return { row: node, nameEl: candidates[0], candidates };
      fallback = node;
      node = node.parentElement;
    }
    return { row: fallback, nameEl: null, candidates: [] };
  };

  const cards = [];
  for (const [model, entries] of groups) {
    const first = entries[0].inp;
    const groupSet = new Set(entries.map((e) => e.inp));
    // Outermost ancestor that holds this model's inputs and no other model's
    // inputs: the full card (border + header + rows), not just the row body.
    let card = null;
    for (const a of ancestors(first)) {
      if (a === first) continue;
      const ins = Array.from(a.querySelectorAll(shareSelector));
      const onlyThisModel = ins.length > 0 && ins.every((i) => groupSet.has(i));
      if (onlyThisModel && entries.every((e) => a.contains(e.inp))) card = a;
    }
    if (!card) card = first.parentElement;
    const bg = effectiveBackground(card);

    const rows = entries.map(({ inp, parsed }) => {
      const { row, nameEl } = findRow(inp);
      const rowChildren = Array.from(row.children || []);
      const shareCell = rowChildren.find((c) => c.contains(inp)) || inp.parentElement;
      let nameCell = nameEl && rowChildren.find((c) => c.contains(nameEl));
      if (!nameCell) nameCell = rowChildren.find((c) => c !== shareCell && c.getBoundingClientRect().width > 0) || nameEl;
      const pctEl = inp.parentElement ? Array.from(inp.parentElement.children).find((c) => c !== inp) : null;
      const advanced = Array.from(row.querySelectorAll('input')).filter((i) => i !== inp).map((i) => {
        const lbl = i.getAttribute('aria-label') || '';
        const role = /^Weight\b/i.test(lbl) ? 'weight' : /^Priority\b/i.test(lbl) ? 'priority' : 'other';
        return { role, label: lbl, rect: R(i) };
      });
      return {
        channelId: parsed.channelId,
        label: parsed.label,
        value: inp.value,
        rowRect: R(row),
        rowHeight: R(row) ? R(row).h : null,
        nameText: nameEl ? (nameEl.textContent || '').trim() : null,
        nameCellRect: R(nameCell),
        nameGlyphRect: glyphRect(nameEl),
        nameStyle: textStyle(nameEl, bg),
        shareCellRect: R(shareCell),
        shareInputRect: R(inp),
        pctRect: R(pctEl),
        advanced,
      };
    });

    const tierHeaders = Array.from(card.querySelectorAll('span'))
      .filter((s) => /^Priority\s+\d+$/.test((s.textContent || '').trim()))
      .map((s) => ({ text: (s.textContent || '').trim(), rect: R(s), style: textStyle(s, bg) }));
    const statuses = Array.from(card.querySelectorAll('span'))
      .filter((s) => /^(Participating|Fallback)$/.test((s.textContent || '').trim()))
      .map((s) => ({ text: (s.textContent || '').trim(), rect: R(s), style: textStyle(s, bg) }));

    // Card header: the model title is the first non-empty text element before
    // the first tier header / first row.
    const headerTexts = Array.from(card.querySelectorAll('span,p,div'))
      .filter((el) => el.children.length === 0 && (el.textContent || '').trim())
      .map((el) => ({ text: (el.textContent || '').trim(), rect: R(el), style: textStyle(el, bg) }))
      .filter((t) => t.rect && t.rect.h > 0);

    const titleEl = Array.from(card.querySelectorAll('span'))
      .find((s) => (s.textContent || '').trim() === model) || null;
    const title = titleEl
      ? { text: model, rect: R(titleEl), style: textStyle(titleEl, bg) }
      : null;

    cards.push({
      model,
      title,
      cardRect: R(card),
      cardClass: typeof card.className === 'string' ? card.className : null,
      rows,
      tierHeaders,
      statuses,
      headerTexts,
    });
  }

  const allShareRects = shares.map((i) => R(i));
  const doc = document.documentElement;

  return {
    url: location.href,
    title: document.title,
    lang: document.documentElement.lang || null,
    i18nextLng: (() => { try { return localStorage.getItem('i18nextLng'); } catch (e) { return null; } })(),
    viewport: { width: window.innerWidth, height: window.innerHeight },
    page: {
      scrollHeight: doc.scrollHeight,
      scrollWidth: doc.scrollWidth,
      clientWidth: doc.clientWidth,
      hasHorizontalScroll: doc.scrollWidth > doc.clientWidth,
    },
    shareCount: shares.length,
    shareInputs: allShareRects,
    cards,
  };
}

/*
 * Expand every "Advanced" toggle so the advanced numeric columns can be
 * measured too. Returns how many toggles were clicked.
 */
function expandAdvancedInPage() {
  const buttons = Array.from(document.querySelectorAll('button')).filter((b) =>
    /advanced/i.test(b.textContent || '') && b.getAttribute('aria-expanded') === 'false'
  );
  for (const b of buttons) b.click();
  return buttons.length;
}

function aggregate(metrics) {
  const rows = metrics.cards.flatMap((c) => c.rows);
  const share = stats(rows.map((r) => r.shareInputRect).filter(Boolean).map((r) => r.x));
  const shareWidth = stats(rows.map((r) => r.shareInputRect).filter(Boolean).map((r) => r.w));
  const shareLefts = rows.map((r) => r.shareInputRect).filter(Boolean).map((r) => r.x);
  const shareWidths = rows.map((r) => r.shareInputRect).filter(Boolean).map((r) => r.w);

  const rowHeights = stats(rows.map((r) => r.rowHeight).filter((v) => v !== null));
  const rowTops = rows.map((r) => r.rowRect && r.rowRect.y).filter((v) => Number.isFinite(v));
  const pitches = [];
  for (let i = 1; i < rowTops.length; i += 1) pitches.push(rowTops[i] - rowTops[i - 1]);

  // Column gap / dead space. Only meaningful when the name and share cells are
  // on the same visual line (i.e. not stacked on a narrow viewport).
  const columnGaps = [];
  const deadSpaces = [];
  let stackedRows = 0;
  for (const r of rows) {
    if (!r.nameCellRect || !r.shareCellRect) continue;
    const stacked = r.nameCellRect.bottom <= r.shareCellRect.y + 1;
    if (stacked) { stackedRows += 1; continue; }
    columnGaps.push(r.shareCellRect.x - r.nameCellRect.right);
    if (r.nameGlyphRect) deadSpaces.push(r.shareCellRect.x - r.nameGlyphRect.right);
  }

  const advancedRects = (role) => rows
    .flatMap((r) => r.advanced.filter((a) => a.role === role))
    .map((a) => a.rect)
    .filter(Boolean);

  const numericAlignment = (rects) => {
    if (rects.length < 2) return { count: rects.length, aligned: null, leftSpread: null, widthSpread: null, lefts: rects.map((r) => r.x), widths: rects.map((r) => r.w) };
    const s = stats(rects.map((r) => r.x));
    const ws = stats(rects.map((r) => r.w));
    return {
      count: rects.length,
      aligned: s.spread <= 0.5 && ws.spread <= 0.5,
      leftSpread: s.spread,
      widthSpread: ws.spread,
      lefts: s.values,
      widths: ws.values,
    };
  };

  const tierStyles = metrics.cards.flatMap((c) => c.tierHeaders.map((t) => t.style)).filter(Boolean);
  const nameStyles = rows.map((r) => r.nameStyle).filter(Boolean);
  const titleStyles = metrics.cards.map((c) => c.title && c.title.style).filter(Boolean);
  const tierWeight = tierStyles.length
    ? Math.round(tierStyles.reduce((a, s) => a + s.visualWeight, 0) / tierStyles.length)
    : null;
  const nameWeight = nameStyles.length
    ? Math.round(nameStyles.reduce((a, s) => a + s.visualWeight, 0) / nameStyles.length)
    : null;
  const titleWeight = titleStyles.length
    ? Math.round(titleStyles.reduce((a, s) => a + s.visualWeight, 0) / titleStyles.length)
    : null;

  return {
    cardCount: metrics.cards.length,
    rowCount: rows.length,
    shareInputCount: metrics.shareCount,
    rowHeightPx: rowHeights,
    rowPitchPx: stats(pitches),
    columnGapPx: stats(columnGaps),
    deadSpacePx: stats(deadSpaces),
    stackedRowCount: stackedRows,
    shareColumnLeftPx: share,
    shareColumnWidthPx: shareWidth,
    shareAlignment: numericAlignment(rows.map((r) => r.shareInputRect).filter(Boolean)),
    advancedPriorityAlignment: numericAlignment(advancedRects('priority')),
    advancedWeightAlignment: numericAlignment(advancedRects('weight')),
    tierHeader: {
      count: tierStyles.length,
      sampleText: metrics.cards[0] && metrics.cards[0].tierHeaders[0] ? metrics.cards[0].tierHeaders[0].text : null,
      fontSizePx: tierStyles[0] ? tierStyles[0].fontSizePx : null,
      fontWeight: tierStyles[0] ? tierStyles[0].fontWeight : null,
      color: tierStyles[0] ? tierStyles[0].color : null,
      contrast: tierStyles[0] ? tierStyles[0].contrast : null,
      visualWeight: tierWeight,
    },
    rowName: {
      count: nameStyles.length,
      fontSizePx: nameStyles[0] ? nameStyles[0].fontSizePx : null,
      fontWeight: nameStyles[0] ? nameStyles[0].fontWeight : null,
      color: nameStyles[0] ? nameStyles[0].color : null,
      contrast: nameStyles[0] ? nameStyles[0].contrast : null,
      visualWeight: nameWeight,
    },
    modelTitle: {
      count: titleStyles.length,
      fontSizePx: titleStyles[0] ? titleStyles[0].fontSizePx : null,
      fontWeight: titleStyles[0] ? titleStyles[0].fontWeight : null,
      color: titleStyles[0] ? titleStyles[0].color : null,
      contrast: titleStyles[0] ? titleStyles[0].contrast : null,
      visualWeight: titleWeight,
    },
    tierEmphasisRatioVsRowName: tierWeight && nameWeight ? +(tierWeight / nameWeight).toFixed(3) : null,
    tierEmphasisRatioVsModelTitle: tierWeight && titleWeight ? +(tierWeight / titleWeight).toFixed(3) : null,
  };
}

async function waitForStableShares(page, selector, timeout) {
  const deadline = Date.now() + timeout;
  let last = -1;
  let stableSince = 0;
  while (Date.now() < deadline) {
    const count = await page.locator(selector).count().catch(() => 0);
    if (count > 0 && count === last) {
      if (!stableSince) stableSince = Date.now();
      if (Date.now() - stableSince >= 800) return count;
    } else {
      stableSince = 0;
    }
    last = count;
    await page.waitForTimeout(200);
  }
  return last;
}

/*
 * Navigate, tolerating the candidate's global 429 rate limiter (which answers
 * static assets and HTML with `Retry-After: 180`). A 429 page renders blank, so
 * both the document status and an empty app root trigger a backoff + retry.
 */
async function gotoWithRetry(page, url, { timeout, navAttempts = 4, maxRetryWait = 190000 }) {
  for (let attempt = 0; attempt < navAttempts; attempt += 1) {
    let response = null;
    try {
      response = await page.goto(url, { waitUntil: 'domcontentloaded', timeout });
    } catch (err) {
      response = null;
    }
    const status = response ? response.status() : null;
    if (status && status < 400) {
      const mounted = await page.waitForFunction(() => {
        const root = document.getElementById('root') || document.body;
        return !!root && (root.innerText || '').trim().length > 0;
      }, { timeout: 15000 }).then(() => true).catch(() => false);
      if (mounted) return { response, attempts: attempt + 1 };
    }
    let waitMs = Math.min(3000 * (attempt + 1), 20000);
    const retryAfter = response && response.headers() ? response.headers()['retry-after'] : null;
    if (retryAfter) waitMs = Math.min(parseInt(retryAfter, 10) * 1000 || waitMs, maxRetryWait);
    if (attempt < navAttempts - 1) {
      console.error(`  [nav-retry] ${url} status=${status} attempt=${attempt + 1}/${navAttempts} wait=${waitMs}ms`);
      await page.waitForTimeout(waitMs);
    }
  }
  return { response: null, attempts: navAttempts };
}

async function login(page, { baseUrl, route, username, password, timeout, navAttempts, maxRetryWait }) {
  const signIn = new URL('/sign-in', baseUrl);
  signIn.searchParams.set('redirect', route);
  const nav = await gotoWithRetry(page, signIn.toString(), { timeout, navAttempts, maxRetryWait });
  await page.waitForTimeout(1200);
  const usernameInput = page.locator('input[name="username"]').first();
  if ((await usernameInput.count()) === 0) {
    // Already authenticated (or a non-password login page): nothing to fill.
    await page.waitForTimeout(800);
    if ((await usernameInput.count()) === 0) return { alreadyAuthed: true, navAttempts: nav.attempts };
  }
  await usernameInput.fill(username, { timeout });
  await page.locator('input[name="password"]').first().fill(password, { timeout });
  await page.locator('button[type="submit"]').first().click({ timeout });
  await page.waitForURL((u) => !u.pathname.includes('sign-in'), { timeout }).catch(() => {});
  await page.waitForTimeout(1500);
  return { alreadyAuthed: false, navAttempts: nav.attempts };
}

function makeCollectors() {
  return { viewport: null, consoleErrors: [], pageErrors: [], failedRequests: [], patchRequests: [] };
}

/*
 * Event listeners are attached to one long-lived page. `col` is reset before
 * every viewport, and each event is tagged with the viewport that was current
 * when it fired, so a single context can serve all viewports without losing
 * per-viewport attribution.
 */
function attachCollectors(page, col) {
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      col.consoleErrors.push({ text: msg.text().slice(0, 500), location: msg.location() });
    }
  });
  page.on('pageerror', (err) => col.pageErrors.push({ message: String(err && err.message ? err.message : err).slice(0, 500) }));
  page.on('requestfailed', (req) => {
    col.failedRequests.push({ kind: 'requestfailed', method: req.method(), url: req.url(), failure: req.failure() ? req.failure().errorText : null });
  });
  page.on('response', async (res) => {
    const status = res.status();
    if (status >= 400) {
      let body = null;
      try { body = (await res.text()).slice(0, 2000); } catch (e) { /* ignore */ }
      col.failedRequests.push({ kind: 'status', method: res.request().method(), url: res.url(), status, body });
    }
  });
  page.on('request', (req) => {
    const method = req.method();
    if (method === 'PATCH' || method === 'PUT') {
      let parsed = null;
      const postData = req.postData();
      if (postData) { try { parsed = JSON.parse(postData); } catch (e) { parsed = null; } }
      col.patchRequests.push({ viewport: col.viewport, method, url: req.url(), postData, json: parsed, startedAt: new Date().toISOString() });
    }
  });
  page.on('response', async (res) => {
    const req = res.request();
    const method = req.method();
    if (method !== 'PATCH' && method !== 'PUT') return;
    const entry = col.patchRequests.find((p) => p.url === req.url() && p.method === method && !p.status);
    if (!entry) return;
    entry.status = res.status();
    try { entry.responseText = (await res.text()).slice(0, 4000); } catch (e) { entry.responseError = String(e && e.message ? e.message : e); }
  });
}

async function runViewport(page, col, opts, password, vp, index, loginOnce) {
  const label = `${opts.label}-${vp.spec}`;
  const shareSelector = `input[aria-label^="${opts.shareLabelPrefix}"]`;
  col.viewport = vp.spec;
  col.consoleErrors = [];
  col.pageErrors = [];
  col.failedRequests = [];
  col.patchRequests = [];
  const { consoleErrors, pageErrors, failedRequests, patchRequests } = col;

  const report = {
    label: vp.spec,
    startedAt: new Date().toISOString(),
    login: null,
    collapsed: null,
    advanced: null,
    consoleErrors,
    pageErrors,
    failedRequests,
    patchRequests,
    screenshots: {},
    error: null,
  };

  try {
    await page.setViewportSize({ width: vp.width, height: vp.height });
    if (index === 0) {
      report.login = await login(page, { baseUrl: opts.baseUrl, route: opts.route, username: opts.username, password, timeout: opts.timeout, navAttempts: opts.navAttempts, maxRetryWait: opts.maxRetryWait });
    } else {
      report.login = { reusedContext: true, loginOnce };
    }
    const nav = await gotoWithRetry(page, opts.baseUrl + opts.route, { timeout: opts.timeout, navAttempts: opts.navAttempts, maxRetryWait: opts.maxRetryWait });
    report.navAttempts = nav.attempts;
    const count = await waitForStableShares(page, shareSelector, opts.timeout);
    if (count === 0) {
      await page.waitForTimeout(2000);
      throw new Error(`card not populated: no elements matching ${shareSelector}`);
    }
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(300);

    report.collapsed = await page.evaluate(measureInPage, { prefix: opts.shareLabelPrefix });
    report.collapsed.aggregate = aggregate(report.collapsed);

    if (!opts.noScreenshots) {
      const viewportPath = path.join(opts.outDir, `${label}-viewport.png`);
      const fullPath = path.join(opts.outDir, `${label}-full.png`);
      await page.screenshot({ path: viewportPath });
      await page.screenshot({ path: fullPath, fullPage: true });
      report.screenshots.viewport = viewportPath;
      report.screenshots.full = fullPath;
    }

    if (opts.expandAdvanced) {
      const clicked = await page.evaluate(expandAdvancedInPage);
      await page.waitForTimeout(600);
      await page.evaluate(() => window.scrollTo(0, 0));
      report.advancedClicked = clicked;
      report.advanced = await page.evaluate(measureInPage, { prefix: opts.shareLabelPrefix });
      report.advanced.aggregate = aggregate(report.advanced);
      if (!opts.noScreenshots) {
        const advancedPath = path.join(opts.outDir, `${label}-advanced-full.png`);
        await page.screenshot({ path: advancedPath, fullPage: true });
        report.screenshots.advancedFull = advancedPath;
      }
    }

    // One deterministic weight edit, only on the first viewport, so the PATCH
    // payload can be compared across branches.
    if (opts.editFirstShare !== null && index === 0) {
      const first = page.locator(shareSelector).first();
      const original = await first.inputValue();
      // --edit-first-share accepts an absolute value ("42") or a relative
      // step ("+5" / "-5"), so callers do not need to know the seeded value.
      const rawTarget = String(opts.editFirstShare);
      const rel = rawTarget.match(/^([+-])(\d+)$/);
      const target = rel
        ? String(Math.min(100, Math.max(0, Number(original) + (rel[1] === '-' ? -1 : 1) * Number(rel[2]))))
        : rawTarget;
      if (original === target) {
        report.edit = { skipped: true, reason: 'target equals current value', original, target };
      } else {
        await first.fill(target);
        await page.waitForTimeout(200);
        const waitPatch = page.waitForResponse(
          (res) => (res.request().method() === 'PATCH' || res.request().method() === 'PUT') && res.url().includes('/api/'),
          { timeout: opts.timeout }
        );
        const saveButton = page.getByRole('button', { name: new RegExp(opts.saveLabel, 'i') }).first();
        await saveButton.click({ timeout: opts.timeout });
        const response = await waitPatch.catch(() => null);
        await page.waitForTimeout(1200);
        report.edit = {
          skipped: false,
          original,
          target,
          savedValue: await first.inputValue(),
          responseStatus: response ? response.status() : null,
        };
        if (opts.restore) {
          const restoreInput = page.locator(shareSelector).first();
          await restoreInput.fill(original);
          await page.waitForTimeout(200);
          const waitRestore = page.waitForResponse(
            (res) => (res.request().method() === 'PATCH' || res.request().method() === 'PUT') && res.url().includes('/api/'),
            { timeout: opts.timeout }
          );
          await page.getByRole('button', { name: new RegExp(opts.saveLabel, 'i') }).first().click({ timeout: opts.timeout });
          const restoreResponse = await waitRestore.catch(() => null);
          await page.waitForTimeout(1200);
          report.edit.restored = { value: await page.locator(shareSelector).first().inputValue(), responseStatus: restoreResponse ? restoreResponse.status() : null };
        }
      }
    }
  } catch (err) {
    report.error = String(err && err.message ? err.message : err);
    try {
      report.collapsed = report.collapsed || await page.evaluate(measureInPage, { prefix: opts.shareLabelPrefix });
      if (report.collapsed && !report.collapsed.aggregate) report.collapsed.aggregate = aggregate(report.collapsed);
      if (!opts.noScreenshots) {
        const failPath = path.join(opts.outDir, `${label}-failure.png`);
        await page.screenshot({ path: failPath, fullPage: true }).catch(() => {});
        report.screenshots.failure = failPath;
      }
    } catch (e) { /* ignore secondary failure */ }
  } finally {
    /* the shared context is closed once by main() */
  }

  report.finishedAt = new Date().toISOString();
  report.consoleErrorCount = consoleErrors.length;
  report.failedRequestCount = failedRequests.length;
  report.patchCount = patchRequests.length;
  return report;
}

async function main() {
  const opts = parseArgs(process.argv.slice(2));
  if (opts.help) { console.log(usage()); return; }
  const password = readPassword(opts);
  const viewports = opts.viewports.map(parseViewport);
  if (!opts.outDir) opts.outDir = path.join('ops', 'visual', 'out', opts.label);
  fs.mkdirSync(opts.outDir, { recursive: true });

  const browser = await chromium.launch({ headless: !opts.headed });
  const run = {
    tool: 'ops/visual/model-weights-verify.cjs',
    startedAt: new Date().toISOString(),
    baseUrl: opts.baseUrl,
    route: opts.route,
    label: opts.label,
    outDir: path.resolve(opts.outDir),
    viewports: viewports.map((v) => v.spec),
    editFirstShare: opts.editFirstShare,
    restore: opts.restore,
    expandAdvanced: opts.expandAdvanced,
    navAttempts: opts.navAttempts,
    maxRetryWait: opts.maxRetryWait,
    betweenViewports: opts.betweenViewports,
    playwrightModule: PLAYWRIGHT_MODULE,
    reports: [],
  };

  // One context and one page for all viewports: the SPA bundle is fetched once
  // (the candidate rate-limits static assets hard), and only the viewport size
  // changes between measurements.
  const context = await browser.newContext({
    viewport: { width: viewports[0].width, height: viewports[0].height },
    deviceScaleFactor: 1,
    locale: 'en-US',
    colorScheme: 'light',
  });
  await context.addInitScript(() => {
    try { localStorage.setItem('i18nextLng', 'en'); } catch (e) {}
  });
  const page = await context.newPage();
  const col = makeCollectors();
  attachCollectors(page, col);

  try {
    for (let i = 0; i < viewports.length; i += 1) {
      const report = await runViewport(page, col, opts, password, viewports[i], i, true);
      run.reports.push(report);
      console.log(`[${viewports[i].spec}] rows=${report.collapsed ? report.collapsed.aggregate.rowCount : 0} errors=${report.consoleErrorCount} failed=${report.failedRequestCount} patches=${report.patchCount}${report.error ? ' ERROR=' + report.error : ''}`);
      if (i < viewports.length - 1 && opts.betweenViewports > 0) {
        await page.waitForTimeout(opts.betweenViewports);
      }
    }
  } finally {
    await context.close().catch(() => {});
    await browser.close().catch(() => {});
  }

  run.finishedAt = new Date().toISOString();
  run.summary = {
    viewports: run.reports.map((r) => ({
      viewport: r.label,
      error: r.error,
      cards: r.collapsed ? r.collapsed.aggregate.cardCount : 0,
      rows: r.collapsed ? r.collapsed.aggregate.rowCount : 0,
      consoleErrors: r.consoleErrorCount,
      failedRequests: r.failedRequestCount,
      patches: r.patchCount,
    })),
  };
  const reportPath = path.join(opts.outDir, 'report.json');
  fs.writeFileSync(reportPath, JSON.stringify(run, null, 2));

  const payloads = run.reports.flatMap((r) => r.patchRequests.map((p) => ({
    viewport: p.viewport, method: p.method, url: p.url, status: p.status,
    json: p.json, postData: p.postData, responseText: p.responseText,
  })));
  fs.writeFileSync(path.join(opts.outDir, 'patch-payloads.json'), JSON.stringify(payloads, null, 2));

  console.log('report=' + path.resolve(reportPath));
  console.log('patch-payloads=' + path.resolve(path.join(opts.outDir, 'patch-payloads.json')));
  console.log('screenshots=' + JSON.stringify(run.reports.map((r) => r.screenshots)));
  if (run.reports.some((r) => r.error)) process.exitCode = 2;
}

main().catch((err) => {
  console.error('FATAL: ' + (err && err.stack ? err.stack : err));
  process.exit(1);
});
