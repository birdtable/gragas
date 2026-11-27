// Builds the contract package only when its dist is older than what it builds from: `node
// scripts/ensure-contract.ts [--package <dir>] [--input <file or dir>]... -- <build command> [args...]`.
// The explorer's typecheck, lint and test each need the contract's dist and used to run its build
// every time. The build's second step is an incremental emit into dist/fixtures, and under load it
// rewrote all five of its files with the same bytes and a new time, so the write-nothing guard
// (scripts/write-nothing.ts) read it as a write. A build that is not run
// cannot write: this runs it when an output is missing or an input is newer than the oldest output,
// and otherwise prints that it skipped it. A fresh checkout, or a source edit, still builds once.
//
// It is a gate (ts.gates.1): the guard is imported first, a missing command is refused, and the
// exit code is set, never process.exit.
import "./gate-guard.ts";
import { program } from "./program.ts";
import { spawn } from "node:child_process";
import { existsSync, lstatSync, readdirSync, utimesSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// outputs are the files the build leaves, each relative to the package: the library's and the
// fixtures' build info, and the fixtures' entry.
const outputs = [
  "dist/.tsbuildinfo",
  "dist/fixtures/.tsbuildinfo",
  "dist/fixtures/index.js",
];

// inputs are what the build reads, relative to the package: its sources, its tsconfigs and manifest,
// the exports the generator wrote, the library and base tsconfigs two folders up, and the lockfile
// there, since a compiler bumped by it emits other bytes from the same sources.
const inputs = [
  "src",
  "tsconfig.build.json",
  "tsconfig.fixtures.json",
  "tsconfig.json",
  "package.json",
  "exports.gen.json",
  "../../tsconfig.library.json",
  "../../tsconfig.base.json",
  "../../pnpm-lock.yaml",
];

// newest is the latest modification time under path, 0 when it is not there.
function newest(path: string): number {
  if (!existsSync(path)) return 0;
  const stat = lstatSync(path);
  if (!stat.isDirectory()) return stat.mtimeMs;
  let latest = stat.mtimeMs;
  for (const entry of readdirSync(path)) {
    latest = Math.max(latest, newest(join(path, entry)));
  }
  return latest;
}

// stale says why the build must run, or "" when the outputs are current.
function stale(pkg: string, extra: string[]): string {
  let oldest = Infinity;
  for (const out of outputs) {
    const path = join(pkg, out);
    if (!existsSync(path)) return `${out} is missing`;
    oldest = Math.min(oldest, lstatSync(path).mtimeMs);
  }
  for (const input of [...inputs, ...extra]) {
    if (newest(resolve(pkg, input)) > oldest)
      return `${input} is newer than dist`;
  }
  return "";
}

// run resolves with the command's exit code, its output going where this script's goes.
function run(cmd: string, args: string[]): Promise<number> {
  return new Promise((resolveRun, reject) => {
    const [file, fileArgs] = program(cmd, args);
    const child = spawn(file, fileArgs, { stdio: "inherit" });
    child.on("error", reject);
    child.on("close", (code) => {
      resolveRun(code ?? 1);
    });
  });
}

async function main(): Promise<void> {
  const argv = process.argv.slice(2);
  let pkg = resolve(
    dirname(fileURLToPath(import.meta.url)),
    "..",
    "packages",
    "contract",
  );
  const extra: string[] = [];
  while (argv[0] === "--package" || argv[0] === "--input") {
    const value = argv[1];
    if (value === undefined)
      throw new Error(`${String(argv[0])} wants a value`);
    if (argv[0] === "--package") pkg = resolve(value);
    else extra.push(value);
    argv.splice(0, 2);
  }
  if (argv[0] === "--") argv.shift();
  const [cmd, ...args] = argv;
  if (cmd === undefined) {
    throw new Error(
      "usage: ensure-contract.ts [--package <dir>] [--input <path>]... -- <build command> [args...]",
    );
  }
  const why = stale(pkg, extra);
  if (why === "") {
    process.stdout.write(`ensure-contract: dist is current, build skipped\n`);
    return;
  }
  process.stdout.write(`ensure-contract: ${why}, building\n`);
  // The stamp is the time before the build, so an input saved while it runs is newer than the
  // outputs and reads as stale next time.
  const before = new Date();
  const code = await run(cmd, args);
  process.exitCode = code;
  if (code !== 0) return;
  // tsc leaves an output it finds current untouched, which would read as stale next to a source
  // whose time moved with no change to its bytes (a checkout): stamp the outputs, once, after a build.
  for (const out of outputs) {
    if (existsSync(join(pkg, out))) utimesSync(join(pkg, out), before, before);
  }
}

await main();
