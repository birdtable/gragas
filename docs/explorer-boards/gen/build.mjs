// Writes the boards of the views named on the command line into ../canvas.
// Usage: node build.mjs [view ...]     (no view: every view in VIEWS)
import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { page, WIDTHS } from "./lib.mjs";

import { VIEWS } from "./views.mjs";
const NAMES = { 390: "Phone", 900: "Tablet", 1440: "Desktop" };
const camel = (k) =>
  k
    .split("-")
    .map((p) => p[0].toUpperCase() + p.slice(1))
    .join("");
const want = process.argv.slice(2);
for (const key of want.length ? want : VIEWS) {
  const { view } = await import(`./${key}.mjs`);
  for (const w of WIDTHS) {
    const file = fileURLToPath(
      new URL(`../canvas/${camel(key)}${NAMES[w]}.dc.html`, import.meta.url),
    );
    writeFileSync(file, page(view, w, view.css || ""));
    console.log("wrote", file.split("/").slice(-2).join("/"));
  }
}
