// The coverage view: rules to bindings and entries to rules, both ways (spec 17.1, S7). A
// rule is held when it has an effective binding; an entry is explained when a rule says why.
import { esc, stand, obs, markChip } from "./lib.mjs";
import {
  m,
  head,
  skeleton,
  message,
  act,
  COMMON_CAPS,
  TABLE_CSS,
} from "./kit.mjs";

const UNHELD = [
  {
    id: "go.errors.4",
    tx: "Wrap an error with the call that failed.",
    s: "planned",
    marks: ["unproven"],
  },
  {
    id: "go.docs.2",
    tx: "Every exported name has a comment.",
    s: "planned",
    marks: [],
  },
  {
    id: "go.test.3",
    tx: "A test reads a fixture, never writes one.",
    s: "planned",
    marks: ["trial"],
  },
  {
    id: "go.naming.2",
    tx: "Name a test after what it holds.",
    s: "advisory",
    marks: [],
  },
];
const UNEXPLAINED = [
  { id: "errorlint.asserts", tool: "errorlint" },
  { id: "wrapcheck.ignore-pkg", tool: "wrapcheck" },
  { id: "prose-lint.heading-case", tool: "prose-lint" },
  { id: "prose-lint.max-line", tool: "prose-lint" },
];
const meter = (label, n, of, what) =>
  `<div class="cov" data-part="coverage-meter"><div class="cv"><b>${n}</b> of ${of}</div><div class="cl">${esc(label)}</div><div class="meter" role="img" aria-label="${n} of ${of} ${esc(label.toLowerCase())}"><div class="fill" style="width:${Math.round((100 * n) / of)}%"></div></div><div class="tx2">${esc(what)}</div></div>`;
const meters = () =>
  `<section class="metres" data-part="coverage-meters">${meter("Rules held", 14, 18, "Rules with an effective binding.")}${meter("Entries explained", 17, 21, "Entries in an effective config that a rule explains.")}</section>`;
const ruleRow = (r, w) =>
  `<a class="brow${w === 390 ? " stack" : ""}" data-part="coverage-rule-row"><div class="main2"><div class="id">${r.id}</div><div class="tx">${esc(r.tx)}</div></div><div class="tags">${stand(r.s)}${r.marks.map(markChip).join("")}</div></a>`;
const entryRow = (e, w) =>
  `<a class="brow${w === 390 ? " stack" : ""}" data-part="coverage-entry-row"><div class="main2"><div class="id">${e.id}</div><div class="tx">Entry of ${e.tool}, in the effective config. No rule explains it.</div></div><div class="tags">${obs("unmapped")}</div></a>`;
const rules = (w) =>
  `<section class="blk"><h2 class="sec">Rules with no effective binding</h2><div class="list" data-part="coverage-rules">${UNHELD.map((r) => ruleRow(r, w)).join("")}</div></section>`;
const entries = (w) =>
  `<section class="blk"><h2 class="sec">Entries no rule explains</h2><div class="list" data-part="coverage-entries">${UNEXPLAINED.map((e) => entryRow(e, w)).join("")}</div></section>`;
const sets = () =>
  `<section class="blk" data-part="coverage-sets"><h2 class="sec">Covered sets</h2><div class="brow" data-part="coverage-set-row"><div class="main2"><div class="id">acme-go-base</div><div class="tx">An upstream preset adopted whole at rung 0 counts as one covered set: 12 entries and 6 rules.</div></div></div></section>`;
function defaultBody(w) {
  const h = head(
    "coverage-head",
    ["Coverage"],
    "Coverage",
    "<b>14</b> of 18 rules held, <b>17</b> of 21 entries explained",
  );
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${meters()}${sets()}</aside><div class="col">${rules(w)}${entries(w)}</div></div></main>`;
  return `<main class="main">${h}${meters()}${rules(w)}${entries(w)}${sets()}</main>`;
}
const empty = () =>
  message(
    "coverage-empty",
    ["Coverage"],
    "There is nothing to cover yet",
    [
      "Coverage compares rules with the entries of the tools that enforce them. This rulebook has no binding, so both counts are zero.",
      "A rule is held once a binding names it.",
    ],
    act("open-rules", "Open the rules"),
  );
const error = () =>
  message(
    "coverage-error",
    ["Coverage"],
    "Coverage could not be read",
    [
      "The bundle has bindings but no effective config, so entries cannot be matched to rules. Rules held are known: 14 of 18.",
      'Build the bundle again with <span class="mono">gragas explorer</span>.',
    ],
    act("open-tools", "Open the tools"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "coverage-head", "coverage", 4)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "coverage",
  title: "Coverage",
  nav: "coverage",
  route: "/coverage",
  body,
  css: `${TABLE_CSS}.metres{display:flex;flex-direction:column;gap:16px;margin-bottom:20px}.cov{display:flex;flex-direction:column;gap:6px}.cv{font-size:calc(22px*var(--tg,1))}.cl{font-size:calc(13px*var(--tg,1));font-weight:600}.meter{height:12px;background:var(--surface);border:1px solid var(--line-strong);border-radius:6px;overflow:hidden}.meter .fill{height:100%;background:var(--accent)}.blk{display:flex;flex-direction:column;gap:8px}.tx2{font-size:calc(13px*var(--tg,1));color:var(--prose)}`,
  caps: {
    default:
      "Two counts with their bars, the rules that have no binding, the entries no rule explains, and the covered set.",
    ...COMMON_CAPS,
    empty:
      "A rulebook with no bindings. It says both counts are zero and offers the rules.",
    error:
      "A bundle with no effective config. The rule count is still shown, the entry count is not.",
  },
  note: {
    purpose:
      "A rulebook owner opens this view to see how far the rules and the tool configs agree, both ways. It answers: which rules are held by a tool, and which entries explain nothing?",
    conveys: [
      "Rules held: how many rules have an effective binding, as a count and a bar, never colour alone.",
      "Entries explained: how many entries in an effective config a rule explains.",
      "The rules with no effective binding, each with its standing word and symbol and its marks.",
      "The entries no rule explains, as unmapped, and the upstream preset counted as one covered set.",
    ],
    layout: {
      390: "One column: the two counts stacked, the rules, the entries, then the covered set.",
      900: "One column with the same order; rows have more room for the statement.",
      1440: "The two counts and the covered set as a rail on the left, the two lists on the right.",
    },
    standing:
      "Every rule carries its standing as a word and a symbol; trial and unproven are marks beside it. An entry carries the observation unmapped as a word and a symbol.",
  },
  differs: [
    'The canvas had Coverage and TargetCoverage. The second folded standing into coverage ("Colour by Standing"); standing bars is its own view by hub ruling, so coverage here is the two counts and their lists only.',
    'The canvas filtered by "Later", which is not a standing; this draws the six of 17.2 only, and the marks of 17.3 beside them.',
  ],
  parts: [
    {
      name: "coverage-head",
      purpose: "the title and both counts in a line",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "coverage-meters",
      purpose: "the two counts and their bars",
      measured: { count: m(2, 2, 2), width: m(358, 852, 264) },
    },
    {
      name: "coverage-meter",
      purpose: "one count with its bar and what it counts",
      measured: { "bar-height": m(12, 12, 12) },
    },
    {
      name: "coverage-rules",
      purpose: "the rules with no effective binding",
      measured: { count: m(4, 4, 4) },
    },
    {
      name: "coverage-rule-row",
      purpose: "one rule with no binding: id, statement, standing and marks",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "coverage-entries",
      purpose: "the entries no rule explains",
      measured: { count: m(4, 4, 4) },
    },
    {
      name: "coverage-entry-row",
      purpose: "one unexplained entry and its tool",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "coverage-sets",
      purpose: "upstream presets counted as one covered set",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "coverage-set-row",
      purpose: "one covered set with its entries and rules",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "coverage-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "coverage-empty",
      purpose: "a rulebook with no bindings",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "coverage-error",
      purpose: "a bundle without the effective config",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-rules",
      purpose: "leaves a state of its view for the rules view",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "open-tools",
      purpose: "leaves the error state for the tools view",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
