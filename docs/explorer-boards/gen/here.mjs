// The here view: one project's fidelity state, read from a local report file. Standalone only
// (ruling 6): the iframe shows the rulebook views. Its empty state is the no-report state.
import { esc, obs } from "./lib.mjs";
import {
  m,
  head,
  skeleton,
  message,
  act,
  COMMON_CAPS,
  TABLE_CSS,
} from "./kit.mjs";

const COUNTS = {
  "as-set": 11,
  tighter: 2,
  weaker: 1,
  absent: 2,
  unmapped: 2,
  excluded: 1,
};
const ENTRIES = [
  {
    id: "errorlint.comparison",
    o: "weaker",
    why: "The rulebook sets it to error and this project sets it to warning.",
  },
  {
    id: "wrapcheck.ignore-sigs",
    o: "absent",
    why: "The rulebook sets it and the effective config does not.",
  },
  {
    id: "errorlint.asserts",
    o: "unmapped",
    why: "In the effective config, no rule explains it.",
  },
  {
    id: "wrapcheck.ignore-pkg",
    o: "excluded",
    why: "Excluded by overlay acme-web, decision web-d4.",
    grey: true,
  },
];
const CHECKS = [
  {
    id: "Tool version",
    kind: "finding",
    why: "errorlint 2.6 is newer than the tested set, which stops at 2.5.",
  },
  {
    id: "Waiver",
    kind: "finding",
    why: "One waiver has no owner and one expires in 6 days.",
  },
  {
    id: "Lock",
    kind: "notice",
    why: "The lock is behind the pin: an upgrade is owed.",
  },
];
const counts = () =>
  `<section class="blk" data-part="here-counts"><h2 class="sec">This project against the rulebook</h2><div class="cnt">${Object.entries(
    COUNTS,
  )
    .map(
      ([k, n]) =>
        `<span class="oc">${obs(k)}<span class="n mono">${n}</span></span>`,
    )
    .join("")}</div></section>`;
const entryRow = (e, w) =>
  `<a class="brow${w === 390 ? " stack" : ""}${e.grey ? " grey" : ""}" data-part="here-entry"><div class="main2"><div class="id">${e.id}</div><div class="tx">${esc(e.why)}</div></div><div class="tags">${obs(e.o)}</div></a>`;
const checkRow = (c, w) =>
  `<div class="brow${w === 390 ? " stack" : ""}" data-part="here-check"><div class="main2"><div class="id plain">${esc(c.id)}</div><div class="tx">${esc(c.why)}</div></div><div class="tags"><span class="kt" data-part="here-kind">${c.kind}</span></div></div>`;
const entries = (w) =>
  `<section class="blk"><h2 class="sec">Entries to look at</h2><div class="list" data-part="here-entries">${ENTRIES.map((e) => entryRow(e, w)).join("")}</div></section>`;
const checks = (w) =>
  `<section class="blk"><h2 class="sec">Checks</h2><div class="list" data-part="here-checks">${CHECKS.map((c) => checkRow(c, w)).join("")}</div></section>`;
const source = () =>
  `<aside class="keybox" data-part="here-source"><h2 class="sec">Report</h2><p class="tx2">Written 2026-11-07 by <span class="mono">gragas report</span>. Rulebook acme-go 0.4.0, lock 0.4.0. Read from a file on this machine; nothing was sent.</p><a class="btn" data-part="here-open">Open another report</a></aside>`;
function defaultBody(w) {
  const h = head(
    "here-head",
    ["Here"],
    "This project",
    "<b>19</b> entries, <b>3</b> to look at",
  );
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${source()}</aside><div class="col">${counts()}${entries(w)}${checks(w)}</div></div></main>`;
  return `<main class="main">${h}${source()}${counts()}${entries(w)}${checks(w)}</main>`;
}
const empty = () =>
  message(
    "here-empty",
    ["Here"],
    "There is no report to show",
    [
      'This view reads a report file that gragas writes for a project, by default <span class="mono">.gragas/report.json</span>. None was found next to the page.',
      'Run <span class="mono">gragas report</span> in the project, or open a report file from another place. The file is read here and never sent.',
    ],
    act("here-open", "Open a report file"),
  );
const error = () =>
  message(
    "here-error",
    ["Here"],
    "This file is not a gragas report",
    [
      "It was read and it has no report sections. It may be a bundle or another JSON file.",
      "Choose the report the report command wrote.",
    ],
    act("here-open", "Open a report file"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "here-head", "the report", 4)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "here",
  title: "Here",
  nav: null,
  route: "/here",
  body,
  css: `${TABLE_CSS}.blk{display:flex;flex-direction:column;gap:8px}.cnt{display:flex;flex-wrap:wrap;gap:12px}.oc{display:inline-flex;align-items:center;gap:4px}.n.mono{font-size:calc(12px*var(--tg,1));color:var(--muted)}.brow.grey{opacity:.72}.tx2{font-size:calc(13px*var(--tg,1));color:var(--prose)}.keybox{display:flex;flex-direction:column;gap:8px}`,
  caps: {
    default:
      "A project against its rulebook: the six observation counts, the entries to look at (one excluded, greyed, with the reason), and the checks.",
    ...COMMON_CAPS,
    empty:
      "The no-report state: no file was found. It says where it looks, how one is written and offers to open one.",
    error:
      "A file that is not a report. It says what it found and offers to open another.",
  },
  note: {
    purpose:
      "A project owner opens this view, on their own machine, to see how their project stands against its rulebook. It answers: where do I differ, and what does check say?",
    conveys: [
      "The six observations with how many entries have each, as a word and a symbol.",
      "The entries to look at: weaker, absent and unmapped, and an excluded one greyed with the overlay and decision that excluded it.",
      "The checks: a finding fails a gate, a notice never does.",
      "Where the report came from, and that it was read here and sent nowhere.",
    ],
    layout: {
      390: "One column: where the report came from, the counts, the entries, the checks.",
      900: "The same column with wider rows.",
      1440: "The report source as a rail on the left, the counts, the entries and the checks on the right.",
    },
    standing:
      "Observations are the six of 17.4, a word and a symbol each. A finding or a notice is text in a chip, never colour alone.",
  },
  differs: [
    "The canvas had no here board: it is new, and by ruling 6 it is standalone only, so it does not appear in the iframe packaging.",
    "Excluded is drawn greyed with its reason (S33), and the grey is not the only sign: the word and the symbol / are drawn too.",
  ],
  parts: [
    {
      name: "here-head",
      purpose: "the title and how many entries and how many to look at",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "here-source",
      purpose: "where the report came from, and the way to open another",
      measured: { width: m(358, 852, 264) },
    },
    {
      name: "here-open",
      purpose: "chooses a report file on this machine",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "here-counts",
      purpose: "the six observations with their counts",
      measured: { count: m(6, 6, 6) },
    },
    {
      name: "here-entries",
      purpose: "the entries that differ from the rulebook",
      measured: { count: m(4, 4, 4) },
    },
    {
      name: "here-entry",
      purpose: "one entry, its observation and why",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "here-checks",
      purpose: "what check says about the project",
      measured: { count: m(3, 3, 3) },
    },
    {
      name: "here-check",
      purpose: "one check, as a finding or a notice",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "here-kind",
      purpose: "finding or notice, as text in a chip",
      measured: { height: m(20, 20, 20) },
    },
    {
      name: "here-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "here-empty",
      purpose: "the no-report state",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "here-error",
      purpose: "a file that is not a report",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
  ],
};
