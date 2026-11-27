// The attention view: the rules that need a maintainer, in the eight kinds of S13. A kind
// that cannot fire (high override rate needs two report batches) is shown and says so.
import { esc, stand, markChip } from "./lib.mjs";
import {
  m,
  head,
  skeleton,
  message,
  act,
  COMMON_CAPS,
  TABLE_CSS,
} from "./kit.mjs";

const KINDS = [
  ["Unproven binding", 1],
  ["Missing pair, source or decision", 1],
  ["Overdue review-by", 1],
  ["Planned past zero-by", 1],
  ["Trial past its window", 1],
  ["Blocked binding", 0],
  ["High override rate", "two batches needed"],
  ["Held by a linter, still in review text", 0],
];
const ITEMS = [
  {
    id: "go.errors.3",
    s: "enforced",
    marks: ["unproven"],
    kind: "Unproven binding",
    why: "The binding for errorlint failed its pair on v2.5.",
    fix: "Prove it again, or rebind it.",
  },
  {
    id: "go.test.2",
    s: "planned",
    marks: [],
    kind: "Missing pair, source or decision",
    why: "The rule has no example pair.",
    fix: "Add a good and a bad example.",
  },
  {
    id: "go.docs.4",
    s: "enforced",
    marks: [],
    kind: "Overdue review-by",
    why: "Review-by was 2026-10-28, twelve days ago.",
    fix: "Confirm, change or retire the rule.",
  },
  {
    id: "go.naming.1",
    s: "planned",
    marks: [],
    kind: "Planned past zero-by",
    why: "Zero-by was 2026-10-31 and nothing binds the rule.",
    fix: "Bind it, or move the date with a decision.",
  },
  {
    id: "go.api.2",
    s: "enforced",
    marks: ["trial"],
    kind: "Trial past its window",
    why: "In trial for 2 releases; the window is 1.",
    fix: "Promote it, or retire it.",
  },
];
const chip = ([k, n]) =>
  `<a class="kind" data-part="attention-kind"><span>${esc(k)}</span><span class="n mono">${typeof n === "number" ? n : esc(n)}</span></a>`;
const kinds = (w) =>
  `<nav class="kinds${w === 390 ? " wrap" : ""}" data-part="attention-kinds" aria-label="Kinds of attention">${KINDS.map(chip).join("")}</nav>`;
const row = (x, w) =>
  `<a class="brow${w === 390 ? " stack" : ""}" data-part="attention-row"><div class="main2"><div class="id">${x.id}</div><div class="tx" data-part="attention-why"><b>${esc(x.kind)}.</b> ${esc(x.why)}</div><div class="tx"><b>Fix.</b> ${esc(x.fix)}</div></div><div class="tags">${stand(x.s)}${x.marks.map(markChip).join("")}</div></a>`;
const list = (w) =>
  `<div class="list" data-part="attention-list">${ITEMS.map((x) => row(x, w)).join("")}</div>`;
function defaultBody(w) {
  const h = head(
    "attention-head",
    ["Attention"],
    "Attention",
    "<b>5</b> rules need a maintainer",
  );
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${kinds(w)}</aside><div class="col">${list(w)}</div></div></main>`;
  return `<main class="main">${h}${kinds(w)}${list(w)}</main>`;
}
const empty = () =>
  message(
    "attention-empty",
    ["Attention"],
    "Nothing needs a maintainer",
    [
      "No rule is unproven, overdue, past a date or blocked. The eight kinds were all checked.",
      "One kind, a high override rate, is only checked when two report batches are given.",
    ],
    act("open-health", "Open health"),
  );
const error = () =>
  message(
    "attention-error",
    ["Attention", "go.errors.9"],
    "No rule has this id",
    [
      "The attention list names rules by id, and the bundle has no rule go.errors.9. It may have been renumbered; an old id keeps its alias.",
      "Open the list to see the rules that need a maintainer.",
    ],
    act("open-attention", "Open attention"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "attention-head", "the attention list", 5)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "attention",
  title: "Attention",
  nav: null,
  route: "/attention",
  body,
  css: `${TABLE_CSS}.kinds{display:flex;flex-direction:column;gap:4px}.kinds.wrap{flex-direction:row;flex-wrap:wrap;gap:6px}.kind{display:inline-flex;justify-content:space-between;gap:12px;align-items:center;min-height:var(--ctl);padding:0 10px;border:1px solid var(--line-strong);border-radius:6px;font-size:calc(13px*var(--tg,1));color:var(--ink)}.kind .n{color:var(--muted)}`,
  caps: {
    default:
      "Five rules, each with its kind, why it is listed, and what to do. The eight kinds as a filter with their counts.",
    ...COMMON_CAPS,
    empty:
      "A rulebook where nothing needs a maintainer. It says what was checked and the one kind that needs two batches.",
    error:
      "An id the bundle does not hold. It mentions aliases and offers the list.",
  },
  note: {
    purpose:
      "A rulebook maintainer opens this view to learn which rules need them now. It answers: what is out of date, unproven or past a date, and what do I do?",
    conveys: [
      "The eight kinds, each with how many rules it holds. The kind that needs two batches says so instead of showing a zero.",
      "Each rule: its id, its kind, why it is listed and the fix, in text.",
      "The rule's standing as a word and a symbol, and trial and unproven as marks.",
      "That the list is derived: nothing is typed into it.",
    ],
    layout: {
      390: "One column. The kinds wrap above the list as chips, then the rules as stacked rows.",
      900: "One column. The kinds as a wrapped row above the list.",
      1440: "The kinds as a rail on the left, the list on the right.",
    },
    standing:
      "Every rule carries its standing as a word and a symbol; marks sit beside it. A kind is a word with a count, never colour alone.",
  },
  differs: [
    "The canvas had no attention board: it is new. It is drawn from S13 and its eight kinds, and the attention item for a rule a linter now holds that still sits in review text.",
  ],
  parts: [
    {
      name: "attention-head",
      purpose: "the title and how many rules need a maintainer",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "attention-kinds",
      purpose: "the eight kinds with their counts",
      measured: { count: m(8, 8, 8), width: m(358, 852, 264) },
    },
    {
      name: "attention-kind",
      purpose: "one kind and how many rules it holds",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "attention-list",
      purpose: "the rules that need a maintainer",
      measured: { count: m(5, 5, 5), width: m(358, 852, 768) },
    },
    {
      name: "attention-row",
      purpose: "one rule: id, kind, why, fix, standing and marks",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "attention-why",
      purpose: "the kind and why the rule is listed",
      measured: { count: m(5, 5, 5) },
    },
    {
      name: "attention-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "attention-empty",
      purpose: "a rulebook where nothing needs a maintainer",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "attention-error",
      purpose: "an id the bundle does not hold",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-health",
      purpose: "leaves the empty state for the health view",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "open-attention",
      purpose: "leaves the error state for the attention list",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
