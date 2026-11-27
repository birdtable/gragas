// supply.install.*: pnpm runs a workspace package's own lifecycle scripts on an
// install, whatever the dependencies' scripts are held to (strictDepBuilds). So no
// package.json of the workspace, the root's and every package's, carries one. The
// test reads the files from disk and writes nothing under the checkout.
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as z from "zod";

const PackageSchema = z.object({
  scripts: z.record(z.string(), z.string()).optional(),
});

// Each name pnpm runs on an install, by family: the dev pre-install, the install
// family and the prepare family.
export const lifecycle = [
  "pnpm:devPreinstall",
  "preinstall",
  "install",
  "postinstall",
  "preprepare",
  "prepare",
  "postprepare",
];

// lifecycleScripts is the names in a package.json's text that run on an install.
export function lifecycleScripts(text: string): string[] {
  const { scripts = {} } = PackageSchema.parse(JSON.parse(text));
  return lifecycle.filter((name) => name in scripts);
}

const explorer = join(dirname(fileURLToPath(import.meta.url)), "..");

// manifests is the workspace's package.json files: the root's and every package's.
function manifests(): string[] {
  const packages = join(explorer, "packages");
  return [
    join(explorer, "package.json"),
    ...readdirSync(packages, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => join(packages, entry.name, "package.json")),
  ];
}

describe("the workspace's package.json files", () => {
  it("carry no install-time lifecycle script", () => {
    const files = manifests();
    expect(files.length).toBeGreaterThan(1);
    const found = files.flatMap((file) =>
      lifecycleScripts(readFileSync(file, "utf8")).map(
        (name) => `${file}: ${name}`,
      ),
    );
    expect(found).toEqual([]);
  });

  it.each([
    ["the dev pre-install", "pnpm:devPreinstall"],
    ["the install family", "postinstall"],
    ["the prepare family", "prepare"],
  ])("is read by the helper: %s is seen", (_family, name) => {
    const text = JSON.stringify({ scripts: { build: "tsc", [name]: "x" } });
    expect(lifecycleScripts(text)).toEqual([name]);
  });

  it("sees every name of every family and passes a package with none", () => {
    for (const name of lifecycle) {
      expect(
        lifecycleScripts(JSON.stringify({ scripts: { [name]: "x" } })),
      ).toEqual([name]);
    }
    expect(
      lifecycleScripts(JSON.stringify({ scripts: { test: "x" } })),
    ).toEqual([]);
    expect(lifecycleScripts("{}")).toEqual([]);
  });
});
