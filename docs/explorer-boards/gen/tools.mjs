// The tools view: each tool the rulebook binds, how many rules it enforces, and the
// entries in a config that no rule explains (S38, spec 17.4). Made-up tools.
import { esc, obs, OBS } from "./lib.mjs";
import {
  m,
  head,
  skeleton,
  message,
  act,
  COMMON_CAPS,
  TABLE_CSS,
} from "./kit.mjs";

const TOOLS = [
  {
    id: "errorlint",
    ver: "v2.3 to v2.5",
    entries: 6,
    rules: 4,
    obs: { "as-set": 5, tighter: 1 },
  },
  {
    id: "wrapcheck",
    ver: "v1.0 to v1.2",
    entries: 4,
    rules: 3,
    obs: { "as-set": 3, weaker: 1 },
  },
  {
    id: "gofmt",
    ver: "v1.22 to v1.24",
    entries: 1,
    rules: 1,
    obs: { "as-set": 1 },
  },
  {
    id: "prose-lint",
    ver: "v0.8 to v0.9",
    entries: 9,
    rules: 7,
    obs: { "as-set": 6, absent: 2, tighter: 1 },
  },
];
const UNMAPPED = [
  {
    id: "errorlint.asserts",
    why: "in the effective config, no rule explains it",
    o: "unmapped",
  },
  {
    id: "wrapcheck.ignore-pkg",
    why: "unmapped, was excluded by overlay acme-web, decision web-d4",
    o: "unmapped",
    ex: true,
  },
];
const obsRow = (o) =>
  Object.entries(o)
    .map(
      ([k, n]) =>
        `<span class="oc">${obs(k)}<span class="n mono">${n}</span></span>`,
    )
    .join("");
const toolRow = (t, w) =>
  `<a class="brow${w === 390 ? " stack" : ""}" data-part="tools-row"><div class="main2"><div class="id">${t.id}</div><div class="tx">${t.entries} ${t.entries === 1 ? "entry" : "entries"} · ${t.rules} ${t.rules === 1 ? "rule" : "rules"} · plugin for ${t.ver}</div></div><div class="tags">${obsRow(t.obs)}</div></a>`;
const unRow = (u, w) =>
  `<a class="brow${w === 390 ? " stack" : ""}" data-part="tools-unmapped-row"><div class="main2"><div class="id">${u.id}</div><div class="tx">${esc(u.why)}</div></div><div class="tags">${obs("unmapped")}${u.ex ? obs("excluded") : ""}</div></a>`;
const count = () => "<b>4</b> tools, <b>2</b> unmapped entries";
const tools = (w) =>
  `<section class="blk"><h2 class="sec">Tools</h2><div class="list" data-part="tools-list">${TOOLS.map((t) => toolRow(t, w)).join("")}</div></section>`;
const unmapped = (w) =>
  `<section class="blk"><h2 class="sec">Unmapped entries</h2><p class="tx2">An entry is in a tool's effective config and no rule explains it.</p><div class="list" data-part="tools-unmapped">${UNMAPPED.map((u) => unRow(u, w)).join("")}</div></section>`;
const key = () =>
  `<aside class="keybox" data-part="tools-key"><h2 class="sec">Observations</h2><ul>${Object.keys(
    OBS,
  )
    .map((k) => `<li>${obs(k)}</li>`)
    .join("")}</ul></aside>`;
function defaultBody(w) {
  const h = head(
    "tools-head",
    ["Tools"],
    "Tools and unmapped entries",
    count(),
  );
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${key()}</aside><div class="col">${tools(w)}${unmapped(w)}</div></div></main>`;
  return `<main class="main">${h}${tools(w)}${unmapped(w)}${w === 900 ? key() : ""}</main>`;
}
const empty = () =>
  message(
    "tools-empty",
    ["Tools"],
    "No rule is bound to a tool",
    [
      "Every rule here is advisory or at rung 6, review, so no tool runs for it. A tool appears as soon as a binding names one.",
      "The rules view shows which rules have no binding.",
    ],
    act("open-rules", "Open the rules"),
  );
const error = () =>
  message(
    "tools-error",
    ["Tools"],
    "This bundle has no tool entries",
    [
      "The bundle was built without the effective config, so bindings cannot be compared with entries. The tools are listed from the bindings alone.",
      'Build it again with <span class="mono">gragas explorer</span>.',
    ],
    act("open-rules", "Open the rules"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "tools-head", "the tools", 4)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "tools",
  title: "Tools",
  nav: "tools",
  route: "/tools",
  body,
  css: `${TABLE_CSS}.oc{display:inline-flex;align-items:center;gap:4px}.n.mono{font-size:calc(12px*var(--tg,1));color:var(--muted)}.blk{display:flex;flex-direction:column;gap:8px}.tx2{font-size:calc(13px*var(--tg,1));color:var(--prose)}.keybox ul{list-style:none;display:flex;flex-direction:column;gap:8px;margin-top:8px}`,
  caps: {
    default:
      "Four tools, how many entries and rules each has, and how a project sets them. Two entries no rule explains.",
    ...COMMON_CAPS,
    empty: "No binding names a tool. It says why and offers the rules.",
    error:
      "A bundle built without the effective config. It says what is missing and how to build again.",
  },
  note: {
    purpose:
      "A rulebook owner or a project owner opens this view to see which tools do the enforcing and where a config has drifted from the rules. It answers: what runs, and what runs that no rule explains?",
    conveys: [
      "Each tool, with how many entries and rules it has and the version range its plugin describes.",
      "How projects set each tool: the observation counts, as a word and a symbol.",
      "The unmapped entries: in a config, with no rule behind them, with the reason where an overlay excluded the rule.",
      "A key to the six observations.",
    ],
    layout: {
      390: "One column. The tools first, as stacked rows with their observation counts under the name, then the unmapped entries. The key is on the observation chips themselves.",
      900: "One column. Tools, unmapped entries, then the key of observations.",
      1440: "The key of the six observations as a rail on the left, the tools and the unmapped entries on the right.",
    },
    standing:
      "Tools have no standing. An observation (as-set, tighter, weaker, absent, unmapped, excluded) is drawn as a word and a symbol (= > < _ . /), never colour alone.",
  },
  differs: [
    'S38 names this view "tools and unmapped entries". The canvas listed rules per tool and counted rules whose checks name no tool, but did not list unmapped entries. This draws the unmapped entries, with the "was excluded by" reason of 17.4.',
    "Standing words and symbols: observations are the six of 17.4, a word and a symbol each. The canvas used coloured chips only.",
    "Navigation: Tools is in the header at 1440 and the pages menu below it. The canvas header also named Guides and Handbook.",
  ],
  parts: [
    {
      name: "tools-head",
      purpose: "the title and how many tools and unmapped entries there are",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "tools-list",
      purpose: "the tools the bindings name",
      measured: { width: m(358, 852, 768) },
    },
    {
      name: "tools-row",
      purpose: "one tool: entries, rules, plugin range and observation counts",
      measured: { "min-height": m(64, 56, 52), count: m(4, 4, 4) },
    },
    {
      name: "tools-unmapped",
      purpose: "entries in a config that no rule explains",
      measured: { count: m(2, 2, 2) },
    },
    {
      name: "tools-unmapped-row",
      purpose: "one unmapped entry, with why it is unmapped",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "tools-key",
      purpose: "the six observations as word and symbol",
      measured: { count: m(null, 6, 6), width: m(null, 852, 264) },
    },
    {
      name: "tools-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "tools-empty",
      purpose: "no binding names a tool",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "tools-error",
      purpose: "a bundle without the effective config",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-rules",
      purpose: "leaves a state of its view for the rules view",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
