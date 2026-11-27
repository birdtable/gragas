// The rule view: one rule with its anatomy, bindings, proof grid, history, relations
// and translations (S38, spec 6, 17). One page, reached from the rules view.
import { stand, obs, markChip, esc, crumb, bar } from "./lib.mjs";

const R = {
  id: "go.errors.4",
  standing: "enforced",
  trial: true,
  statement: "Wrap an error with %w when it crosses a package boundary.",
  facts: [
    "must",
    "Part go.errors",
    "Kinds service, library",
    "Owner alex",
    "Review by 2027-03-01",
  ],
};
const SECTIONS = [
  "Anatomy",
  "Bindings",
  "Proof grid",
  "History",
  "Relations",
  "Translations",
];
const ANATOMY = [
  [
    "Definition",
    "A package boundary is where an exported function returns an error to code in another package.",
  ],
  [
    "Pros",
    "A caller can unwrap the cause and test it with errors.Is. The trace keeps every layer.",
  ],
  [
    "Cons",
    "A wrapped error exposes its cause to callers, who may come to depend on it.",
  ],
  ["Decision", "wrap-errors: wrap at package edges so a caller can unwrap."],
];
const BINDINGS = [
  {
    tool: "errorlint",
    key: "errorf-multi",
    rung: 1,
    run: "commit",
    sev: "warning",
    obs: "as-set",
  },
  {
    tool: "wrapcheck",
    key: "ignore-sigs",
    rung: 2,
    run: "push",
    sev: "error",
    obs: "tighter",
  },
];
const GRID = {
  cols: ["v1.64 i", "v1.64 s", "v1.62 i"],
  rows: [
    ["errorlint", ["+", "+", "+"]],
    ["wrapcheck", ["+", "!", "."]],
  ],
};
const GLOSS = { "+": "pass", "!": "fail", ".": "not run", "~": "reviewed" };
const HISTORY = [
  ["2026-10-30", "Promoted to trial", "decision wrap-errors"],
  ["2026-10-12", "Bound to wrapcheck at rung 2", "by alex"],
  ["2026-09-21", "Proposed", "by alex, source Handling errors across packages"],
];
const RELS = [
  ["depends on", "go.code.3", "enforced"],
  ["relates to", "go.errors.2", "enforced"],
  ["conflicts with", "go.errors.9", "planned"],
];
const TRANS = [
  ["Deutsch", "current"],
  ["Francais", "stale"],
];

const trialChip = markChip("trial");
const head = (w, id = R.id, st = true) =>
  `<section class="focus" data-part="rule-head">${crumb("rule-crumb", ["Rules", id])}<div class="t"><h1>${id}</h1>${st ? stand(R.standing, { big: true, trial: true }) + trialChip : ""}</div><p class="stmt">${esc(R.statement)}</p><div class="facts">${R.facts.map((f) => `<span>${esc(f)}</span>`).join("")}</div></section>`;
const sec = (name, inner, n = "") =>
  `<section class="rsec" data-part="rule-section" id="${name}"><h2 class="gh">${name}<span class="n">${n}</span></h2>${inner}</section>`;

function contents(w) {
  const items = SECTIONS.map(
    (s) => `<a class="kind" data-part="rule-contents-item">${s}</a>`,
  ).join("");
  if (w === 1440)
    return `<nav class="rail" data-part="rule-contents" aria-label="On this page"><h2>On this page</h2>${items}</nav>`;
  if (w === 900)
    return `<nav class="kinds" data-part="rule-contents" aria-label="On this page">${SECTIONS.map((s) => `<a class="kind" data-part="rule-contents-item">${s}</a>`).join("")}</nav>`;
  return `<span class="btn" data-part="rule-contents" style="width:100%;justify-content:space-between"><span>On this page</span><span aria-hidden="true">+</span></span>`;
}
const anatomy = () =>
  sec(
    "Anatomy",
    `<dl class="anat">${ANATOMY.map(([k, v]) => `<dt>${k}</dt><dd>${esc(v)}</dd>`).join("")}</dl><div class="pair2" data-part="rule-example"><figure class="ex bad"><figcaption><i aria-hidden="true">!</i> Does not pass</figcaption><pre>return fmt.Errorf("read: %v", err)</pre></figure><figure class="ex good"><figcaption><i aria-hidden="true">+</i> Passes</figcaption><pre>return fmt.Errorf("read: %w", err)</pre></figure></div>`,
  );
function bindings(w) {
  const rows = BINDINGS.map(
    (b) =>
      `<div class="brow" data-part="binding-row"><div class="main2"><div class="id">${b.tool}</div><div class="tx">${b.key} · rung ${b.rung} · run at ${b.run} · ${b.sev}</div></div><div class="tags">${obs(b.obs)}</div></div>`,
  ).join("");
  return sec(
    "Bindings",
    `<div data-part="rule-bindings">${rows}</div>`,
    BINDINGS.length,
  );
}
function grid() {
  const head = `<tr><th></th>${GRID.cols.map((c) => `<th>${c}</th>`).join("")}</tr>`;
  const rows = GRID.rows
    .map(
      ([t, cs]) =>
        `<tr><th>${t}</th>${cs.map((c) => `<td data-part="proof-cell"><span class="pc ${GLOSS[c].replace(" ", "-")}"><i aria-hidden="true">${c}</i>${GLOSS[c]}</span></td>`).join("")}</tr>`,
    )
    .join("");
  return sec(
    "Proof grid",
    `<div class="scroll"><table class="pg" data-part="proof-grid">${head}${rows}</table></div><p class="cap2">Columns are the tool version and the run level, isolated (i) or sandboxed (s).</p>`,
  );
}
const history = () =>
  sec(
    "History",
    `<ol class="hist" data-part="rule-history">${HISTORY.map(([d, w, by]) => `<li data-part="history-row"><span class="mono d">${d}</span><span>${w}</span><span class="by">${by}</span></li>`).join("")}</ol>`,
    HISTORY.length,
  );
const relations = () =>
  sec(
    "Relations",
    `<div data-part="rule-relations">${RELS.map(([e, id, st]) => `<a class="brow"><div class="main2"><div class="id">${id}</div><div class="tx">${e}</div></div><div class="tags">${stand(st)}</div></a>`).join("")}</div><a class="clr" data-part="open-relations">Show all 13 on the relations view</a>`,
    13,
  );
const trans = () =>
  sec(
    "Translations",
    `<div data-part="rule-translations">${TRANS.map(([l, s]) => `<div class="brow" data-part="translation-row"><div class="main2"><div class="id plain">${l}</div></div><div class="tags"><span class="kt">${s === "stale" ? "! stale, the text changed since it was translated" : "+ current"}</span></div></div>`).join("")}</div>`,
    TRANS.length,
  );

function defaultBody(w) {
  const sections = [
    anatomy(),
    bindings(w),
    grid(),
    history(),
    relations(),
    trans(),
  ].join("");
  if (w === 1440)
    return `<main class="main">${head(w)}<div class="body">${contents(w)}<div class="col rcol">${sections}</div></div></main>`;
  return `<main class="main">${head(w)}${contents(w)}${sections}</main>`;
}
function skeleton(w) {
  const blk = (h) => `<div class="sk" style="height:${h}px"></div>`;
  const h = `<section class="focus" data-part="rule-head" aria-busy="true"><div style="display:flex;gap:12px;align-items:center">${bar("200px", 32)}${bar("104px", 32)}</div>${bar("70%", 18)}<div class="live" role="status"><span aria-hidden="true">...</span>Loading the rule</div></section>`;
  const col = `${blk(120)}${blk(72)}${blk(96)}${blk(72)}`;
  if (w === 1440)
    return `<main class="main">${h}<div class="body"><aside class="rail">${Array.from({ length: 6 }, () => `<div class="sk" style="height:32px;margin-top:4px"></div>`).join("")}</aside><div class="col rcol">${col}</div></div></main>`;
  return `<main class="main">${h}${col}</main>`;
}
function empty(w) {
  const none = (t) => `<p class="none">${t}</p>`;
  const s = [
    sec(
      "Anatomy",
      `<dl class="anat"><dt>Definition</dt><dd>A fact held by a policy tool is a value the tool reads, not code it runs.</dd><dt>Decision</dt><dd>None yet. A proposed rule names a decision before it is promoted.</dd></dl>`,
    ),
    sec(
      "Bindings",
      none(
        "No bindings. A proposed rule ships in no preset until a binding is made and proved.",
      ),
      0,
    ),
    sec(
      "Proof grid",
      none("Nothing to prove. The grid appears with the first binding."),
    ),
    sec(
      "History",
      `<ol class="hist" data-part="rule-history"><li data-part="history-row"><span class="mono d">2026-11-02</span><span>Proposed</span><span class="by">by alex</span></li></ol>`,
      1,
    ),
    sec("Relations", none("No relations yet."), 0),
    sec(
      "Translations",
      none("No translations. The authoritative language is English."),
      0,
    ),
  ].join("");
  const hd = `<section class="focus" data-part="rule-head">${crumb("rule-crumb", ["Rules", "go.test.14"])}<div class="t"><h1>go.test.14</h1>${stand("proposed", { big: true })}</div><p class="stmt">Keep a fixture beside the test that reads it.</p><div class="facts"><span>should</span><span>Part go.test</span><span>Owner sam</span></div></section>`;
  if (w === 1440)
    return `<main class="main">${hd}<div class="body">${contents(w)}<div class="col rcol">${s}</div></div></main>`;
  return `<main class="main">${hd}${contents(w)}${s}</main>`;
}
function error(w) {
  return `<main class="main"><section class="msg" data-part="rule-error" role="alert">${crumb("rule-crumb", ["Rules", "go.errors.99"])}<h1>No rule go.errors.99</h1><p>This rulebook has no rule with this id, and no rule that was renumbered from it.</p><p>Check the id in the link, or search for it. A renumbered rule keeps its old id as an alias, so the old link would have found it.</p><div class="acts"><a class="btn p" data-part="open-rules">Open the rules</a><span class="search" data-part="search-action" style="height:44px">Search for a rule</span></div></section></main>`;
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
  key: "rule",
  title: "Rule",
  nav: "rules",
  route: "/rules/:id",
  body,
  css: `
.rcol{gap:32px}.rsec{display:flex;flex-direction:column;gap:12px;scroll-margin-top:72px}
.anat{display:grid;grid-template-columns:max-content 1fr;gap:8px 16px;font-size:calc(14px*var(--tg,1))}.anat dt{color:var(--muted);font-weight:600}.anat dd{color:var(--prose);max-width:62ch}
@media (max-width:480px){.anat{grid-template-columns:1fr}}
.pair2{display:flex;gap:12px;flex-wrap:wrap}.ex{flex:1 1 220px;border:1px solid var(--line-strong);border-radius:6px;overflow:hidden;margin:0}
.ex figcaption{display:flex;gap:8px;align-items:center;padding:6px 12px;font-size:calc(12px*var(--tg,1));font-weight:600;border-bottom:1px solid var(--line)}
.ex figcaption i{font-style:normal;font-family:"IBM Plex Mono",monospace;font-weight:700}
.ex.bad figcaption{color:var(--danger);background:var(--danger-soft)}.ex.good figcaption{color:var(--s-enforced);background:var(--s-enforced-soft)}
.ex pre{margin:0;padding:12px;font-family:"IBM Plex Mono",monospace;font-size:calc(12px*var(--tg,1));white-space:pre-wrap;overflow-wrap:anywhere;background:var(--surface)}
.brow{display:flex;gap:12px;align-items:flex-start;min-height:var(--row);padding:10px 4px;border-bottom:1px solid var(--line);color:inherit;text-decoration:none}
.brow .id{font-family:"IBM Plex Mono",monospace;font-size:calc(13px*var(--tg,1));font-weight:500}.brow .id.plain{font-family:"IBM Plex Sans",sans-serif}
.brow .tx{font-size:calc(13px*var(--tg,1));color:var(--prose);margin-top:2px}.brow .main2{flex:1;min-width:0}.brow .tags{display:flex;gap:6px;flex-wrap:wrap;justify-content:flex-end}
.scroll{overflow-x:auto}.pg{border-collapse:collapse;font-size:calc(13px*var(--tg,1));min-width:100%}.pg th,.pg td{border:1px solid var(--line);padding:6px 12px;text-align:left;white-space:nowrap}.pg th{background:var(--surface);font-weight:600}
.pc{display:inline-flex;align-items:center;gap:6px;min-height:24px}.pc i{font-style:normal;font-family:"IBM Plex Mono",monospace;font-weight:700}.pc.fail{color:var(--danger)}.pc.pass{color:var(--s-enforced)}.pc.not-run{color:var(--muted)}
.cap2{font-size:calc(12px*var(--tg,1));color:var(--muted)}
.hist{list-style:none;display:flex;flex-direction:column}.hist li{display:flex;flex-wrap:wrap;gap:4px 16px;padding:10px 4px;border-bottom:1px solid var(--line);font-size:calc(14px*var(--tg,1))}.hist .d{color:var(--muted);font-size:calc(13px*var(--tg,1))}.hist .by{color:var(--muted)}
.none{font-size:calc(14px*var(--tg,1));color:var(--muted);padding:8px 4px}
.clr{color:var(--accent);text-decoration:underline;display:inline-flex;align-items:center;min-height:var(--ctl)}
.mk.unproven{border-style:dotted}.mk.blocked{border-color:var(--danger);color:var(--danger)}`,
  caps: {
    default:
      "The rule go.errors.4, enforced with the trial mark, with every section drawn: anatomy, bindings, proof grid, history, relations and translations.",
    loading:
      "The same frame as grey blocks and one line that says what is loading. The region is marked busy.",
    empty:
      "A proposed rule with nothing yet. Each section says what is missing and why, so an empty section is never blank.",
    error:
      "An id the rulebook does not know, and no alias for it. It names the id and offers the rules and search.",
    stale:
      "A bundle from an older contract major. It is read, with a banner above the view.",
    grown:
      "Default with text at 200 percent. The tables scroll inside their own box and the page never scrolls sideways.",
  },
  note: {
    purpose:
      "A maintainer, a reviewer or a project owner opens this view from the rules view to learn one rule: what it says, why, what enforces it and how well that is proved. It answers: can I trust this rule, and what does it ask of me?",
    conveys: [
      "The rule by id, its standing with its marks, and its statement, largest and first.",
      "What it says and why, in the anatomy: definition, pros, cons, decision, and an example pair.",
      "What enforces it and what proves it: the bindings, then the proof grid.",
      "How it came to be and what it touches: the history, the relations and the translations.",
    ],
    layout: {
      390: "One column. The id, standing and statement first, one On this page button, then the six sections in the order above. The tables become rows, and the proof grid scrolls inside its own box.",
      900: "One column. The six sections are anchor chips under the head, then the sections at full width.",
      1440: "The head across the top. The contents rail on the left, then the sections in one column. The eye goes from the id to what the rule says, then to its proof.",
    },
    standing:
      "The head carries the standing as a word and a symbol, and the trial mark as a hollow symbol with the word. A binding carries its observation the same way. A proof cell carries its result as a symbol and a word.",
  },
  differs: [
    "The history section is on the rule view, as the hub ruled from the spec (S38 lists it). The canvas drew a history section only on its Changes and ProposedRule boards.",
    "Standing words: the spec's six, with a symbol beside each. The canvas drew a word on a coloured chip only.",
    "Marks (17.3): trial is drawn beside the standing. The canvas drew none.",
    "Tablet: drawn. The canvas had no tablet board for the rule view.",
    "The change commands the canvas showed (rule edit, rule replace) are not drawn: they are commands, and the explorer reads the contract only (S16).",
  ],
  parts: [
    {
      name: "rule-crumb",
      purpose: "names the view and the rule the reader is in",
      measured: { count: m(1, 1, 1) },
    },
    {
      name: "rule-head",
      purpose: "the rule by id, standing, marks and statement",
      measured: { "id-size": m(24, 28, 28), count: m(1, 1, 1) },
    },
    {
      name: "rule-contents",
      purpose: "moves to a section of the rule",
      measured: { width: m(358, 852, 264) },
    },
    {
      name: "rule-contents-item",
      purpose: "one section anchor",
      measured: { count: m(null, 6, 6), height: m(null, 44, 32) },
    },
    {
      name: "rule-section",
      purpose: "one of the six sections",
      measured: { count: m(6, 6, 6) },
    },
    {
      name: "rule-example",
      purpose: "the example pair, one that does not pass and one that does",
      measured: { count: m(1, 1, 1), "min-width": m(220, 220, 220) },
    },
    {
      name: "rule-bindings",
      purpose: "the tools that enforce the rule",
      measured: { "row-min-height": m(64, 56, 52) },
    },
    {
      name: "binding-row",
      purpose: "one binding: tool, key, rung, run point, severity, observation",
      measured: { count: m(2, 2, 2) },
    },
    {
      name: "proof-grid",
      purpose: "tool by tool version and run level, each cell a result",
      measured: { columns: m(3, 3, 3), rows: m(2, 2, 2) },
    },
    {
      name: "proof-cell",
      purpose: "one result as a symbol and a word",
      measured: { "min-height": m(24, 24, 24) },
    },
    {
      name: "rule-history",
      purpose: "what happened to the rule, newest first",
      measured: { count: m(3, 3, 3) },
    },
    {
      name: "history-row",
      purpose: "one event with its date and who",
      measured: { count: m(3, 3, 3) },
    },
    {
      name: "rule-relations",
      purpose: "the nearest relations, with a way to the relations view",
      measured: { count: m(3, 3, 3) },
    },
    {
      name: "open-relations",
      purpose: "leaves for the relations view centred on this rule",
      measured: { height: m(44, 44, 32) },
    },
    {
      name: "rule-translations",
      purpose:
        "the languages the rule is translated into and whether each is current",
      measured: { count: m(2, 2, 2) },
    },
    {
      name: "translation-row",
      purpose: "one language and its state",
      measured: { "min-height": m(64, 56, 52) },
    },
    {
      name: "rule-error",
      purpose: "an id the rulebook does not know",
      measured: { "max-width": m("56ch", "56ch", "56ch") },
    },
    {
      name: "open-rules",
      purpose: "leaves a state of its view for the rules view",
      measured: { height: m(44, 44, 32) },
    },
  ],
};
