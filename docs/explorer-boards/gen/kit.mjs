// Small pieces the view modules share: the head, the loading frame, the message of an
// empty or error state, and the measured-part shorthand.
import { crumb, bar, esc } from "./lib.mjs";

export const m = (a, b, c) => [a, b, c];

// head: crumb, title and a count or note line.
export function head(part, trail, title, note = "") {
  return `<section class="focus" data-part="${part}">${crumb(`${part.replace(/-head$/, "")}-crumb`, trail)}<div class="t"><h1 class="plain">${esc(title)}</h1>${note ? `<p class="count">${note}</p>` : ""}</div></section>`;
}

// skeleton: the same frame as grey blocks and one line saying what loads.
export function skeleton(w, part, what, blocks = 5) {
  const rows = Array.from(
    { length: blocks },
    () =>
      `<div class="r" style="gap:12px;align-items:center"><div class="main2" style="display:flex;flex-direction:column;gap:8px">${bar("34%")}${bar("78%", 12)}</div>${bar("72px", 24)}</div>`,
  ).join("");
  const h = `<section class="focus" data-part="${part}" aria-busy="true"><div style="display:flex;gap:12px;align-items:center">${bar("180px", 32)}${bar("110px", 20)}</div><div class="live" role="status"><span aria-hidden="true">...</span>Loading ${what}</div></section>`;
  return `<main class="main">${h}<div class="list">${rows}</div></main>`;
}

// message: the body of an empty or error state, with one or two actions.
export function message(part, trail, h1, paras, acts, role = "") {
  return `<main class="main"><section class="msg" data-part="${part}"${role ? ` role="${role}"` : ""}>${crumb(`${part.replace(/-(empty|error)$/, "")}-crumb`, trail)}<h1>${esc(h1)}</h1>${paras.map((p) => `<p>${p}</p>`).join("")}<div class="acts">${acts}</div></section></main>`;
}
export const act = (part, text, primary = true) =>
  `<a class="btn${primary ? " p" : ""}" data-part="${part}">${esc(text)}</a>`;

export const COMMON_CAPS = {
  loading:
    "The same frame as grey blocks and one line that says what is loading. The region is marked busy.",
  stale:
    "A bundle from an older contract major. It is read, with a banner above the view.",
  grown:
    "Default with text at 200 percent. The rows wrap and nothing scrolls sideways.",
};
export const TABLE_CSS = `
.scroll{overflow-x:auto}.pg{border-collapse:collapse;font-size:calc(13px*var(--tg,1));min-width:100%}.pg th,.pg td{border:1px solid var(--line);padding:6px 10px;text-align:left;white-space:nowrap}.pg th{background:var(--surface);font-weight:600}
.count{font-size:calc(15px*var(--tg,1));color:var(--muted)}.count b{color:var(--ink)}
.mk.unproven{border-style:dotted}.mk.blocked{border-color:var(--danger);color:var(--danger)}
.brow{display:flex;gap:12px;align-items:flex-start;min-height:var(--row);padding:10px 4px;border-bottom:1px solid var(--line);color:inherit;text-decoration:none}
.brow .id{font-family:"IBM Plex Mono",monospace;font-size:calc(13px*var(--tg,1));font-weight:500;overflow-wrap:anywhere}.brow .id.plain{font-family:"IBM Plex Sans",sans-serif;font-size:calc(14px*var(--tg,1))}
.brow .tx{font-size:calc(13px*var(--tg,1));color:var(--prose);margin-top:2px}.brow .main2{flex:1;min-width:0}.brow .tags{display:flex;gap:6px;flex-wrap:wrap;justify-content:flex-end}
.brow.stack{flex-direction:column;gap:6px}.brow.stack .tags{justify-content:flex-start}
.sec{font-size:calc(12px*var(--tg,1));font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--muted)}`;
