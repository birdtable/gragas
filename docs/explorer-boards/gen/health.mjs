// The health view: the seven named numbers a release publishes about the rulebook, computed by
// the engine and claimed by the release (S29, spec 6 "Health"). No project is named.
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

const NUMBERS = [
  { n: "Rules held", v: "14 of 18", how: "Rules with an effective binding." },
  {
    n: "Entries explained",
    v: "17 of 21",
    how: "Entries in an effective config that a rule explains.",
  },
  {
    n: "Proof pass rate",
    v: "31 of 32",
    how: "Example pairs that passed across the tested set.",
  },
  {
    n: "Completeness",
    v: "12 of 14",
    how: "Active rules with a source, a pair and a decision.",
  },
  {
    n: "Review-by currency",
    v: "13 of 14",
    how: "Active rules inside their review-by date.",
  },
  { n: "Attention list size", v: "5", how: "Rules that need a maintainer." },
  {
    n: "Median time to active",
    v: "9 days",
    how: "From proposed to active, over the rules that got there.",
  },
];
const card = (x) =>
  `<div class="hcard" data-part="health-card"><div class="hn">${esc(x.n)}</div><div class="hv mono" data-part="health-value">${esc(x.v)}</div><div class="tx2">${esc(x.how)}</div><div class="claim" data-part="health-claim">Claimed by the release: none yet. The engine computes <b>${esc(x.v)}</b> now.</div></div>`;
const grid = () =>
  `<div class="hgrid" data-part="health-grid">${NUMBERS.map(card).join("")}</div>`;
const note = () =>
  `<aside class="keybox" data-part="health-note"><h2 class="sec">How to read these</h2><p class="tx2">The engine computes each number from the bundle. A release claims the same seven. Where the two differ the card says so, and the release is wrong, not the engine. Signals from any project are never folded in, so no number names one.</p></aside>`;
function defaultBody(w) {
  const h = head(
    "health-head",
    ["Health"],
    "Health",
    "Seven numbers, computed now",
  );
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${note()}</aside><div class="col">${grid()}</div></div></main>`;
  return `<main class="main">${h}${grid()}${note()}</main>`;
}
const empty = () =>
  message(
    "health-empty",
    ["Health"],
    "No number can be computed yet",
    [
      "Health counts active rules. This rulebook has none: every rule is proposed or in trial, so the seven numbers would all be zero or undefined.",
      "Open the rules to see what is waiting to be promoted.",
    ],
    act("open-rules", "Open the rules"),
  );
const error = () =>
  message(
    "health-error",
    ["Health"],
    "The release claims a number the engine does not compute",
    [
      "Completeness is claimed as 13 of 14 and the bundle computes 12 of 14. The engine's number is shown; the release has to be made again.",
      "Nothing else differs.",
    ],
    act("open-changelog", "Open the changelog"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "health-head", "health", 4)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "health",
  title: "Health",
  nav: null,
  route: "/health",
  body,
  css: `${TABLE_CSS}.hgrid{display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(240px,1fr))}.hcard{border:1px solid var(--line);border-radius:8px;padding:12px;display:flex;flex-direction:column;gap:6px;background:var(--canvas)}.hn{font-size:calc(13px*var(--tg,1));font-weight:600}.hv{font-size:calc(22px*var(--tg,1))}.claim{font-size:calc(12px*var(--tg,1));color:var(--muted)}.tx2{font-size:calc(13px*var(--tg,1));color:var(--prose)}.keybox{display:flex;flex-direction:column;gap:8px}`,
  caps: {
    default:
      "The seven numbers as cards: a name, the number, how it is counted, and what the release claimed.",
    ...COMMON_CAPS,
    empty:
      "A rulebook with no active rule. It says why no number exists and offers the rules.",
    error:
      "A claim the engine does not reproduce. It names the number and shows the engine's.",
  },
  note: {
    purpose:
      "A rulebook owner or a consumer opens this view to judge how complete the rulebook is. It answers: how healthy is this rulebook, by the seven numbers the release publishes?",
    conveys: [
      "Each of the seven numbers, as a count over a total or a duration, in a code face so digits line up.",
      "How each is counted, in one line.",
      "What the release claimed beside what the engine computes, so a difference is seen.",
      "That no project is named in any number.",
    ],
    layout: {
      390: "One column of cards, the note on reading them last.",
      900: "Three columns of cards, then the note.",
      1440: "The note as a rail on the left, three columns of cards on the right, as wide as the room allows.",
    },
    standing:
      "Health numbers have no standing. Nothing here is conveyed by colour: each card is a name and a number in text.",
  },
  differs: [
    "The canvas had no health board: it is new. It is drawn from S29 and the list of seven numbers.",
  ],
  parts: [
    {
      name: "health-head",
      purpose: "the title and a line saying the numbers are computed now",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "health-grid",
      purpose: "the seven numbers as cards",
      measured: { columns: m(1, 3, 3), count: m(7, 7, 7) },
    },
    {
      name: "health-card",
      purpose: "one number: its name, value, how it is counted and its claim",
      measured: { "min-height": m(120, 120, 120) },
    },
    {
      name: "health-value",
      purpose: "the number itself",
      measured: { "font-size": m(22, 22, 22) },
    },
    {
      name: "health-claim",
      purpose: "what the release claimed for this number",
      measured: { count: m(7, 7, 7) },
    },
    {
      name: "health-note",
      purpose: "how to read the numbers",
      measured: { width: m(358, 852, 264) },
    },
    {
      name: "health-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "health-empty",
      purpose: "a rulebook with no active rule",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "health-error",
      purpose: "a claim the engine does not reproduce",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-rules",
      purpose: "leaves a state of its view for the rules view",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "open-changelog",
      purpose: "leaves the error state for the changelog",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
