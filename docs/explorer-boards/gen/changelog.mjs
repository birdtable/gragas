// The changelog view: per release and per rule, with the consumer action and the remedy
// (S11, spec 6 "Release", "Consumer action"). The bump is computed, never typed.
import { esc, stand } from "./lib.mjs";
import {
  m,
  head,
  skeleton,
  message,
  act,
  COMMON_CAPS,
  TABLE_CSS,
} from "./kit.mjs";

const RELEASES = [
  {
    v: "v0.1.0",
    when: "unreleased",
    bump: "minor, computed from 4 changes",
    items: [
      {
        id: "go.errors.4",
        kind: "added",
        s: "planned",
        action: "Nothing to do: the rule has no binding yet.",
        remedy: "",
      },
      {
        id: "go.errors.2",
        kind: "tightened",
        s: "enforced",
        action: "Run check. Code that returns a bare error now fails.",
        remedy: "Wrap with the call that failed.",
      },
      {
        id: "go.docs.1",
        kind: "explanation changed",
        s: "enforced",
        action: "Read the new reasoning. Nothing fails.",
        remedy: "",
      },
      {
        id: "go.naming.3",
        kind: "retired",
        s: "retired",
        action:
          "Drop the rule from your overlay. It is replaced by go.naming.4.",
        remedy: "Rebind to go.naming.4.",
      },
    ],
  },
  {
    v: "v0.0.9",
    when: "2026-10-02",
    bump: "patch, computed from 1 change",
    items: [
      {
        id: "go.test.1",
        kind: "translation changed",
        s: "enforced",
        action: "None. Ids and bindings are not translated.",
        remedy: "",
      },
    ],
  },
];
const item = (x, w) =>
  `<div class="brow${w === 390 ? " stack" : ""}" data-part="changelog-entry"><div class="main2"><div class="id">${x.id}</div><div class="tx" data-part="changelog-action"><b>Consumer action.</b> ${esc(x.action)}</div>${x.remedy ? `<div class="tx" data-part="changelog-remedy"><b>Remedy.</b> ${esc(x.remedy)}</div>` : ""}</div><div class="tags"><span class="kt" data-part="changelog-kind">${esc(x.kind)}</span>${stand(x.s)}</div></div>`;
const release = (r, w) =>
  `<section class="blk" data-part="changelog-release"><div class="rh"><h2 class="sec">${r.v} · ${esc(r.when)}</h2><span class="kt" data-part="changelog-bump">bump: ${esc(r.bump)}</span></div><div class="list">${r.items.map((x) => item(x, w)).join("")}</div></section>`;
const key = () =>
  `<aside class="keybox" data-part="changelog-key"><h2 class="sec">Kinds of change</h2><ul><li>added</li><li>tightened</li><li>weakened</li><li>explanation changed</li><li>translation changed</li><li>retired</li></ul><p class="tx2">The bump is computed from the diff of rules, bindings and presets.</p></aside>`;
function defaultBody(w) {
  const h = head(
    "changelog-head",
    ["Changelog"],
    "Changelog",
    "<b>2</b> releases, one unreleased",
  );
  const list = RELEASES.map((r) => release(r, w)).join("");
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${key()}</aside><div class="col">${list}</div></div></main>`;
  return `<main class="main">${h}${list}${w === 900 ? key() : ""}</main>`;
}
const empty = () =>
  message(
    "changelog-empty",
    ["Changelog"],
    "This rulebook has no release",
    [
      "The changelog is written per release. Until the first one, there is nothing to list and no bump to compute.",
      "The rules view shows what the first release will hold.",
    ],
    act("open-rules", "Open the rules"),
  );
const error = () =>
  message(
    "changelog-error",
    ["Changelog", "v0.0.7"],
    "This release is not in the bundle",
    [
      "The bundle holds releases from v0.0.9. A release before it was withdrawn or archived.",
      "Open the changelog to see the releases it does hold.",
    ],
    act("open-changelog", "Open the changelog"),
    "alert",
  );
const body = (w, s) =>
  s === "loading"
    ? skeleton(w, "changelog-head", "the changelog", 4)
    : s === "empty"
      ? empty()
      : s === "error"
        ? error()
        : defaultBody(w);

export const view = {
  key: "changelog",
  title: "Changelog",
  nav: "changelog",
  route: "/changelog",
  body,
  css: `${TABLE_CSS}.blk{display:flex;flex-direction:column;gap:8px}.rh{display:flex;gap:12px;align-items:center;flex-wrap:wrap}.tx2{font-size:calc(13px*var(--tg,1));color:var(--prose)}.keybox ul{list-style:none;display:flex;flex-direction:column;gap:6px;margin:8px 0}`,
  caps: {
    default:
      "Two releases: the one coming, with four changes, and the last, with one. Each change has its consumer action and remedy.",
    ...COMMON_CAPS,
    empty:
      "A rulebook with no release. It says why the list is empty and offers the rules.",
    error:
      "A release that is not in the bundle. It says which ones are, and offers the changelog.",
  },
  note: {
    purpose:
      "A project owner opens this view before an upgrade to learn what a release changes for them. It answers: what changed, and what do I do about it?",
    conveys: [
      "Each release with its date, or unreleased, and the bump the engine computed.",
      "Each change by rule: the kind, the rule's standing as a word and a symbol, the consumer action and the remedy where there is one.",
      "The kinds of change, so a reader knows what tightened means.",
      "That an explanation or translation change is shown but needs no action.",
    ],
    layout: {
      390: "One column. Each release, then its changes as stacked rows with the kind and standing under them.",
      900: "One column, with the key of kinds below the releases.",
      1440: "The key of kinds as a rail on the left, the releases on the right.",
    },
    standing:
      "Every rule carries its standing as a word and a symbol. The kind of change is text in a bordered chip, not colour.",
  },
  differs: [
    "The canvas counted 1,003 rules for 0.1.0 in the Changelog and 996 in the Overview. ProposedChangelog made them agree; this draws that board's idea: the release being prepared is listed first and called unreleased.",
    "The canvas had no consumer action or remedy; both come from S11 and are drawn here.",
  ],
  parts: [
    {
      name: "changelog-head",
      purpose: "the title and how many releases there are",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "changelog-release",
      purpose: "one release: its version, date or unreleased, bump and changes",
      measured: { count: m(2, 2, 2) },
    },
    {
      name: "changelog-bump",
      purpose: "the bump the engine computed for the release",
      measured: { height: m(20, 20, 20) },
    },
    {
      name: "changelog-entry",
      purpose: "one change to one rule",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "changelog-kind",
      purpose: "the kind of change, as text in a chip",
      measured: { height: m(20, 20, 20) },
    },
    {
      name: "changelog-action",
      purpose: "what a project does about the change",
      measured: { count: m(5, 5, 5) },
    },
    {
      name: "changelog-remedy",
      purpose: "how to do it, where there is a way",
      measured: { count: m(2, 2, 2) },
    },
    {
      name: "changelog-key",
      purpose: "the kinds of change",
      measured: { count: m(null, 6, 6), width: m(null, 852, 264) },
    },
    {
      name: "changelog-crumb",
      purpose: "names the view the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "changelog-empty",
      purpose: "a rulebook with no release",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "changelog-error",
      purpose: "a release the bundle does not hold",
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
