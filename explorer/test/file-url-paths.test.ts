// A path from a file URL goes through fileURLToPath, never `.pathname`:
// on Windows the pathname of file:///C:/x is /C:/x, which no
// file call opens. The guard reads the explorer's own sources, so a Mac or
// Linux push is red on the pattern a Windows run would fail on.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const explorer = fileURLToPath(new URL("..", import.meta.url));
const skipped = new Set(["node_modules", "dist", "coverage", ".turbo"]);
const sources = new Set([".ts", ".tsx", ".js", ".mjs", ".cjs"]);
// The lint's plants hold the pattern as source text on purpose, for the lint to refuse.
// The fixture is named by join, so the skip is the same on every system.
const plants = join("test", "lint-plants.test.ts");

// The pattern is built from parts so this file does not hold it whole. It reads
// across lines, since a formatter may break the call.
const dot = String.raw`,?\s*\)\s*\.` + "pathname";
const bad = new RegExp(
  String.raw`import\.meta\.url` + dot + "|" + String.raw`\bhere\([^)]*` + dot,
  "g",
);

// offenders lists the spans of a text that take a path from a file URL by its pathname.
export function offenders(text: string): string[] {
  return [...text.matchAll(bad)].map((m) => m[0]);
}

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (skipped.has(name)) continue;
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path, out);
    else if (sources.has(extname(name)) && relative(explorer, path) !== plants)
      out.push(path);
  }
  return out;
}

describe("a path taken from a file URL", () => {
  it("goes through fileURLToPath in every explorer source", () => {
    const found = walk(explorer).flatMap((path) =>
      offenders(readFileSync(path, "utf8")).map((s) => `${path}: ${s}`),
    );
    expect(found).toEqual([]);
  });

  it("is found when a source takes its pathname", () => {
    const plant = `const r = new URL("..", import.meta.url).${"pathname"};`;
    expect(offenders(plant)).toHaveLength(1);
  });

  it("is not found when a source uses fileURLToPath", () => {
    expect(
      offenders('const r = fileURLToPath(new URL("..", import.meta.url));'),
    ).toEqual([]);
  });
});
