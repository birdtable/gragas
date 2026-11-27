// scripts/ensure-contract.ts runs the contract's build only when dist is older than its inputs, so
// the typecheck and the lint, which both start with it, write nothing under a built checkout.
// It is checked on a package of its own under the temp folder, with a planted build
// that appends a line to a log, so the real contract is never built and the test runs in a second.
import { spawnSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  statSync,
  utimesSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

const script = fileURLToPath(
  new URL("../scripts/ensure-contract.ts", import.meta.url),
);

let root = "";
let pkg = "";
let log = "";
beforeEach(() => {
  root = realpathSync(mkdtempSync(join(tmpdir(), "ensure-contract-")));
  pkg = join(root, "packages", "contract");
  log = join(root, "builds.log");
  mkdirSync(join(pkg, "src"), { recursive: true });
  mkdirSync(join(pkg, "dist", "fixtures"), { recursive: true });
  for (const file of [
    "src/a.ts",
    "tsconfig.build.json",
    "tsconfig.fixtures.json",
    "package.json",
    "dist/.tsbuildinfo",
    "dist/fixtures/.tsbuildinfo",
    "dist/fixtures/index.js",
  ]) {
    writeFileSync(join(pkg, file), "x\n");
  }
  writeFileSync(join(root, "tsconfig.library.json"), "{}\n");
  // Every input a minute old, every output a minute newer than that: current.
  at("src", -120);
  for (const file of [
    "src/a.ts",
    "tsconfig.build.json",
    "tsconfig.fixtures.json",
    "package.json",
  ]) {
    at(file, -120);
  }
  for (const file of [
    "dist/.tsbuildinfo",
    "dist/fixtures/.tsbuildinfo",
    "dist/fixtures/index.js",
  ]) {
    at(file, -60);
  }
  const old = new Date(Date.now() - 120_000);
  utimesSync(join(root, "tsconfig.library.json"), old, old);
  writeFileSync(log, "");
});
afterEach(() => {
  rmSync(root, { recursive: true, force: true });
});

// at sets a file's times to so many seconds from now, in the package.
function at(file: string, seconds: number): void {
  const when = new Date(Date.now() + seconds * 1000);
  utimesSync(join(pkg, file), when, when);
}

// ensure runs the script over the planted package with a build that logs one line, or fails when asked.
function ensure(
  build = `require("node:fs").appendFileSync(${JSON.stringify(log)}, "build\\n")`,
): {
  code: number | null;
  out: string;
} {
  const run = spawnSync(
    process.execPath,
    [script, "--package", pkg, "--", process.execPath, "-e", build],
    { encoding: "utf8" },
  );
  return { code: run.status, out: run.stdout + run.stderr };
}

const builds = (): number =>
  readFileSync(log, "utf8").split("\n").filter(Boolean).length;
const mtime = (file: string): number => statSync(join(pkg, file)).mtimeMs;

describe("scripts/ensure-contract.ts", () => {
  it("skips the build when every output is newer than every input, and writes nothing", () => {
    const before = [
      "dist/.tsbuildinfo",
      "dist/fixtures/.tsbuildinfo",
      "dist/fixtures/index.js",
    ].map(mtime);
    const run = ensure();
    expect(run.code).toBe(0);
    expect(run.out).toContain("build skipped");
    expect(builds()).toBe(0);
    const after = [
      "dist/.tsbuildinfo",
      "dist/fixtures/.tsbuildinfo",
      "dist/fixtures/index.js",
    ].map(mtime);
    expect(after).toEqual(before);
  });

  it("builds when an output is missing", () => {
    rmSync(join(pkg, "dist", "fixtures", "index.js"));
    const run = ensure();
    expect(run.code).toBe(0);
    expect(run.out).toContain("dist/fixtures/index.js is missing");
    expect(builds()).toBe(1);
  });

  it("builds when a source file is newer than the oldest output", () => {
    at("src/a.ts", -10);
    const run = ensure();
    expect(run.code).toBe(0);
    expect(run.out).toContain("src is newer than dist");
    expect(builds()).toBe(1);
  });

  it("builds when a tsconfig, the library tsconfig or the manifest is newer", () => {
    for (const file of ["tsconfig.fixtures.json", "package.json"]) {
      at(file, -10);
      expect(ensure().out).toContain(`${file} is newer than dist`);
      at(file, -120);
      at("dist/.tsbuildinfo", -60);
    }
    const when = new Date(Date.now() - 10_000);
    utimesSync(join(root, "tsconfig.library.json"), when, when);
    expect(ensure().out).toContain("tsconfig.library.json is newer than dist");
  });

  it("stamps the outputs after a build, so a source touched without a change is current next time", () => {
    at("src/a.ts", -10);
    expect(ensure().code).toBe(0);
    expect(builds()).toBe(1);
    const run = ensure();
    expect(run.out).toContain("build skipped");
    expect(builds()).toBe(1);
  });

  it("keeps a failing build's exit code and stamps nothing", () => {
    at("src/a.ts", -10);
    const before = mtime("dist/.tsbuildinfo");
    const run = ensure(`process.exit(3)`);
    expect(run.code).toBe(3);
    expect(mtime("dist/.tsbuildinfo")).toBe(before);
  });

  it("builds and stamps once, and the next call after a real build calls no build", () => {
    at("src/a.ts", -10);
    // A build that rewrites every output, as tsc does, at the time it runs.
    const rewrite = `const fs = require("node:fs"); fs.appendFileSync(${JSON.stringify(log)}, "build\\n"); for (const f of ["dist/.tsbuildinfo", "dist/fixtures/.tsbuildinfo", "dist/fixtures/index.js"]) fs.writeFileSync(${JSON.stringify(pkg)} + "/" + f, "y\\n")`;
    expect(ensure(rewrite).code).toBe(0);
    expect(builds()).toBe(1);
    const second = ensure(rewrite);
    expect(second.out).toContain("build skipped");
    expect(builds()).toBe(1);
  });

  it("a build that leaves an input newer than the outputs builds on every call, which a call after a build catches", () => {
    at("src/a.ts", -10);
    // The plant: the build touches an input into the future, so the stamp leaves every output older than it.
    const dirty = `const fs = require("node:fs"); fs.appendFileSync(${JSON.stringify(log)}, "build\\n"); const t = new Date(Date.now() + 3600000); fs.utimesSync(${JSON.stringify(pkg)} + "/src/a.ts", t, t)`;
    ensure(dirty);
    ensure(dirty);
    // Two calls, two builds: the second call after a build was not a skip, and a test that holds
    // "no build after a build" is red on this build.
    expect(builds()).toBe(2);
  });

  it("builds again when an input is touched after a build, so a check that never builds would be red", () => {
    at("src/a.ts", -10);
    expect(ensure().code).toBe(0);
    expect(builds()).toBe(1);
    expect(ensure().out).toContain("build skipped");
    expect(builds()).toBe(1);
    at("tsconfig.build.json", 30);
    const run = ensure();
    expect(run.out).toContain("tsconfig.build.json is newer than dist");
    expect(builds()).toBe(2);
  });

  it("builds when the lockfile is newer, since a compiler it bumped emits other bytes", () => {
    const lock = join(root, "pnpm-lock.yaml");
    writeFileSync(lock, "lockfileVersion: x\n");
    const old = new Date(Date.now() - 120_000);
    utimesSync(lock, old, old);
    expect(ensure().out).toContain("build skipped");
    const when = new Date(Date.now() - 10_000);
    utimesSync(lock, when, when);
    expect(ensure().out).toContain("../../pnpm-lock.yaml is newer than dist");
    expect(builds()).toBe(1);
  });

  it("stamps with the time before the build, so an input saved while it ran is stale next call", () => {
    at("src/a.ts", -10);
    // The build saves an input at the time it runs, a moment after the call began.
    const edit = `const fs = require("node:fs"); fs.appendFileSync(${JSON.stringify(log)}, "build\\n"); const t = new Date(); fs.utimesSync(${JSON.stringify(pkg)} + "/src/a.ts", t, t)`;
    expect(ensure(edit).code).toBe(0);
    const second = ensure(edit);
    expect(second.out).toContain("src is newer than dist");
    expect(builds()).toBe(2);
  });

  it("refuses a missing command", () => {
    const run = spawnSync(process.execPath, [script, "--package", pkg, "--"], {
      encoding: "utf8",
    });
    expect(run.status).toBe(1);
    expect(run.stderr).toContain("usage: ensure-contract.ts");
  });
});
