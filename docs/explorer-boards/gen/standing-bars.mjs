// The standing bars view: how many rules hold at each standing, in total and by part
// (S38, spec 17.2). Bars are own parts, no chart library (web.parts.21), and every bar
// has its numbers as text, so a bar is never the only way to read the view.
import { stand, markChip, crumb, bar, STANDING } from "./lib.mjs";

const ORDER = [
  "enforced",
  "reviewed",
  "planned",
  "advisory",
  "proposed",
  "retired",
];
const PARTS = [
  { part: "go.errors", c: { enforced: 3, reviewed: 1, planned: 1 } },
  { part: "go.code", c: { enforced: 1, advisory: 1, retired: 1 } },
  {
    part: "go.test",
    c: { enforced: 21, reviewed: 3, planned: 9, advisory: 3, proposed: 2 },
  },
  {
    part: "docs.voice",
    c: {
      enforced: 27,
      reviewed: 5,
      planned: 7,
      advisory: 4,
      proposed: 5,
      retired: 2,
    },
  },
];
const TOTAL = Object.fromEntries(
  ORDER.map((k) => [k, PARTS.reduce((n, p) => n + (p.c[k] || 0), 0)]),
);
const sum = (c) => Object.values(c).reduce((a, b) => a + b, 0);
const ALL = sum(TOTAL);
const MARKS = [
  ["trial", 6],
  ["blocked", 2],
  ["unproven", 11],
];

// One stacked bar. width is the bar's pixel width, scale the rules a full bar stands for.
function stack(c, width, height, scale, part) {
  const segs = ORDER.filter((k) => c[k])
    .map((k) => {
      const w = Math.max(2, Math.round((c[k] / scale) * width));
      return `<a class="seg ${k}" data-part="bars-segment" style="width:${w}px" title="${c[k]} ${k}"><span class="sn" aria-hidden="true">${w >= 36 ? c[k] : ""}</span></a>`;
    })
    .join("");
  return `<div class="stack" data-part="bars-bar" style="height:${height}px" role="img" aria-label="${part}: ${ORDER.filter(
    (k) => c[k],
  )
    .map((k) => c[k] + " " + k)
    .join(", ")}">${segs}</div>`;
}
const legend = () =>
  `<ul class="legend" data-part="bars-legend">${ORDER.map((k) => `<li>${stand(k)}<span class="n mono">${TOTAL[k]}</span></li>`).join("")}</ul>`;
const sentence = (c) =>
  ORDER.filter((k) => c[k])
    .map((k) => `${STANDING[k].sym} ${c[k]} ${k}`)
    .join(", ");
const tableRows = () =>
  PARTS.map(
    (p) =>
      `<tr data-part="bars-table-row"><th>${p.part}</th>${ORDER.map((k) => `<td>${p.c[k] || 0}</td>`).join("")}<td><b>${sum(p.c)}</b></td></tr>`,
  ).join("");
const table = () =>
  `<div class="scroll"><table class="pg" data-part="bars-table"><tr><th>Part</th>${ORDER.map((k) => `<th>${STANDING[k].sym} ${k}</th>`).join("")}<th>All</th></tr>${tableRows()}<tr><th>All</th>${ORDER.map((k) => `<td><b>${TOTAL[k]}</b></td>`).join("")}<td><b>${ALL}</b></td></tr></table></div>`;
const marks = () =>
  `<div class="marks" data-part="bars-marks"><span class="lbl">Marks</span>${MARKS.map(([m, n]) => `${markChip(m)}<span class="n mono">${n}</span>`).join("")}</div>`;
const head = () =>
  `<section class="focus" data-part="bars-head">${crumb("bars-crumb", ["Standing"])}<div class="t"><h1 class="plain">Standing</h1><p class="count" data-part="rules-count"><b>${ALL}</b> rules in 4 parts</p></div></section>`;

function partRows(w, barW, scale) {
  return PARTS.map(
    (p) =>
      `<a class="prow${w === 390 ? " stack1" : ""}" data-part="bars-part"><div class="pn"><span class="id">${p.part}</span><span class="n mono">${sum(p.c)}</span></div>${stack(p.c, barW, w === 390 ? 20 : 32, scale, p.part)}${w === 390 ? `<div class="sent">${sentence(p.c)}</div>` : ""}</a>`,
  ).join("");
}
function defaultBody(w) {
  const scale = Math.max(...PARTS.map((p) => sum(p.c)));
  const total = (bw, h) =>
    `<div class="total" data-part="bars-total"><h2>The whole rulebook</h2>${stack(TOTAL, bw, h, ALL, "all parts")}${legend()}${marks()}</div>`;
  if (w === 1440)
    return `<main class="main">${head()}${total(1064, 40)}<div class="body"><div class="col"><h2 class="sec">By part</h2>${partRows(w, 640, scale)}</div><div class="side" style="width:392px">${table()}</div></div></main>`;
  if (w === 900)
    return `<main class="main">${head()}${total(852, 40)}<h2 class="sec">By part</h2>${partRows(w, 600, scale)}${table()}</main>`;
  return `<main class="main">${head()}${total(358, 32)}<h2 class="sec">By part</h2>${partRows(w, 358, scale)}${table()}</main>`;
}
function skeleton(w) {
  const b = (n) =>
    Array.from(
      { length: n },
      () =>
        `<div class="sk" style="height:${w === 390 ? 20 : 32}px;margin-bottom:16px"></div>`,
    ).join("");
  const h = `<section class="focus" data-part="bars-head" aria-busy="true"><div style="display:flex;gap:12px;align-items:center">${bar("160px", 32)}${bar("120px", 20)}</div><div class="live" role="status"><span aria-hidden="true">...</span>Loading the standing</div></section>`;
  return `<main class="main">${h}<div class="sk" style="height:40px"></div>${b(4)}</main>`;
}
function empty() {
  return `<main class="main"><section class="msg" data-part="bars-empty">${crumb("bars-crumb", ["Standing"])}<h1>No rules to count</h1><p>This rulebook has no rules yet, so no rule has a standing. A rule has one as soon as it is written: it starts as proposed.</p><div class="acts"><a class="btn p" data-part="open-rules">Open the rules</a></div></section></main>`;
}
function error() {
  return `<main class="main"><section class="msg" data-part="bars-error" role="alert">${crumb("bars-crumb", ["Standing"])}<h1>This bundle has no standing</h1><p>The bundle was built without the bindings, and standing is derived from them, so it cannot be counted. The rules themselves are in the bundle.</p><p>Build it again with <span class="mono">gragas explorer</span>.</p><div class="acts"><a class="btn p" data-part="open-rules">Open the rules</a></div></section></main>`;
}
const body = (w, s) =>
  s === "loading"
    ? skeleton(w)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

const m = (a, b, c) => [a, b, c];
export const view = {
  key: "standing-bars",
  title: "Standing",
  nav: null,
  route: "/standing",
  body,
  css: `
.total{display:flex;flex-direction:column;gap:12px}.total h2,.sec{font-size:calc(12px*var(--tg,1));font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--muted)}
.stack{display:flex;border-radius:4px;overflow:hidden;border:1px solid var(--line-strong);max-width:100%}
.seg{display:flex;align-items:center;justify-content:center;font-family:"IBM Plex Mono",monospace;font-size:calc(12px*var(--tg,1));font-weight:600;border-right:1px solid var(--canvas)}
.seg:last-child{border-right:0}
.seg.enforced{background:var(--s-enforced-soft);color:var(--s-enforced);box-shadow:inset 0 -3px 0 var(--s-enforced)}
.seg.reviewed{background:var(--s-reviewed-soft);color:var(--s-reviewed);box-shadow:inset 0 -3px 0 var(--s-reviewed)}
.seg.planned{background:var(--s-planned-soft);color:var(--s-planned);box-shadow:inset 0 -3px 0 var(--s-planned)}
.seg.advisory{background:var(--s-advisory-soft);color:var(--s-advisory);box-shadow:inset 0 -3px 0 var(--s-advisory)}
.seg.proposed{background:var(--s-proposed-soft);color:var(--s-proposed);box-shadow:inset 0 -3px 0 var(--s-proposed)}
.seg.retired{background:var(--s-retired-soft);color:var(--s-retired);box-shadow:inset 0 -3px 0 var(--s-retired)}
.legend{list-style:none;display:flex;flex-wrap:wrap;gap:8px 16px}.legend li{display:flex;gap:6px;align-items:center}
.marks{display:flex;flex-wrap:wrap;gap:8px;align-items:center}.marks .lbl{font-size:calc(12px*var(--tg,1));color:var(--muted);font-weight:600;text-transform:uppercase;letter-spacing:.04em}
.n.mono{font-size:calc(12px*var(--tg,1));color:var(--muted)}
.prow{display:flex;align-items:center;gap:16px;min-height:var(--ctl);padding:6px 4px;border-bottom:1px solid var(--line);color:inherit;text-decoration:none}
.prow .pn{width:160px;flex:none;display:flex;justify-content:space-between;gap:8px;font-size:calc(13px*var(--tg,1))}.prow .id{font-family:"IBM Plex Mono",monospace;font-weight:500}
.prow.stack1{flex-direction:column;align-items:stretch;gap:6px;min-height:64px}.prow.stack1 .pn{width:auto}.sent{font-size:calc(12px*var(--tg,1));color:var(--prose);overflow-wrap:anywhere}
.side{flex:none}
.scroll{overflow-x:auto}.pg{border-collapse:collapse;font-size:calc(13px*var(--tg,1));min-width:100%}.pg th,.pg td{border:1px solid var(--line);padding:6px 10px;text-align:left;white-space:nowrap}.pg th{background:var(--surface);font-weight:600}
.count{font-size:calc(15px*var(--tg,1));color:var(--muted)}.count b{color:var(--ink)}
.mk.unproven{border-style:dotted}.mk.blocked{border-color:var(--danger);color:var(--danger)}`,
  caps: {
    default: `${ALL} rules, in total and by part. Each bar is also written as numbers, in a table and, on the phone, under the bar.`,
    loading:
      "The same frame as grey blocks and one line that says what is loading. The region is marked busy.",
    empty:
      "A rulebook with no rules. It says why there is nothing to count and offers the rules view.",
    error:
      "A bundle built without the bindings, so standing cannot be derived. It says so and how to build it again.",
    stale:
      "A bundle from an older contract major. It is read, with a banner above the view.",
    grown:
      "Default with text at 200 percent. The bars keep their width, the labels wrap and the table scrolls inside its own box.",
  },
  note: {
    purpose:
      "A rulebook owner or a reviewer opens this view to see how firmly the rulebook holds. It answers: how many of our rules are enforced, and where are the gaps?",
    conveys: [
      "The whole rulebook as one bar, the six standings in a fixed order from firmest to retired, with each count in a legend.",
      "The marks beside it: how many rules are trial, blocked or unproven.",
      "Each part as its own bar on the same scale, so the gaps stand out. Planned and proposed are the gaps.",
      "Every number as text, in a table, so no bar is the only way to read it.",
    ],
    layout: {
      390: "One column. The whole-rulebook bar, the legend, the marks, then a row for each part with its bar and its numbers as a sentence. The table scrolls inside its own box. A segment is not a target at this width; the part row is one 44 px target to the rules of that part.",
      900: "One column. The whole-rulebook bar at full width, then the part bars, then the table.",
      1440: "The whole-rulebook bar across the top. The part bars on the left, the table on the right. A segment links to the rules view filtered to that part and standing.",
    },
    standing:
      "Each segment is a colour with its count, and the legend and table name every standing with its symbol and word. A standing is never told by colour alone: the segment also has a bottom rule in its own role colour and its count as text where it fits.",
  },
  differs: [
    'This view is its own view, as the hub ruled from the spec (S38 lists standing bars as one of the twelve). The canvas folded standing into its coverage board as a "Colour by Standing" control.',
    "Standing words: the spec's six (17.2). The canvas used Enforced, Planned, Advisory, Later and Proposed.",
    "Marks (17.3): counted beside the bars. The canvas drew none.",
    "Navigation: the view is under More, since the header names five of the twelve. The canvas header named Guides and Handbook, which are not explorer views.",
  ],
  parts: [
    {
      name: "bars-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "bars-head",
      purpose: "the title and how many rules are counted",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "bars-total",
      purpose: "the whole rulebook: one bar, its legend and its marks",
      measured: { width: m(358, 852, 1064) },
    },
    {
      name: "bars-bar",
      purpose:
        "one stacked bar of the six standings, own drawn, no chart library",
      measured: { height: m(32, 40, 40), width: m(358, 852, 1064) },
    },
    {
      name: "bars-segment",
      purpose:
        "one standing in a bar, as wide as its count, with its count as text where it fits",
      measured: { "min-width": m(2, 2, 2), "label-from": m(36, 36, 36) },
    },
    {
      name: "bars-legend",
      purpose: "the six standings with symbol, word and count",
      measured: { count: m(6, 6, 6) },
    },
    {
      name: "bars-marks",
      purpose: "how many rules carry each mark",
      measured: { count: m(3, 3, 3) },
    },
    {
      name: "bars-part",
      purpose: "one part: its name, its count and its bar, a link to its rules",
      measured: {
        "min-height": m(64, 44, 32),
        "bar-height": m(20, 32, 32),
        "bar-width": m(358, 600, 640),
        count: m(4, 4, 4),
      },
    },
    {
      name: "bars-table",
      purpose: "every count as text, part by standing",
      measured: {
        width: m(358, 852, 392),
        rows: m(5, 5, 5),
        columns: m(8, 8, 8),
      },
    },
    {
      name: "bars-table-row",
      purpose: "one part in the table",
      measured: { count: m(4, 4, 4) },
    },
    {
      name: "rules-count",
      purpose: "how many rules are counted, spoken as it changes",
      measured: { height: m(24, 24, 24) },
    },
    {
      name: "mark",
      purpose: "trial, blocked or unproven",
      measured: { height: m(24, 24, 24) },
    },
    {
      name: "bars-empty",
      purpose: "a rulebook with no rules",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "bars-error",
      purpose: "a bundle built without bindings",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-rules",
      purpose: "leaves a state of its view for the rules view",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
