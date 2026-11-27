// The terms view: every word the rulebook defines once, with the rules that use it
// (S38, spec 6 "Term"). A term may carry a literal so a statement stays neutral.
import { esc } from "./lib.mjs";
import {
  m,
  head,
  skeleton,
  message,
  act,
  COMMON_CAPS,
  TABLE_CSS,
} from "./kit.mjs";

const TERMS = [
  {
    t: "wrapped error",
    d: "An error that holds the error it was made from.",
    used: 4,
  },
  {
    t: "sentinel error",
    d: "A named error value that callers compare against.",
    used: 2,
  },
  {
    t: "package boundary",
    d: "Where an exported function returns to code in another package.",
    used: 6,
  },
  {
    t: "request id header",
    d: "The header that carries a request id.",
    lit: "X-Request-Id",
    used: 3,
  },
  { t: "fixture", d: "A file a test reads and does not write.", used: 5 },
];
const row = (x, w) =>
  `<a class="brow${w === 390 ? " stack" : ""}" data-part="terms-row"><div class="main2"><div class="id plain">${esc(x.t)}</div><div class="tx">${esc(x.d)}</div>${x.lit ? `<div class="tx">Literal <span class="mono lit">${esc(x.lit)}</span></div>` : ""}</div><div class="tags"><span class="kt" data-part="terms-used">used by ${x.used} rules</span></div></a>`;
const list = (w) =>
  `<div class="list" data-part="terms-list">${TERMS.map((x) => row(x, w)).join("")}</div>`;
const index = () =>
  `<nav class="kinds" data-part="terms-index" aria-label="Jump to a letter">${["F", "P", "R", "S", "W"].map((l) => `<a class="kind" data-part="terms-letter">${l}</a>`).join("")}</nav>`;
function defaultBody(w) {
  const h = head(
    "terms-head",
    ["Terms"],
    "Terms",
    `<b>${TERMS.length}</b> terms`,
  );
  const find = `<span class="search" data-part="terms-find" style="height:${w === 1440 ? 32 : 44}px;max-width:420px">Find a term</span>`;
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${index()}</aside><div class="col">${find}${list(w)}</div></div></main>`;
  return `<main class="main">${h}${find}${w === 900 ? index() : ""}${list(w)}</main>`;
}
const empty = () =>
  message(
    "terms-empty",
    ["Terms"],
    "This rulebook defines no terms",
    [
      "A term is a word the rulebook defines once, so a rule can use it and mean one thing.",
      "Rules that use a word nobody defined are listed on the attention view.",
    ],
    act("open-attention", "Open attention"),
  );
const error = () =>
  message(
    "terms-error",
    ["Terms", "drift"],
    "No term drift",
    [
      "This rulebook defines no term with this name. A term that was renamed keeps its old name as an alias.",
      "Search for it, or open the list of terms.",
    ],
    act("open-terms", "Open the terms"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "terms-head", "the terms", 5)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "terms",
  title: "Terms",
  nav: null,
  route: "/terms",
  body,
  css: `${TABLE_CSS}.lit{background:var(--surface);padding:0 4px;border-radius:3px}`,
  caps: {
    default:
      "Five terms, each defined once, with how many rules use it and its literal where it has one.",
    ...COMMON_CAPS,
    empty:
      "A rulebook with no terms. It says what a term is and where unused words show up.",
    error:
      "A name the rulebook does not define. It says so, mentions aliases and offers the list.",
  },
  note: {
    purpose:
      "A rule author or a reader opens this view to learn what a word means in this rulebook. It answers: what does this word mean here, and where is it used?",
    conveys: [
      "The term, in plain type, with its one definition.",
      "Its literal, where it has one, in a code face, so a header name or a package is exact.",
      "How many rules use it, to see which words matter.",
      "A way to find a term: search, and on larger screens a letter index.",
    ],
    layout: {
      390: "One column. Find a term first, then the terms as stacked rows. No letter index: the list is searched.",
      900: "One column. A letter index above the list, so a long list is crossed in one step.",
      1440: "The letter index as a rail on the left, the search and the terms on the right.",
    },
    standing:
      "A term has no standing, so its row carries its use count as text only.",
  },
  differs: [
    "The canvas had no board for this view: it is new. It is drawn from S38 and the vocabulary of the spec (term, literal, alias).",
  ],
  parts: [
    {
      name: "terms-head",
      purpose: "the title and how many terms there are",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "terms-find",
      purpose: "finds a term by its words",
      measured: { height: m(44, 44, 32), "max-width": m(358, 420, 420) },
    },
    {
      name: "terms-index",
      purpose: "jumps to the terms of one letter",
      measured: { count: m(null, 1, 1), width: m(null, 852, 264) },
    },
    {
      name: "terms-letter",
      purpose: "one letter of the index",
      measured: { height: m(null, 44, 32) },
    },
    {
      name: "terms-list",
      purpose: "the terms, one row each",
      measured: { width: m(358, 852, 768) },
    },
    {
      name: "terms-row",
      purpose: "one term: its name, definition, literal and use count",
      measured: { "min-height": m(64, 56, 52), count: m(5, 5, 5) },
    },
    {
      name: "terms-used",
      purpose: "how many rules use the term",
      measured: { height: m(20, 20, 20) },
    },
    {
      name: "terms-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "terms-empty",
      purpose: "a rulebook with no terms",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "terms-error",
      purpose: "a name the rulebook does not define",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-attention",
      purpose: "leaves the empty state for the attention view",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "open-terms",
      purpose: "leaves the error state for the terms list",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
