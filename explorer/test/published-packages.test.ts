// ts.packages.1 and supply.bumps.3 bind libraries, and the repository declares
// no library kind (docs/architecture.md, A2), so this test holds them for the
// four packages other code imports until the rulebook can scope a rule by path.
// What it holds today: an exports map and no legacy entry field, each entry's
// files inside the package's "files", and a version of the package's own.
// What it cannot hold yet, because no package has a unit or a build: one
// subpath per unit with no root barrel, and every entry resolving in the packed
// tarball. Both come with the first unit of each package.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import * as z from "zod";

const published = ["contract", "element", "plugin-docs", "plugin-portal"];

const ManifestSchema = z.object({
  version: z.string().regex(/^\d+\.\d+\.\d+$/),
  files: z.array(z.string()).min(1),
  exports: z.record(z.string(), z.record(z.string(), z.string())),
  main: z.never().optional(),
  module: z.never().optional(),
  types: z.never().optional(),
});
type Manifest = z.infer<typeof ManifestSchema>;

const read = (name: string): unknown =>
  JSON.parse(
    readFileSync(
      new URL(`../packages/${name}/package.json`, import.meta.url),
      "utf8",
    ),
  );

// problems says what is wrong with one package's manifest.
export function problems(manifest: Manifest): string[] {
  const out: string[] = [];
  for (const [subpath, conditions] of Object.entries(manifest.exports)) {
    if (!("types" in conditions)) out.push(`${subpath} has no types condition`);
    for (const target of Object.values(conditions)) {
      const inside = manifest.files.some((f) =>
        target.startsWith(`./${f.replace(/\/$/, "")}/`),
      );
      if (!inside) out.push(`${subpath} points at ${target}, outside files`);
    }
  }
  return out;
}

describe("a published package", () => {
  it.each(published)(
    "%s has an exports map, no legacy entry field and files that hold its entries",
    (name) => {
      const manifest = ManifestSchema.parse(read(name));
      expect(problems(manifest)).toEqual([]);
    },
  );

  it("refuses a legacy entry field, an entry outside files and a map without types", () => {
    const good = ManifestSchema.parse(read("contract"));
    expect(
      ManifestSchema.safeParse({ ...good, main: "./dist/index.js" }).success,
    ).toBe(false);
    expect(
      problems({
        ...good,
        exports: {
          ".": { types: "./src/index.ts", default: "./src/index.js" },
        },
      }),
    ).toHaveLength(2);
    expect(
      problems({ ...good, exports: { ".": { default: "./dist/index.js" } } }),
    ).toEqual([". has no types condition"]);
  });
});
