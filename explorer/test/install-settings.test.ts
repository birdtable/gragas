// supply.bumps.*, supply.install.*: the settings of pnpm-workspace.yaml that
// refuse an install script nobody
// allowed, and hold no release-age window. test/install-refusals.test.ts shows pnpm refusing, but needs a
// loopback listen, which the push gate's sandbox may refuse; these cases read
// the file itself and run in every gate, so a dropped setting is always noticed.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const read = (path: string): string =>
  readFileSync(new URL(path, import.meta.url), "utf8");

// setting is the value of a top-level key of a yaml file, or undefined.
export function setting(yaml: string, key: string): string | undefined {
  const line = yaml.split("\n").find((l) => l.startsWith(`${key}:`));
  return line === undefined
    ? undefined
    : line
        .slice(key.length + 1)
        .split("#")[0]
        ?.trim();
}

// block is the trimmed, non-comment lines indented under a top-level key.
export function block(yaml: string, key: string): string[] {
  const lines = yaml.split("\n");
  const at = lines.findIndex((l) => l.startsWith(`${key}:`));
  if (at < 0) return [];
  const out: string[] = [];
  for (const l of lines.slice(at + 1)) {
    if (!/^\s/.test(l)) break;
    const text = l.split("#")[0]?.trim() ?? "";
    if (text !== "") out.push(text);
  }
  return out;
}

// runs is the allowBuilds entries set to true (a script that may run) whose name
// is not in allowed. allowed maps a package to the reason its script may run;
// it is empty: esbuild is listed as not run (false), and none may run.
export function runs(
  lines: readonly string[],
  allowed: Readonly<Record<string, string>>,
): string[] {
  return lines
    .filter((l) => /:\s*true$/.test(l))
    .map((l) => l.slice(0, l.lastIndexOf(":")).trim())
    .filter((name) => !Object.hasOwn(allowed, name));
}

describe("pnpm-workspace.yaml", () => {
  const workspace = read("../pnpm-workspace.yaml");

  // pnpm 12 holds a one-day window of its own when the key is absent, so the
  // window is off by the key, stated, in both workspaces.
  it("holds no release-age window: minimumReleaseAge is 0", () => {
    expect(setting(workspace, "minimumReleaseAge")).toBe("0");
    expect(
      setting(
        read("../../tools/.node/pnpm-workspace.yaml"),
        "minimumReleaseAge",
      ),
    ).toBe("0");
  });

  it("fails an install that meets a build script, and lets none run", () => {
    expect(setting(workspace, "strictDepBuilds")).toBe("true");
    // The one entry: esbuild, not run.
    expect(block(workspace, "allowBuilds")).toEqual(["esbuild: false"]);
    expect(runs(block(workspace, "allowBuilds"), {})).toEqual([]);
    for (const key of [
      "onlyBuiltDependencies",
      "neverBuiltDependencies",
      "ignoredBuiltDependencies",
      "ignoreScripts",
    ]) {
      expect({ [key]: setting(workspace, key) }).toEqual({ [key]: undefined });
    }
  });

  it("saves exact versions and refuses a Node outside engines", () => {
    expect(setting(workspace, "saveExact")).toBe("true");
    expect(setting(workspace, "engineStrict")).toBe("true");
  });

  it("is read by the helper: a dropped or loosened setting is seen", () => {
    expect(setting("strictDepBuilds: false\n", "strictDepBuilds")).toBe(
      "false",
    );
    expect(setting("saveExact: true\n", "minimumReleaseAge")).toBeUndefined();
    expect(
      setting("minimumReleaseAge: 1440 # a day\n", "minimumReleaseAge"),
    ).toBe("1440");
    // A second entry, or esbuild turned on, is seen by the pin on allowBuilds.
    const two =
      "allowBuilds:\n  esbuild: false\n  swc: true\nsaveExact: true\n";
    expect(block(two, "allowBuilds")).toEqual(["esbuild: false", "swc: true"]);
    expect(block("allowBuilds:\n  esbuild: true\n", "allowBuilds")).toEqual([
      "esbuild: true",
    ]);
    expect(block("saveExact: true\n", "allowBuilds")).toEqual([]);
    // An entry set to true fails unless it is named, with a reason.
    const on = block(
      "allowBuilds:\n  esbuild: false\n  swc: true\n",
      "allowBuilds",
    );
    expect(runs(on, {})).toEqual(["swc"]);
    expect(runs(on, { swc: "builds its binary from source" })).toEqual([]);
    expect(runs(["esbuild: false"], {})).toEqual([]);
  });
});
