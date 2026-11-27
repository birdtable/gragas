// supply.licences.*: every package in the resolved tree carries a licence on
// the allowlist (licences.json), which follows gate/licences.txt; and NOTICES.md
// is what scripts/notices.ts prints from the installed tree, so a shipped
// dependency cannot arrive without its notice. Both read the installed tree and
// touch no network. scripts/notices.ts is a gate (ts.gates.1), so the plants below
// run it against a throwaway workspace.
import { spawnSync, execFileSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import * as z from "zod";
import { pnpmEnv } from "../scripts/pnpm-env.ts";

const here = (path: string): URL => new URL(`../${path}`, import.meta.url);
const PolicySchema = z.object({
  allowed: z.array(z.string()),
  adds: z.record(
    z.string(),
    z.object({ id: z.string(), reason: z.string().min(10) }),
  ),
});
const ListSchema = z.record(
  z.string(),
  z.array(z.object({ name: z.string() })),
);
type Policy = z.infer<typeof PolicySchema>;

// outside names each package whose licence the policy does not hold: a plain id
// any package may carry, or the one id a package's own adds line gives it.
export function outside(
  listing: z.infer<typeof ListSchema>,
  policy: Policy,
): string[] {
  return Object.entries(listing)
    .flatMap(([license, found]) =>
      found.map((p) => ({ name: p.name, license })),
    )
    .filter(
      (p) =>
        !policy.allowed.includes(p.license) &&
        policy.adds[p.name]?.id !== p.license,
    )
    .map((p) => `${p.name}: ${p.license}`);
}

// plainIds reads the plain-id lines of gate/licences.txt, the one allowlist.
function plainIds(): string[] {
  return readFileSync(
    new URL("../../gate/licences.txt", import.meta.url),
    "utf8",
  )
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => /^[A-Za-z0-9.+-]+$/.test(line));
}

function installed(): z.infer<typeof ListSchema> {
  const out = execFileSync("pnpm", ["licenses", "list", "--json"], {
    cwd: here("."),
    env: pnpmEnv(fileURLToPath(here("."))),
    encoding: "utf8",
  });
  return ListSchema.parse(JSON.parse(out.slice(out.indexOf("{\n"))));
}

const policy = PolicySchema.parse(
  JSON.parse(readFileSync(here("licences.json"), "utf8")),
);

describe("the licences", () => {
  // Both listings of the installed tree run once.
  let listing: z.infer<typeof ListSchema> = {};
  let printed = "";
  beforeAll(() => {
    listing = installed();
    printed = execFileSync(process.execPath, ["scripts/notices.ts"], {
      cwd: here("."),
      encoding: "utf8",
    });
  });

  it("are all on the allowlist", () => {
    expect(Object.keys(listing)).not.toEqual([]);
    expect(outside(listing, policy)).toEqual([]);
  });

  it("allow, for every package, only ids the one allowlist holds", () => {
    expect(policy.allowed.filter((id) => !plainIds().includes(id))).toEqual([]);
  });

  it("give an id beyond those to one named package only", () => {
    expect(
      outside(
        { "Python-2.0": [{ name: "argparse" }, { name: "other" }] },
        policy,
      ),
    ).toEqual(["other: Python-2.0"]);
    expect(outside({ "CC0-1.0": [{ name: "argparse" }] }, policy)).toEqual([
      "argparse: CC0-1.0",
    ]);
  });

  it("refuse a licence that is not on it, and an expression that mixes one in", () => {
    expect(
      outside({ GPL: [{ name: "planted" }], MIT: [{ name: "fine" }] }, policy),
    ).toEqual(["planted: GPL"]);
    expect(
      outside({ "(MIT OR GPL-3.0)": [{ name: "mixed" }] }, policy),
    ).toEqual(["mixed: (MIT OR GPL-3.0)"]);
  });

  it("have a generated notices file that is current", () => {
    expect(readFileSync(here("NOTICES.md"), "utf8")).toBe(printed);
  });

  // Risk: a listing that probes the workspace's folders, so every run of this
  // file makes the next push run the Go packages that walk the checkout afresh,
  // which Go's test cache cannot keep. pnpm probes the folder it
  // runs in only when it is given no store, so the cause is held, not the times:
  // the listing runs with the store the install recorded.
  it("are listed with the store the install recorded, which stops pnpm's probe", () => {
    const root = fileURLToPath(here("."));
    const recorded = z
      .object({ storeDir: z.string().min(1) })
      .parse(
        JSON.parse(
          readFileSync(join(root, "node_modules", ".modules.yaml"), "utf8"),
        ),
      ).storeDir;
    expect(pnpmEnv(root)["pnpm_config_store_dir"]).toBe(recorded);
  });
});

describe("the notices gate", () => {
  let scratch = "";
  const script = fileURLToPath(
    new URL("../scripts/notices.ts", import.meta.url),
  );
  const gate = (
    root: string,
    pnpm?: string,
  ): { status: number | null; stderr: string } => {
    const done = spawnSync(
      process.execPath,
      [script, root, ...(pnpm === undefined ? [] : ["--pnpm", pnpm])],
      { encoding: "utf8" },
    );
    return { status: done.status, stderr: done.stderr };
  };

  beforeAll(() => {
    scratch = mkdtempSync(join(tmpdir(), "gragas-notices-"));
  });

  afterAll(() => {
    rmSync(scratch, { recursive: true, force: true });
  });

  it("fails and names itself when it throws before its first rule", () => {
    const done = gate(join(scratch, "no-such-workspace"));
    expect(done.status).not.toBe(0);
    expect(done.stderr).toContain("notices.ts:");
  });

  it("fails on a workspace with no package", () => {
    const root = join(scratch, "empty");
    mkdirSync(join(root, "packages"), { recursive: true });
    const done = gate(root);
    expect(done.status).not.toBe(0);
    expect(done.stderr).toContain("no package");
  });

  // bare writes a workspace at root whose one package, none, declares the
  // dependencies given and has none installed, so its licence listing is {}.
  const bare = (
    root: string,
    dependencies: Record<string, string> = {},
  ): void => {
    mkdirSync(join(root, "packages", "none"), { recursive: true });
    writeFileSync(
      join(root, "packages", "none", "package.json"),
      JSON.stringify({ name: "none", version: "1.0.0", dependencies }),
    );
    writeFileSync(
      join(root, "pnpm-workspace.yaml"),
      "packages:\n  - packages/*\n",
    );
    writeFileSync(
      join(root, "pnpm-lock.yaml"),
      "lockfileVersion: \x279.0\x27\n\nimporters:\n\n  packages/none: {}\n",
    );
  };

  it("fails when no package lists a licence", () => {
    const root = join(scratch, "bare");
    bare(root);
    const done = gate(root);
    expect(done.status).not.toBe(0);
    expect(done.stderr).toContain("empty");
  });

  // pnpm prints {} with exit 0 for a package it cannot list, so a listing that
  // leaves out a declared dependency is red by name, even while another
  // package's notices would fill the file. A workspace:
  // dependency is another package of the workspace, which the listing leaves
  // out, so the control declaring only one passes this rule and stops at the
  // empty file instead.
  it("fails when a package's listing leaves out a dependency it declares", () => {
    const root = join(scratch, "unlisted");
    bare(root, { "left-pad": "1.3.0", other: "workspace:*" });
    const done = gate(root);
    expect(done.status).not.toBe(0);
    expect(done.stderr).toContain("leaves out left-pad,");
  });

  it("does not ask the listing for a workspace dependency", () => {
    const root = join(scratch, "workspace-only");
    bare(root, { other: "workspace:*" });
    const done = gate(root);
    expect(done.status).not.toBe(0);
    expect(done.stderr).not.toContain("leaves out");
    expect(done.stderr).toContain("empty");
  });

  // A pnpm that prints no JSON listing and exits 0 is refused, even for a
  // package that declares nothing, never read as a package with nothing to
  // print. The stub pnpm, a node script the gate runs in its
  // place, lists one package, full, so the file is not empty and only the
  // refusal can make the gate red.
  it("fails when pnpm prints no JSON listing for a package", () => {
    const root = join(scratch, "no-json");
    bare(root);
    const full = join(root, "packages", "full");
    mkdirSync(full);
    writeFileSync(
      join(full, "package.json"),
      JSON.stringify({ name: "full", dependencies: { listed: "1.0.0" } }),
    );
    const stub = join(scratch, "stub-pnpm.mjs");
    writeFileSync(
      stub,
      [
        'import { basename } from "node:path";',
        `const listing = ${JSON.stringify({ MIT: [{ name: "listed", versions: ["1.0.0"], paths: [full], license: "MIT" }] })};`,
        'console.log(basename(process.cwd()) === "full" ? JSON.stringify(listing) : "no listing here");',
        "",
      ].join("\n"),
    );
    const done = gate(root, stub);
    expect(done.status).not.toBe(0);
    expect(done.stderr).toContain("printed no JSON listing");
  });

  // pnpmEnv names the recorded store, given an install that recorded one, and
  // leaves the environment as it is, given none.
  it("names the recorded store in the environment pnpm runs with, and only then", () => {
    const root = join(scratch, "probe");
    bare(root);
    expect(pnpmEnv(root)).toBe(process.env);
    mkdirSync(join(root, "node_modules"));
    writeFileSync(
      join(root, "node_modules", ".modules.yaml"),
      JSON.stringify({ storeDir: join(scratch, "store") }),
    );
    expect(pnpmEnv(root)["pnpm_config_store_dir"]).toBe(join(scratch, "store"));
  });
});
