// Writes the parts of each view's boards into explorer/core/parts.seed.yaml. The relations
// entry (pass 1) stays as it is; every other view is written from its module, after it.
// A part is listed once, under the first view that names it, since the same name is the
// same part. Usage: node seed.mjs
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { VIEWS } from "./views.mjs";

const file = fileURLToPath(
  new URL("../../../explorer/core/parts.seed.yaml", import.meta.url),
);
const cur = readFileSync(file, "utf8");
const at = cur.indexOf("\n  - view: ", cur.indexOf("  - view: relations") + 1);
const kept = at < 0 ? cur : cur.slice(0, at + 1);
const seen = new Set([...kept.matchAll(/- name: (\S+)/g)].map((x) => x[1]));
const fmt = (v) =>
  v === null ? "null" : typeof v === "string" ? JSON.stringify(v) : String(v);
const q = (s) => JSON.stringify(s);
let out = kept;
for (const key of VIEWS) {
  const { view } = await import(`./${key}.mjs`);
  const own = view.parts.filter((p) => !seen.has(p.name));
  own.forEach((p) => seen.add(p.name));
  out += `  - view: ${key === "standing-bars" ? "standing" : key}
    route: ${view.route}
    purpose: >-
      ${view.note.purpose}
    conveys:
${view.note.conveys.map((c) => `      - ${q(c)}`).join("\n")}
    breakpoints: { phone: 390, tablet: 900, desktop: 1440 }
    states: [default, loading, empty, error, stale]
    parts:
${own
  .map(
    (p) => `      - name: ${p.name}
        purpose: ${q(p.purpose)}
        measured: { ${Object.entries(p.measured)
          .map(
            ([k, v]) =>
              `${k}: { phone: ${fmt(v[0])}, tablet: ${fmt(v[1])}, desktop: ${fmt(v[2])} }`,
          )
          .join(", ")} }`,
  )
  .join("\n")}
`;
}
writeFileSync(file, out);
console.log(
  "wrote",
  file.split("/").slice(-3).join("/"),
  "with",
  seen.size,
  "parts",
);
