// Shared parts of the explorer boards: the style, the header, the standing chips, the
// frame in each state and theme, the note beside a frame, and the page. A view module
// (rules.mjs, relations.mjs, ...) gives its own body, its note and its parts.
// Made-up names only (tests.doubles.7). No chart library: bars and graphs are own parts.

export const STANDING = {
  proposed: { sym: "?", word: "proposed" },
  enforced: { sym: "+", word: "enforced" },
  reviewed: { sym: "~", word: "reviewed" },
  planned: { sym: "o", word: "planned" },
  advisory: { sym: "-", word: "advisory" },
  retired: { sym: "x", word: "retired" },
};
export const OBS = {
  "as-set": { sym: "=", word: "as-set" },
  tighter: { sym: ">", word: "tighter" },
  weaker: { sym: "<", word: "weaker" },
  absent: { sym: "_", word: "absent" },
  unmapped: { sym: ".", word: "unmapped" },
  excluded: { sym: "/", word: "excluded" },
};

// The numbers each width draws (web.parts.17). Three named breakpoints (web.layout.4).
export const W = {
  390: { name: "phone", gut: 16, hdr: 60, row: 64, ctl: 44 },
  900: { name: "tablet", gut: 24, hdr: 60, row: 56, ctl: 44 },
  1440: {
    name: "desktop",
    gut: 40,
    hdr: 60,
    row: 52,
    ctl: 32,
    rail: 264,
    list: 392,
  },
};
export const WIDTHS = [390, 900, 1440];

export const css = `
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
body{margin:0;background:#FBFAF7;font-family:"IBM Plex Sans",system-ui,sans-serif}
.sheet{padding:40px;background:#FBFAF7;color:#14130F}
.sheet .row{display:flex;gap:40px;align-items:flex-start;margin-bottom:56px}
.sheet .note{width:360px;flex:none;font-size:14px;line-height:21px;color:#2C2B27;position:sticky;top:24px}
.sheet .note h2{font-size:15px;font-weight:600;margin:0 0 6px}
.sheet .note h3{font-size:12px;font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:#5A5850;margin:18px 0 6px}
.sheet .note p{margin:0 0 8px}
.sheet .note ol,.sheet .note ul{margin:0 0 8px 18px}
.sheet .note li{margin:0 0 4px}
.sheet .note table{border-collapse:collapse;width:100%;font-size:12px}
.sheet .note td,.sheet .note th{border-top:1px solid #E4E2DB;padding:3px 4px;text-align:left;vertical-align:top}
.sheet .note code{font-family:"IBM Plex Mono",ui-monospace,monospace;font-size:12px}
.sheet .cap{font-size:13px;line-height:19px;color:#2C2B27}
.sheet .cap b{display:block;font-size:13px;margin-bottom:2px}
.frame{--fs:1;position:relative;overflow:hidden;background:var(--canvas);color:var(--ink);font-family:"IBM Plex Sans",system-ui,sans-serif;font-size:calc(14px*var(--tg,1));line-height:1.5;box-shadow:0 0 0 1px #CFCCC2;display:flex;flex-direction:column}
.frame[data-state=loading],.frame[data-state=empty],.frame[data-state=error]{min-height:560px}
.frame[data-theme=light]{color-scheme:light;--canvas:#FFFFFF;--surface:#F7F6F2;--hover:#FAFAF8;--ink:#14130F;--prose:#4A4943;--muted:#5A5850;--faint:#6B6962;--line:#E4E2DB;--line-strong:#CFCCC2;--accent:#1E3A8A;--on-accent:#FFFFFF;--accent-soft:#ECEFF8;--danger:#9B1C1C;--danger-soft:#FDF1F1;
 --s-proposed:#5B21B6;--s-proposed-soft:#F3EEFD;--s-proposed-line:#D5C3F5;
 --s-enforced:#14532D;--s-enforced-soft:#E8F3EC;--s-enforced-line:#9BC9AA;
 --s-reviewed:#1E3A8A;--s-reviewed-soft:#ECEFF8;--s-reviewed-line:#C5CCE6;
 --s-planned:#92400E;--s-planned-soft:#FFFBEB;--s-planned-line:#F3D49B;
 --s-advisory:#5A5850;--s-advisory-soft:#F1F0EB;--s-advisory-line:#CFCCC2;
 --s-retired:#475569;--s-retired-soft:#EEF1F5;--s-retired-line:#CBD2DC;
 --skel:#ECEAE3;--logo-vane:#8D97A8;--logo-shade:#5B6574;--logo-quill:#3F4753}
.frame[data-theme=dark]{color-scheme:dark;--canvas:#121210;--surface:#1B1A17;--hover:#191916;--ink:#ECEAE3;--prose:#C4C2B9;--muted:#ABA99F;--faint:#8E8C83;--line:#2C2B27;--line-strong:#47453F;--accent:#A8BBF5;--on-accent:#0D1430;--accent-soft:#1F2642;--danger:#F3A3A3;--danger-soft:#2A1717;
 --s-proposed:#C9B5F5;--s-proposed-soft:#231A3A;--s-proposed-line:#4B3A78;
 --s-enforced:#93D6A9;--s-enforced-soft:#132A1B;--s-enforced-line:#2F5B3E;
 --s-reviewed:#A8BBF5;--s-reviewed-soft:#1F2642;--s-reviewed-line:#3A4478;
 --s-planned:#F2C27A;--s-planned-soft:#2A2110;--s-planned-line:#5C4518;
 --s-advisory:#ABA99F;--s-advisory-soft:#23221F;--s-advisory-line:#47453F;
 --s-retired:#A9B4C4;--s-retired-soft:#1C2128;--s-retired-line:#3B4452;
 --skel:#24231F;--logo-vane:#8D97A8;--logo-shade:#C3CAD6;--logo-quill:#E1E5EC}
.mono{font-family:"IBM Plex Mono",ui-monospace,monospace}
.hdr{display:flex;align-items:center;gap:12px;height:60px;flex:none;border-bottom:1px solid var(--line);padding:0 var(--gut);background:var(--canvas)}
.brand{display:flex;align-items:center;gap:8px;font-weight:600;font-size:calc(15px*var(--tg,1));color:var(--ink);text-decoration:none}
.chipv{font-family:"IBM Plex Mono",ui-monospace,monospace;font-size:calc(11px*var(--tg,1));border:1px solid var(--line-strong);border-radius:4px;padding:2px 6px;color:var(--prose);white-space:nowrap}
.chipv b{font-weight:600;color:var(--ink)}
.nav{display:flex;align-self:stretch;margin-left:8px}
.nav a{display:flex;align-items:center;padding:0 12px;color:var(--muted);text-decoration:none;border-bottom:2px solid transparent;font-size:calc(14px*var(--tg,1))}
.nav a[aria-current]{color:var(--ink);font-weight:600;border-bottom-color:var(--accent)}
.sp{flex:1}
.search{display:flex;align-items:center;gap:8px;height:32px;border:1px solid var(--line);background:var(--surface);border-radius:6px;padding:0 12px;color:var(--faint);font-size:13px;flex:0 1 320px}
.search kbd{margin-left:auto;font-family:"IBM Plex Mono",monospace;font-size:11px;border:1px solid var(--line);border-radius:3px;padding:0 4px;color:var(--muted)}
.ib{display:inline-flex;align-items:center;justify-content:center;gap:6px;min-width:44px;height:44px;border-radius:6px;color:var(--muted);background:transparent;font-size:14px}
.ib.sm{min-width:32px;height:32px}
.unrel{display:none;align-items:center;gap:8px;padding:8px var(--gut);border-bottom:1px solid var(--line);background:var(--surface);font-size:calc(12px*var(--tg,1));color:var(--prose)}
.grown .chipv{display:none}.grown .unrel{display:flex}
.banner{display:flex;gap:12px;align-items:flex-start;padding:12px var(--gut);border-bottom:1px solid var(--s-planned-line);background:var(--s-planned-soft);color:var(--ink);font-size:calc(14px*var(--tg,1))}
.banner .sym{font-family:"IBM Plex Mono",monospace;font-weight:700;color:var(--s-planned);border:1px solid var(--s-planned-line);border-radius:50%;width:22px;height:22px;display:inline-flex;align-items:center;justify-content:center;flex:none;font-size:13px}
.main{padding:24px var(--gut) 40px;display:flex;flex-direction:column;gap:20px;flex:1}
.crumb{font-size:calc(12px*var(--tg,1));color:var(--muted);display:flex;gap:6px;align-items:center}
.crumb a{color:var(--accent);text-decoration:underline}
.focus{display:flex;flex-direction:column;gap:10px}
.focus .t{display:flex;flex-wrap:wrap;align-items:center;gap:12px}
.focus h1{font-family:"IBM Plex Mono",monospace;font-weight:600;font-size:calc(var(--h1)*1px*var(--tg,1));line-height:1.2;color:var(--ink);overflow-wrap:anywhere}
.focus h1.plain{font-family:"IBM Plex Sans",sans-serif}
.focus .stmt{font-size:calc(17px*var(--tg,1));line-height:1.45;color:var(--ink);max-width:62ch}
.focus .facts{display:flex;flex-wrap:wrap;gap:4px 16px;font-size:calc(13px*var(--tg,1));color:var(--muted)}
.acts{display:flex;flex-wrap:wrap;gap:8px;margin-top:4px}
.btn{display:inline-flex;align-items:center;justify-content:center;height:var(--ctl);padding:0 16px;border-radius:6px;border:1px solid var(--line-strong);background:var(--canvas);color:var(--ink);font-weight:500;font-size:calc(14px*var(--tg,1));text-decoration:none}
.btn.p{background:var(--accent);color:var(--on-accent);border-color:var(--accent)}
.st{display:inline-flex;align-items:center;gap:6px;height:24px;padding:0 8px;border-radius:6px;font-size:calc(12px*var(--tg,1));font-weight:500;white-space:nowrap;border:1px solid var(--line);color:var(--ink)}
.st i{font-style:normal;font-family:"IBM Plex Mono",monospace;font-weight:700;font-size:calc(14px*var(--tg,1));width:1ch;text-align:center}
.st.hollow i{color:transparent;-webkit-text-stroke:1px var(--ink)}
.st.big{height:32px;padding:0 12px;font-size:calc(15px*var(--tg,1))}
.st.big i{font-size:calc(18px*var(--tg,1))}
.st.proposed{color:var(--s-proposed);background:var(--s-proposed-soft);border-color:var(--s-proposed-line)}
.st.enforced{color:var(--s-enforced);background:var(--s-enforced-soft);border-color:var(--s-enforced-line)}
.st.reviewed{color:var(--s-reviewed);background:var(--s-reviewed-soft);border-color:var(--s-reviewed-line)}
.st.planned{color:var(--s-planned);background:var(--s-planned-soft);border-color:var(--s-planned-line)}
.st.advisory{color:var(--s-advisory);background:var(--s-advisory-soft);border-color:var(--s-advisory-line)}
.st.retired{color:var(--s-retired);background:var(--s-retired-soft);border-color:var(--s-retired-line)}
.st.obs{color:var(--prose);background:var(--surface);border-color:var(--line)}
.mk{display:inline-flex;align-items:center;gap:4px;height:24px;padding:0 6px;border:1px solid var(--line-strong);border-radius:6px;font-size:calc(12px*var(--tg,1));color:var(--prose)}
.mk i{font-style:normal;font-family:"IBM Plex Mono",monospace;font-weight:700}
.body{display:flex;gap:32px;align-items:flex-start}
.rail{width:264px;flex:none;display:flex;flex-direction:column;gap:4px}
.rail h2,.kinds h2{font-size:calc(12px*var(--tg,1));font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--muted);margin-bottom:4px}
.kind{display:flex;align-items:center;gap:8px;min-height:var(--ctl);padding:0 8px;border-radius:6px;color:var(--ink);font-size:calc(14px*var(--tg,1))}
.kind .box{width:16px;height:16px;border:1.5px solid var(--line-strong);border-radius:3px;display:inline-flex;align-items:center;justify-content:center;font-size:11px;font-weight:700;color:var(--on-accent);flex:none}
.kind .box.on{background:var(--accent);border-color:var(--accent)}
.kind .n{margin-left:auto;font-family:"IBM Plex Mono",monospace;font-size:calc(12px*var(--tg,1));color:var(--muted)}
.kinds{display:flex;flex-wrap:wrap;gap:8px}
.kinds .kind{border:1px solid var(--line);min-height:44px;padding:0 12px}
.kinds .kind.on{border-color:var(--accent);background:var(--accent-soft)}
.col{display:flex;flex-direction:column;gap:16px;min-width:0;flex:1}
.pair{display:flex;gap:32px;align-items:flex-start}
.graph{flex:none;position:sticky;top:12px;border:1px solid var(--line);border-radius:8px;background:var(--surface);position:relative}
.graph svg{display:block}
.graph text{font-family:"IBM Plex Sans",sans-serif}
.gcap{font-size:calc(12px*var(--tg,1));color:var(--muted);padding:8px 12px;border-top:1px solid var(--line)}
.list{display:flex;flex-direction:column;min-width:0;flex:1}
.gh{display:flex;align-items:center;gap:8px;min-height:var(--ctl);padding:0 4px;border-bottom:1px solid var(--line-strong);font-size:calc(12px*var(--tg,1));font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--muted)}
.gh .n{font-family:"IBM Plex Mono",monospace;font-weight:500;margin-left:auto;letter-spacing:0}
.gh.alert{color:var(--ink)}
.r{display:flex;gap:12px;align-items:flex-start;min-height:var(--row);padding:10px 4px;border-bottom:1px solid var(--line);text-decoration:none;color:inherit}
.r .id{font-family:"IBM Plex Mono",monospace;font-size:calc(13px*var(--tg,1));font-weight:500;color:var(--ink);overflow-wrap:anywhere}
.r .id.plain{font-family:"IBM Plex Sans",sans-serif;font-weight:500;font-size:calc(14px*var(--tg,1))}
.r .tx{font-size:calc(13px*var(--tg,1));color:var(--prose);margin-top:2px}
.r .main2{flex:1;min-width:0}
.r .tags{display:flex;gap:6px;flex-wrap:wrap;flex:none;justify-content:flex-end}
.r.stack{flex-direction:column;gap:6px}
.r.stack .tags{justify-content:flex-start}
.kt{font-size:calc(11px*var(--tg,1));color:var(--muted);border:1px solid var(--line);border-radius:4px;padding:0 6px;height:20px;display:inline-flex;align-items:center;white-space:nowrap}
.sk{background:var(--skel);border-radius:6px}
.msg{display:flex;flex-direction:column;gap:12px;max-width:56ch}
.msg h1{font-size:calc(24px*var(--tg,1));line-height:1.25;font-weight:600}
.msg p{font-size:calc(15px*var(--tg,1));color:var(--prose)}
.live{font-size:calc(13px*var(--tg,1));color:var(--muted);display:flex;align-items:center;gap:8px}
`;

export const esc = (s) =>
  String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
const logo = `<svg width="24" height="24" viewBox="0 0 64 64" role="img" aria-label="Gragás"><path fill="var(--logo-vane)" d="M57 5 C 41 8, 23 21, 18 40 L 22 43 C 34 31, 46 19, 57 5 Z"/><path fill="var(--logo-shade)" d="M57 5 C 52 22, 40 37, 27 46 L 22 43 C 34 31, 46 19, 57 5 Z"/><path d="M57 5 C 46 19, 34 31, 22 43 L 8 58" fill="none" stroke="var(--logo-quill)" stroke-width="1.8" stroke-linecap="round"/></svg>`;
const icoSearch = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M20 20l-4-4M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14z"/></svg>`;
const icoMenu = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16"/></svg>`;

export function stand(name, { big = false, trial = false } = {}) {
  const s = STANDING[name];
  return `<span class="st ${name}${trial ? " hollow" : ""}${big ? " big" : ""}" data-part="standing"><i aria-hidden="true">${s.sym}</i>${s.word}</span>`;
}
export function obs(name) {
  const s = OBS[name];
  return `<span class="st obs" data-part="observation"><i aria-hidden="true">${s.sym}</i>${s.word}</span>`;
}
export const mark = `<span class="mk" data-part="mark"><i aria-hidden="true">^</i>trial</span>`;

// A mark beside a standing (spec 17.3). Trial is the hollow symbol and the word.
export const MARKS = { trial: "^", blocked: "!", unproven: "*" };
export function markChip(name) {
  return `<span class="mk ${name}" data-part="mark"><i aria-hidden="true">${MARKS[name]}</i>${name}</span>`;
}

// The five views the desktop header names; the rest are under More (spec S38).
export const NAV = [
  ["rules", "Rules"],
  ["relations", "Relations"],
  ["tools", "Tools"],
  ["coverage", "Coverage"],
  ["changelog", "Changelog"],
];

export function header(w, active) {
  const nav =
    w >= 1440
      ? `<nav class="nav" data-part="nav" aria-label="Sections">${NAV.map(([k, t]) => `<a href="/${k}"${k === active ? ' aria-current="page"' : ""}>${t}</a>`).join("")}</nav>`
      : "";
  const chip = `<span class="chipv" data-part="unreleased">v0.1.0 <b>unreleased</b></span>`;
  const right =
    w >= 1440
      ? `<span class="sp"></span><span class="search" data-part="search">${icoSearch}Search, or type an id<kbd>K</kbd></span><span class="ib sm" data-part="more" style="padding:0 8px">More ${icoMenu}</span>`
      : `<span class="sp"></span><span class="ib" data-part="search-button" aria-label="Search">${icoSearch}</span><span class="ib" data-part="pages-button" aria-label="Pages">${icoMenu}</span>`;
  return `<header class="hdr" data-part="header"><a class="brand">${logo}<span>Gragás</span></a>${chip}${nav}${right}</header><div class="unrel" data-part="unreleased-strip"><span class="chipv">v0.1.0 <b>unreleased</b></span>This rulebook has no release yet.</div>`;
}

export function crumb(part, trail) {
  return `<div class="crumb" data-part="${part}">${trail.map((t, i) => (i < trail.length - 1 ? `<a>${esc(t)}</a><span aria-hidden="true">/</span>` : `<span>${esc(t)}</span>`)).join("")}</div>`;
}

export const bar = (wd, h = 14) =>
  `<div class="sk" style="width:${wd};height:${h}px"></div>`;

export const STATES = {
  default: "Default",
  loading: "Loading",
  empty: "Empty",
  error: "Error",
  stale: "Stale",
  grown: "Text grown",
};
export const STATE_ORDER = [
  ["default", "light"],
  ["default", "dark"],
  ["loading", "light"],
  ["loading", "dark"],
  ["empty", "light"],
  ["empty", "dark"],
  ["error", "light"],
  ["error", "dark"],
  ["stale", "light"],
  ["stale", "dark"],
  ["grown", "light"],
  ["grown", "dark"],
];

export const staleBanner = `<div class="banner" data-part="stale-banner" role="status"><span class="sym" aria-hidden="true">!</span><div><b>This bundle is from an older contract major.</b> It is read as it is, so some fields may be missing. Build it again with <span class="mono">gragas explorer</span>.</div></div>`;

// frame: one width, theme and state of a view. view.body(w, state) gives the <main> of
// default, loading, empty and error; stale and grown are the default with a banner or
// text at 200 percent.
export function frame(view, w, theme, state) {
  const grown = state === "grown";
  const kind = state === "stale" || grown ? "default" : state;
  let body = view.body(w, kind);
  if (state === "stale") body = staleBanner + body;
  const p = W[w];
  const st = `--gut:${p.gut}px;--row:${p.row}px;--ctl:${p.ctl}px;--h1:${w === 390 ? 24 : 28};${grown ? "--tg:2;" : ""}width:${w}px`;
  return `<section class="frame${grown ? " grown" : ""}" data-theme="${theme}" data-state="${state}" data-width="${w}" data-part="frame" style="${st}">${header(w, view.nav)}${body}</section>`;
}

// partsTable: the measured numbers of the view's own parts at width w. A part's measured
// is {measure: [phone, tablet, desktop]} with null where the part is not drawn.
const IDX = { 390: 0, 900: 1, 1440: 2 };
export function partsTable(view, w) {
  const rows = view.parts.map((p) => {
    const cells = Object.entries(p.measured)
      .map(([k, v]) => [k, v[IDX[w]]])
      .filter(([, v]) => v !== null && v !== undefined);
    return [
      p.name,
      cells.length
        ? cells.map(([k, v]) => `${k} ${v}`).join(", ")
        : "not drawn at " + w,
    ];
  });
  rows.unshift(
    ["header", `height ${W[w].hdr}`],
    ["unreleased", "one chip in the header, on every view"],
  );
  rows.push(["standing", "height 24, word and symbol"]);
  return `<table data-part="parts-table"><tr><th>Part</th><th>Number</th></tr>${rows.map((r) => `<tr><td><code>${r[0]}</code></td><td>${r[1]}</td></tr>`).join("")}</table>`;
}

export function firstNote(view, w) {
  const n = view.note;
  return `<aside class="note" data-part="note"><h2>${view.title} at ${w} (${W[w].name})</h2>
<h3>Purpose</h3><p>${n.purpose}</p>
<h3>What it conveys, in order</h3><ol>${n.conveys.map((c) => `<li>${c}</li>`).join("")}</ol>
<h3>How the layout carries that order</h3><p>${n.layout[w]}</p>
<h3>Standing on every row</h3><p>${n.standing}</p>${view.differs ? `<h3>Where it differs from the canvas, and why</h3><ul>${view.differs.map((d) => `<li>${d}</li>`).join("")}</ul>` : ""}
<h3>Measured parts</h3>${partsTable(view, w)}</aside>`;
}

export function capNote(view, state, theme) {
  return `<aside class="note cap" data-part="note"><b>${STATES[state]}, ${theme}</b>${view.caps[state]}${state === "grown" ? " The unreleased chip moves from the header to a strip under it." : ""}</aside>`;
}

export function board(view, w) {
  const rows = STATE_ORDER.map(
    ([s, t], i) =>
      `<div class="row" data-board-row="${s}-${t}">${frame(view, w, t, s)}${i === 0 ? firstNote(view, w) : capNote(view, s, t)}</div>`,
  ).join("");
  return `<div lang="en" class="sheet" data-width="${w}" style="width:${w + 40 + 40 + 360 + 40}px">${rows}</div>`;
}

// The two families are the copies explorer/packages/tokens/fonts already vendors (gragas#1655,
// X274), loaded by a path from the page's folder (canvas/), so no page asks a font service. The
// weights vendored are the ones that exist: Sans 400, 500 and 600, Mono 400 and 500.
const FONTS = "../../../explorer/packages/tokens/fonts";
const faces = [
  [
    "IBM Plex Sans",
    "ibm-plex-sans",
    "IBMPlexSans",
    [
      ["Regular", 400],
      ["Medium", 500],
      ["SemiBold", 600],
    ],
  ],
  [
    "IBM Plex Mono",
    "ibm-plex-mono",
    "IBMPlexMono",
    [
      ["Regular", 400],
      ["Medium", 500],
    ],
  ],
];
const fontFaces = faces
  .flatMap(([family, folder, stem, weights]) =>
    weights.map(
      ([name, weight]) =>
        `@font-face{font-family:"${family}";font-weight:${weight};font-display:swap;src:url("${FONTS}/${folder}/${stem}-${name}.woff2") format("woff2")}`,
    ),
  )
  .join("");

export function page(view, w, extraCss = "") {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${view.title} at ${w} · Gragás</title>
</head>
<body>
<x-dc>
<helmet>
<style>${fontFaces}${css}${extraCss}</style>
</helmet>
${board(view, w)}
</x-dc>
</body>
</html>
`;
}
