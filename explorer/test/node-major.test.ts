// supply.runtime.*: one Node major, and every place that names it says the same
// one: the version file, engines in the root and in each package, the types
// package, and every workflow that sets a node-version. The release workflow
// comes with the release recipe (M10); a workflow that sets node-version is held
// to the major from the day it exists, so it cannot be added out of step.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import * as z from "zod";

const root = (path: string): URL => new URL(`../../${path}`, import.meta.url);
const text = (path: string): string => readFileSync(root(path), "utf8");

const PackageSchema = z.object({
  engines: z.object({ node: z.string() }).optional(),
  devDependencies: z.record(z.string(), z.string()).optional(),
});

function major(version: string): string {
  const found = /^v?(\d+)(?:\.\d+){0,2}$/.exec(version.trim());
  if (found?.[1] === undefined) throw new Error(`not a version: ${version}`);
  return found[1];
}

// disagreements names every place whose major is not the first one's.
export function disagreements(
  places: Readonly<Record<string, string>>,
): string[] {
  const entries = Object.entries(places);
  const first = entries[0];
  if (first === undefined) return ["no place names a Node major"];
  const want = major(first[1]);
  return entries
    .filter(([, v]) => major(v) !== want)
    .map(([k, v]) => `${k} says ${v}, ${first[0]} says ${first[1]}`);
}

function workflowVersions(): Record<string, string> {
  const found: Record<string, string> = {};
  const dir = ".github/workflows";
  // No workflows are kept; a workflow added later is held to the major.
  if (!existsSync(root(dir))) return found;
  for (const file of readdirSync(root(dir))) {
    for (const m of text(`${dir}/${file}`).matchAll(
      /node-version:\s*["']?([\dv.]+)["']?/g,
    )) {
      if (m[1] !== undefined) found[`${dir}/${file}`] = m[1];
    }
  }
  return found;
}

describe("the Node major", () => {
  const packages = readdirSync(root("explorer/packages")).map(
    (name) => `explorer/packages/${name}/package.json`,
  );
  const places: Record<string, string> = {
    "explorer/.node-version": text("explorer/.node-version"),
  };
  for (const file of ["explorer/package.json", ...packages]) {
    const engines = PackageSchema.parse(JSON.parse(text(file))).engines?.node;
    if (engines !== undefined) places[`${file} engines`] = engines;
  }
  const types = PackageSchema.parse(JSON.parse(text("explorer/package.json")))
    .devDependencies?.["@types/node"];
  if (types !== undefined) places["@types/node"] = types;
  Object.assign(places, workflowVersions());

  it("is the same in the version file, engines, the types package and the workflows", () => {
    expect(disagreements(places)).toEqual([]);
  });

  it("names the types package and every package's engines, so the check has something to hold", () => {
    expect(Object.keys(places)).toContain("@types/node");
    expect(
      Object.keys(places).filter((k) => k.endsWith("engines")).length,
    ).toBeGreaterThanOrEqual(8);
  });

  it("refuses a place that names another major", () => {
    expect(disagreements({ a: "24.22.1", b: "24", c: "22.1.0" })).toEqual([
      "c says 22.1.0, a says 24.22.1",
    ]);
    expect(disagreements({})).toHaveLength(1);
  });
});
