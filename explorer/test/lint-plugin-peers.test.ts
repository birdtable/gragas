// ts.lint-config.9: the test lint plugin is pinned exactly, and its peer range
// for TypeScript accepts the version the workspace uses.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import * as semver from "semver";
import * as z from "zod";

const PackageSchema = z.object({
  version: z.string().optional(),
  peerDependencies: z.record(z.string(), z.string()).optional(),
  devDependencies: z.record(z.string(), z.string()).optional(),
});

const read = (path: string): z.infer<typeof PackageSchema> =>
  PackageSchema.parse(
    JSON.parse(readFileSync(new URL(`../${path}`, import.meta.url), "utf8")),
  );

describe("the test lint plugin", () => {
  const workspace = read("package.json");
  const plugin = read("node_modules/@vitest/eslint-plugin/package.json");
  const typescript = read("node_modules/typescript/package.json");

  it("is pinned to an exact version", () => {
    const pinned = workspace.devDependencies?.["@vitest/eslint-plugin"];
    expect(pinned).toBeDefined();
    expect(semver.valid(pinned)).toBe(pinned);
  });

  it("accepts the TypeScript the workspace has", () => {
    const range = plugin.peerDependencies?.["typescript"];
    expect(range).toBeDefined();
    expect(
      semver.satisfies(typescript.version ?? "0.0.0", range ?? "<0.0.0"),
    ).toBe(true);
  });

  it("refuses a range that leaves the version out", () => {
    const version = typescript.version ?? "0.0.0";
    expect(semver.satisfies(version, "<" + version)).toBe(false);
  });
});
