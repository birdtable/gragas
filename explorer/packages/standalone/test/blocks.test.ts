// The page has one reader of its JSON script blocks: core's
// documentBlocks. The shell's own copy is gone, and no other file in the
// package names the convention, so a second reader cannot grow back unseen.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { expect, test } from "vitest";

const src = fileURLToPath(new URL("../src", import.meta.url));
const sources = readdirSync(src, { recursive: true })
  .map(String)
  .filter((f) => /\.tsx?$/.test(f));

test("main.tsx reads the page's blocks through core's documentBlocks", () => {
  const main = readFileSync(join(src, "main.tsx"), "utf8");
  expect(main).toMatch(
    /import\s*\{[^}]*\bdocumentBlocks\b[^}]*\}\s*from\s*"@gragas\/core"/,
  );
  expect(main).toMatch(/inline:\s*documentBlocks\(document\)/);
});

test("the shell's own reader is gone and no other file names the convention", () => {
  expect(existsSync(join(src, "inline.ts"))).toBe(false);
  const readers = sources.filter((f) =>
    /application\/json|data-file/.test(readFileSync(join(src, f), "utf8")),
  );
  expect(readers).toEqual([]);
});

test("main.tsx imports the jitless switch before anything that parses (trusted types)", () => {
  const main = readFileSync(join(src, "main.tsx"), "utf8");
  const first = /^import\s.*$/m.exec(main.replace(/^\/\/.*$/gm, ""));
  expect(first?.[0]).toBe('import "./jitless.ts";');
});
