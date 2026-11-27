// The rules view: every rule, filtered, with live counts (S38, spec 17.2, 17.3).
import { stand, markChip, esc, crumb, bar, STANDING, W } from "./lib.mjs";

// A made-up rulebook: 96 rules. The shown rules are 8 of the 14 that match.
const RULES = [
  {
    id: "go.errors.4",
    text: "Wrap an error with %w when it crosses a package boundary.",
    standing: "enforced",
    part: "go.errors",
    strength: "must",
  },
  {
    id: "go.errors.2",
    text: "Check an error with errors.Is, never with ==.",
    standing: "enforced",
    trial: true,
    part: "go.errors",
    strength: "must",
  },
  {
    id: "go.errors.6",
    text: "Add context once, at the layer that knows it.",
    standing: "reviewed",
    part: "go.errors",
    strength: "should",
  },
  {
    id: "go.errors.9",
    text: "Return a bare sentinel error from a package API.",
    standing: "planned",
    unproven: true,
    part: "go.errors",
    strength: "should",
  },
  {
    id: "go.errors.8",
    text: "Wrap with the verb %w, not %v.",
    standing: "proposed",
    part: "go.errors",
    strength: "must",
  },
  {
    id: "go.code.3",
    text: "Name a package for what it provides.",
    standing: "enforced",
    part: "go.code",
    strength: "must",
  },
  {
    id: "go.code.7",
    text: "Keep an interface where it is used, not where it is made.",
    standing: "advisory",
    part: "go.code",
    strength: "may",
  },
  {
    id: "go.code.1",
    text: "Do not use init to set state a caller can set.",
    standing: "retired",
    part: "go.code",
    strength: "must",
  },
];
const FILTERS = [
  {
    head: "Standing",
    opts: Object.keys(STANDING).map((k) => [
      k,
      {
        proposed: 7,
        enforced: 52,
        reviewed: 9,
        planned: 17,
        advisory: 8,
        retired: 3,
      }[k],
      k,
    ]),
  },
  {
    head: "Strength",
    opts: [
      ["must", 61],
      ["should", 27],
      ["may", 8],
    ],
  },
  {
    head: "Part",
    opts: [
      ["go.errors", 5],
      ["go.code", 3],
      ["go.test", 38],
      ["docs.voice", 50],
    ],
  },
  {
    head: "Kind",
    opts: [
      ["service", 71],
      ["library", 83],
      ["app", 40],
    ],
  },
  {
    head: "Mark",
    opts: [
      ["trial", 6],
      ["blocked", 2],
      ["unproven", 11],
    ],
  },
];
const ON = { Part: ["go.errors", "go.code"] }; // the filters the default frame has set

function chip(head, name) {
  return head === "Standing"
    ? stand(name)
    : head === "Mark"
      ? markChip(name)
      : "";
}
function opt(head, o, sel) {
  return `<label class="kind${sel ? " on" : ""}" data-part="rules-filter-option"><span class="box${sel ? " on" : ""}" aria-hidden="true">${sel ? "v" : ""}</span><span>${chip(head, o[0]) || esc(o[0])}</span><span class="n">${o[1]}</span></label>`;
}
function filterGroup(g) {
  return `<fieldset class="fg" data-part="rules-filter"><legend>${g.head}</legend>${g.opts.map((o) => opt(g.head, o, (ON[g.head] || []).includes(o[0]))).join("")}</fieldset>`;
}
const activeChips = (on = ON) =>
  `<div class="act-chips" data-part="rules-active"><span class="lbl">Showing</span>${Object.entries(
    on,
  )
    .flatMap(([h, v]) =>
      v.map(
        (x) =>
          `<span class="fchip">${h.toLowerCase()}: ${x}<span class="x" aria-label="Remove">x</span></span>`,
      ),
    )
    .join("")}<a class="clr" data-part="rules-clear">Clear all</a></div>`;
function row(r, w) {
  const tags =
    stand(r.standing, { trial: !!r.trial }) +
    (r.trial ? markChip("trial") : "") +
    (r.unproven ? markChip("unproven") : "");
  return `<a class="r${w === 390 ? " stack" : ""}" data-part="rules-row"><div class="main2"><div class="id">${r.id}</div><div class="tx">${esc(r.text)}</div><div class="meta">${r.part} · ${r.strength}</div></div><div class="tags">${tags}</div></a>`;
}
const list = (w) =>
  `<div class="list" data-part="rules-list" role="list">${RULES.map((r) => row(r, w)).join("")}</div>`;
const count = `<p class="count" data-part="rules-count" role="status" aria-live="polite"><b>8</b> of 96 rules</p>`;
const sortCtl = `<span class="btn" data-part="rules-sort">Sort: id</span>`;
const head = (w) =>
  `<section class="focus" data-part="rules-head">${crumb("rules-crumb", ["Rules"])}<div class="t"><h1 class="plain">Rules</h1>${count}</div>${w === 1440 ? "" : ""}</section>`;

function defaultBody(w) {
  if (w === 1440) {
    return `<main class="main">${head(w)}<div class="body"><aside class="rail" data-part="rules-filters" aria-label="Filters"><h2>Filter</h2>${FILTERS.map(filterGroup).join("")}</aside><div class="col">${activeChips()}<div class="tools"><span class="search" data-part="rules-find" style="flex:1">Find in these rules</span>${sortCtl}</div>${list(w)}</div></div></main>`;
  }
  if (w === 900) {
    const chips = `<div class="kinds" data-part="rules-filters">${FILTERS[0].opts.map((o) => `<span class="kind${(ON.Standing || []).includes(o[0]) ? " on" : ""}" data-part="rules-filter-option"><span class="box${(ON.Standing || []).includes(o[0]) ? " on" : ""}" aria-hidden="true">${(ON.Standing || []).includes(o[0]) ? "v" : ""}</span>${stand(o[0])}<span class="n">${o[1]}</span></span>`).join("")}</div>`;
    return `<main class="main">${head(w)}${chips}<div class="tools"><span class="btn" data-part="rules-filters-button">More filters (1)</span><span class="search" data-part="rules-find" style="flex:1;height:44px">Find in these rules</span>${sortCtl}</div>${activeChips()}${list(w)}</main>`;
  }
  return `<main class="main">${head(w)}<span class="btn" data-part="rules-filters-button" style="width:100%;justify-content:space-between"><span>Filters (2)</span><span aria-hidden="true">+</span></span>${activeChips()}<span class="search" data-part="rules-find" style="height:44px">Find in these rules</span>${list(w)}</main>`;
}

function skeleton(w) {
  const rows = Array.from(
    { length: w === 390 ? 5 : 7 },
    () =>
      `<div class="r" style="gap:12px;align-items:center"><div class="main2" style="display:flex;flex-direction:column;gap:8px">${bar("30%")}${bar("80%", 12)}</div>${bar("72px", 24)}</div>`,
  ).join("");
  const head = `<section class="focus" data-part="rules-head" aria-busy="true"><div style="display:flex;gap:12px;align-items:center">${bar("120px", 32)}${bar("90px", 20)}</div><div class="live" role="status"><span aria-hidden="true">...</span>Loading the rules</div></section>`;
  if (w === 1440)
    return `<main class="main">${head}<div class="body"><aside class="rail">${Array.from({ length: 8 }, () => `<div class="sk" style="height:32px;margin-top:4px"></div>`).join("")}</aside><div class="col"><div class="list">${rows}</div></div></div></main>`;
  return `<main class="main">${head}${w === 900 ? `<div class="sk" style="height:44px"></div>` : `<div class="sk" style="height:44px"></div>`}<div class="list">${rows}</div></main>`;
}
function empty(w) {
  return `<main class="main"><section class="msg" data-part="rules-empty">${crumb("rules-crumb", ["Rules"])}<h1>No rule matches</h1><p>No rule is retired and also carries the trial mark: a trial sits on enforced, reviewed or planned, never on a retired rule. <b>0</b> of 96 rules.</p><p>Remove a filter to see more. The last one you set was mark: trial.</p>${activeChips({ Standing: ["retired"], Mark: ["trial"] })}<div class="acts"><a class="btn p" data-part="rules-clear-all">Clear all filters</a><a class="btn" data-part="open-bars">See standing by part</a></div></section></main>`;
}
function error(w) {
  return `<main class="main"><section class="msg" data-part="rules-error" role="alert">${crumb("rules-crumb", ["Rules"])}<h1>No standing named enforcing</h1><p>The link filters by standing enforcing, and the rulebook has six standings: proposed, enforced, reviewed, planned, advisory, retired.</p><p>The rules are shown unfiltered below the link. Pick a standing to filter again.</p><div class="acts"><a class="btn p" data-part="rules-clear-all">Show all 96 rules</a><span class="search" data-part="search-action" style="height:44px">Search for a rule</span></div></section></main>`;
}
const body = (w, state) =>
  state === "loading"
    ? skeleton(w)
    : state === "empty"
      ? empty(w)
      : state === "error"
        ? error(w)
        : defaultBody(w);

const m = (a, b, c) => [a, b, c];
export const view = {
  key: "rules",
  title: "Rules",
  nav: "rules",
  route: "/rules",
  body,
  css: `
.fg{border:0;display:flex;flex-direction:column;gap:2px;margin:0 0 12px}.fg legend{font-size:calc(12px*var(--tg,1));font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--muted);margin-bottom:4px}
.count{font-size:calc(15px*var(--tg,1));color:var(--muted)}.count b{color:var(--ink)}
.tools{display:flex;gap:8px;align-items:center}
.act-chips{display:flex;flex-wrap:wrap;gap:8px;align-items:center;font-size:calc(13px*var(--tg,1));color:var(--muted)}
.fchip{display:inline-flex;align-items:center;gap:8px;min-height:var(--ctl);padding:0 12px;border:1px solid var(--line-strong);border-radius:6px;color:var(--ink);background:var(--canvas)}
.fchip .x{font-family:"IBM Plex Mono",monospace;color:var(--muted);display:inline-flex;align-items:center;justify-content:center;min-width:24px;min-height:24px}
.clr{color:var(--accent);text-decoration:underline;display:inline-flex;align-items:center;min-height:var(--ctl)}
.r .meta{font-size:calc(12px*var(--tg,1));color:var(--muted);margin-top:4px}
.mk.unproven{border-style:dotted}.mk.blocked{border-color:var(--danger);color:var(--danger)}`,
  caps: {
    default:
      "8 of 96 rules, filtered to the parts go.errors and go.code. Every row carries its standing as a word and a symbol.",
    loading:
      "The same frame as grey blocks and one line that says what is loading. The region is marked busy.",
    empty:
      "Filters that match nothing. It names the filters, says how many rules there are, and offers one button to clear them.",
    error:
      "A link that names a standing the rulebook does not have. It lists the six that exist and offers all the rules.",
    stale:
      "A bundle from an older contract major. It is read, with a banner above the view.",
    grown:
      "Default with text at 200 percent. The rows wrap and nothing scrolls sideways.",
  },
  note: {
    purpose:
      "A maintainer, a reviewer or a project owner opens this view to find the rules that matter to them. It answers: which rules are these, and how firmly does each hold?",
    conveys: [
      'How many rules match, live, as the filters change: "8 of 96 rules".',
      "The filters that are set, each removable, and one place to clear them all.",
      "Each rule by id and statement, with its standing as a word and a symbol and its marks beside it.",
    ],
    layout: {
      390: "One column. The count and the title first, one Filters button that opens the filters, the set filters as removable chips, then the rules. The filters are not a rail, which would take the width.",
      900: "One column. The standing chips are always visible, since standing is the filter people reach for first. The other filters are behind one More filters button.",
      1440: "The filter rail on the left, always open, with a count beside every option. The rules on the right, the active filters above them. The eye goes from the count to the filters to the rules.",
    },
    standing:
      "Every row carries the standing as a word and a symbol in both themes, and the marks (trial, unproven) as words with their own shape. Colour is never the only carrier (web.accessibility.4).",
  },
  differs: [
    'Standing words: the spec\'s six (proposed, enforced, reviewed, planned, advisory, retired). The canvas filter offered Enforced, Planned, Advisory, Later and Proposed, so "Later" is gone and "reviewed" and "retired" are added. The spec (17.2) wins.',
    "Symbols: a symbol beside every standing word. The canvas rows showed the word on a coloured chip only.",
    "Marks (17.3): trial, blocked and unproven are drawn beside a standing. The canvas drew none.",
    "Navigation: only explorer views. The canvas header also named Guides and Handbook.",
    "Breakpoints: three named values, 390, 900 and 1440. The canvas used the 768 utility for its tablet layout.",
  ],
  parts: [
    {
      name: "rules-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "rules-head",
      purpose: "the title and the live count of the rules shown",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "rules-count",
      purpose: "how many rules match, spoken as it changes (aria-live polite)",
      measured: { height: m(24, 24, 24) },
    },
    {
      name: "rules-filters",
      purpose: "the filters, with a count beside every option",
      measured: { width: m(358, 852, 264), groups: m(null, 1, 5) },
    },
    {
      name: "rules-filter",
      purpose: "one group of options: standing, strength, part, kind, mark",
      measured: { count: m(null, null, 5) },
    },
    {
      name: "rules-filter-option",
      purpose: "one option, a toggle with its count",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "rules-filters-button",
      purpose: "opens the filters that do not fit, with how many are set",
      measured: { height: m(44, 44, null), width: m(358, null, null) },
    },
    {
      name: "rules-active",
      purpose: "the filters that are set, each removable",
      measured: {
        "chip-height": m(44, 44, 32),
        "remove-target": m(24, 24, 24),
      },
    },
    {
      name: "rules-clear",
      purpose: "clears every filter at once",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "rules-find",
      purpose: "finds a rule among those shown",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "rules-sort",
      purpose: "chooses the order of the list",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "rules-list",
      purpose: "the matching rules, one row each",
      measured: { width: m(358, 852, 1064 - 264 - 32) },
    },
    {
      name: "rules-row",
      purpose: "one rule: id, statement, part, strength, standing and marks",
      measured: { "min-height": m(64, 56, 52), count: m(8, 8, 8) },
    },
    {
      name: "mark",
      purpose: "trial, blocked or unproven beside a standing",
      measured: { height: m(24, 24, 24) },
    },
    {
      name: "rules-empty",
      purpose: "filters that match nothing, with a way back",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "rules-clear-all",
      purpose: "the one button of the empty and error states",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "open-bars",
      purpose: "leaves the empty state for the standing bars",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "rules-error",
      purpose: "a link that names a standing the rulebook does not have",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
  ],
};
