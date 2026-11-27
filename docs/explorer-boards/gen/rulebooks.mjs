// The rulebooks view: the chain a project reads, what each rulebook imports, the rule
// maps between releases, and archived successors (S38, spec 6). Made-up rulebooks.
import { esc, bar } from "./lib.mjs";
import {
  m,
  head,
  skeleton,
  message,
  act,
  COMMON_CAPS,
  TABLE_CSS,
} from "./kit.mjs";

const CHAIN = [
  {
    id: "acme-base",
    kind: "base",
    ver: "2.1.0",
    rules: 96,
    note: "The base. No parent.",
  },
  {
    id: "acme-org",
    kind: "overlay",
    ver: "1.4.2",
    rules: 12,
    note: "Overlay of acme-base at 2.1.0. Prefix org.",
  },
  {
    id: "acme-web",
    kind: "overlay",
    ver: "0.9.0",
    rules: 5,
    note: "Overlay of acme-org at 1.4.2. Prefix web.",
  },
];
const IMPORTS = [
  ["acme-docs", "import", "part docs.voice, ids namespaced docs-"],
  ["acme-style", "companion", "cites a public guide by anchor"],
];
const MAPS = [
  ["org.7", "go.errors.4", "tighter", "the overlay sets a stricter severity"],
  ["web.2", "go.code.3", "same", "the same rule under a new id"],
  ["web.5", "none", "none", "dropped, with decision web-d4"],
];
const ARCH = [
  ["acme-legacy", "archived 2026-06-01, successor acme-base, 41 rules mapped"],
];
const kindTag = (k) => `<span class="kt">${k}</span>`;
const mk = (t) =>
  `<span class="st obs"><i aria-hidden="true">${{ same: "=", tighter: ">", weaker: "<", none: "." }[t]}</i>${t}</span>`;

const chain = (w) =>
  `<ol class="chain" data-part="rulebooks-chain" aria-label="The chain, base first">${CHAIN.map((c, i) => `<li data-part="rulebooks-link"><div class="lk${i === CHAIN.length - 1 ? " cur" : ""}"><div class="id">${c.id}</div><div class="tx">${esc(c.note)}</div><div class="tags">${kindTag(c.kind)}<span class="kt">v${c.ver}</span><span class="kt">${c.rules} rules</span>${i === CHAIN.length - 1 ? '<span class="kt">this project reads it</span>' : ""}</div></div></li>`).join("")}</ol>`;
const imports = (w) =>
  `<section class="blk"><h2 class="sec">Imports and companions</h2><div data-part="rulebooks-imports">${IMPORTS.map(([i, k, t]) => `<a class="brow${w === 390 ? " stack" : ""}"><div class="main2"><div class="id">${i}</div><div class="tx">${esc(t)}</div></div><div class="tags">${kindTag(k)}</div></a>`).join("")}</div></section>`;
const maps = (w) => {
  const tbl = `<div class="scroll"><table class="pg" data-part="rulebooks-map"><tr><th>Left</th><th>Arrived at</th><th>Mark</th><th>Reason</th></tr>${MAPS.map(([a, b, c, d]) => `<tr data-part="rulebooks-map-row"><td class="mono">${a}</td><td class="mono">${b}</td><td>${mk(c)}</td><td style="white-space:normal">${d}</td></tr>`).join("")}</table></div>`;
  return `<section class="blk"><h2 class="sec">Rule map, acme-org 1.4.2 to acme-base 2.1.0</h2>${tbl}</section>`;
};
const arch = (w) =>
  `<section class="blk"><h2 class="sec">Archived</h2><div data-part="rulebooks-archived">${ARCH.map(([i, t]) => `<a class="brow"><div class="main2"><div class="id">${i}</div><div class="tx">${esc(t)}</div></div><div class="tags">${kindTag("archived")}</div></a>`).join("")}</div></section>`;

function defaultBody(w) {
  const h = head(
    "rulebooks-head",
    ["Rulebooks"],
    "Rulebooks",
    `<b>3</b> in the chain, 2 imported, 1 archived`,
  );
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><div class="col" style="flex:none;width:520px"><h2 class="sec">The chain</h2>${chain(w)}${imports(w)}${arch(w)}</div><div class="col">${maps(w)}</div></div></main>`;
  return `<main class="main">${h}<h2 class="sec">The chain</h2>${chain(w)}${imports(w)}${maps(w)}${arch(w)}</main>`;
}
const empty = () =>
  message(
    "rulebooks-empty",
    ["Rulebooks"],
    "This rulebook has no parent and no imports",
    [
      "It is a base: the chain is this rulebook alone, and nothing is imported, forked or archived.",
      "A rulebook gets a chain when it is an overlay of another, and imports when it names parts from another.",
    ],
    act("open-rules", "Open the rules"),
  );
const error = () =>
  message(
    "rulebooks-error",
    ["Rulebooks"],
    "The parent acme-base 2.1.0 is not in this bundle",
    [
      "This rulebook is an overlay and names acme-base at 2.1.0 as its parent, and the bundle does not hold that release, so the chain stops here.",
      'Build the bundle again with the parent available: <span class="mono">gragas explorer</span>.',
    ],
    act("open-rules", "Open the rules"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "rulebooks-head", "the rulebooks", 4)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "rulebooks",
  title: "Rulebooks",
  nav: null,
  route: "/rulebooks",
  body,
  css: `${TABLE_CSS}
.chain{list-style:none;display:flex;flex-direction:column;gap:0;border-left:2px solid var(--line-strong);margin-left:8px;padding-left:16px}
.chain li{position:relative;padding:8px 0}.chain li::before{content:"";position:absolute;left:-23px;top:22px;width:10px;height:10px;border-radius:50%;background:var(--canvas);border:2px solid var(--line-strong)}
.chain .lk{border:1px solid var(--line);border-radius:8px;padding:12px;display:flex;flex-direction:column;gap:6px;min-height:var(--row)}.chain .lk.cur{border-color:var(--accent);background:var(--accent-soft)}
.chain .id{font-family:"IBM Plex Mono",monospace;font-weight:600;font-size:calc(14px*var(--tg,1))}.chain .tx{font-size:calc(13px*var(--tg,1));color:var(--prose)}.chain .tags{display:flex;gap:6px;flex-wrap:wrap}
.blk{display:flex;flex-direction:column;gap:8px}`,
  caps: {
    default:
      "A project reads a chain of three rulebooks, with two imports, a rule map and one archived rulebook.",
    ...COMMON_CAPS,
    empty:
      "A base rulebook: no parent, no imports. It says so and offers the rules.",
    error:
      "An overlay whose parent release is not in the bundle. It names the parent and how to build again.",
  },
  note: {
    purpose:
      "A rulebook owner or a project owner opens this view to see where their rules come from. It answers: what do I inherit, from whom, and what changed between releases?",
    conveys: [
      "The chain, base first, with the rulebook this project reads marked.",
      "What each rulebook imports or cites.",
      "How rules moved between two releases: the rule map, row by row, with its mark and reason.",
      "Archived rulebooks and their successors.",
    ],
    layout: {
      390: "One column, the chain first as a vertical line of cards, then imports, the rule map as a table that scrolls in its own box, then archived.",
      900: "One column at full width, in the same order.",
      1440: "The chain, imports and archived on the left, the rule map on the right, so the map is read beside the chain it explains.",
    },
    standing:
      "This view shows no rule standing. A rule map row carries its mark as a word and a symbol (same =, tighter >, weaker <, none .), never colour alone.",
  },
  differs: [
    "The canvas had no board for this view: it is new. It is drawn from S38 and the vocabulary of the spec (chain, import, companion, rule map, archived).",
  ],
  parts: [
    {
      name: "rulebooks-head",
      purpose: "the title and how many rulebooks are in play",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "rulebooks-chain",
      purpose: "the chain from the base to the rulebook the project reads",
      measured: { width: m(358, 852, 520), links: m(3, 3, 3) },
    },
    {
      name: "rulebooks-link",
      purpose: "one rulebook in the chain, with kind, version and rule count",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "rulebooks-imports",
      purpose: "the rulebooks imported or cited",
      measured: { count: m(2, 2, 2), "row-min-height": m(64, 56, 52) },
    },
    {
      name: "rulebooks-map",
      purpose: "the rule map between two releases, row by row",
      measured: {
        columns: m(4, 4, 4),
        rows: m(3, 3, 3),
        width: m(358, 852, 520),
      },
    },
    {
      name: "rulebooks-map-row",
      purpose:
        "one rule: the id left, the id arrived at, the mark and the reason",
      measured: { count: m(3, 3, 3) },
    },
    {
      name: "rulebooks-archived",
      purpose: "archived rulebooks with their successor",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "rulebooks-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "rulebooks-empty",
      purpose: "a base rulebook with no parent and no imports",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "rulebooks-error",
      purpose: "an overlay whose parent release is not in the bundle",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-rules",
      purpose: "leaves a state of its view for the rules view",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
