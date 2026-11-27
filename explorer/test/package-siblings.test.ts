// supply.install.8 and arch.ports.8: each package declares exactly the sibling
// packages that layers.yaml lets its modules import, no more and no fewer. The
// table is read from docs/explorer-boundaries.json, which gragas-dev renders.
import { readFileSync, readdirSync } from "node:fs";
import { describe, expect, it } from "vitest";
import * as z from "zod";

const root = (path: string): URL => new URL(`../../${path}`, import.meta.url);

const TableSchema = z.object({
  elements: z.array(z.object({ type: z.string(), pattern: z.string() })),
  rules: z.array(z.object({ from: z.string(), allow: z.array(z.string()) })),
});
const PackageSchema = z.object({
  name: z.string(),
  dependencies: z.record(z.string(), z.string()).optional(),
});

const table = TableSchema.parse(
  JSON.parse(readFileSync(root("docs/explorer-boundaries.json"), "utf8")),
);

function packageOf(type: string): string {
  const element = table.elements.find((e) => e.type === type);
  const found = /^explorer\/packages\/([^/]+)/.exec(element?.pattern ?? "");
  if (found?.[1] === undefined)
    throw new Error(`no package for module ${type}`);
  return found[1];
}

// allowed gives, for each package, the sibling packages its modules may import.
export function allowed(): Map<string, Set<string>> {
  const out = new Map<string, Set<string>>();
  for (const rule of table.rules) {
    const own = packageOf(rule.from);
    const set = out.get(own) ?? new Set<string>();
    for (const to of rule.allow.map(packageOf)) if (to !== own) set.add(to);
    out.set(own, set);
  }
  return out;
}

describe("each package's declared siblings", () => {
  const want = allowed();
  const names = readdirSync(root("explorer/packages"));

  it("covers every package folder and no other", () => {
    expect([...want.keys()].sort()).toEqual([...names].sort());
  });

  it.each(names)("%s declares what the table allows", (name) => {
    const manifest = PackageSchema.parse(
      JSON.parse(
        readFileSync(root(`explorer/packages/${name}/package.json`), "utf8"),
      ),
    );
    const declared = Object.entries(manifest.dependencies ?? {})
      .filter(([k]) => k.startsWith("@gragas/"))
      .map(([k, v]) => {
        expect(v).toBe("workspace:*");
        return k.slice("@gragas/".length);
      });
    expect(declared.sort()).toEqual([...(want.get(name) ?? [])].sort());
  });

  it("refuses an unknown module", () => {
    expect(() => packageOf("nowhere")).toThrow();
  });
});
